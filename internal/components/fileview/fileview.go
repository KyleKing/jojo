// Package fileview is the panel listing the files one revision touches, with the diff, the restore,
// and the conflict resolution that only make sense per file. Space checks files and never writes;
// squash, split, and (in the working copy) commit act on the checked files, or on the file under the
// cursor when none is checked. The parent passes the files it already read and performs the jj
// calls; this package never calls jj.
package fileview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// DiffMsg asks for the diff of one file in the revision.
type DiffMsg struct {
	Revision string
	Path     string
}

// RestoreMsg asks to undo one file's change, leaving the rest of the revision alone.
type RestoreMsg struct {
	Revision string
	Path     string
}

// MoveMsg asks to move files into another revision, leaving the rest of this one alone.
type MoveMsg struct {
	Revision string
	Paths    []string
}

// SplitMsg asks to split files out of the revision into a commit of their own.
type SplitMsg struct {
	Revision string
	// Message is the revision's description, which the split-out commit offers to start from.
	Message string
	Paths   []string
}

// ResolveMsg asks to run the merge tool over one conflicted file.
type ResolveMsg struct {
	Revision string
	Path     string
}

// CommitMsg asks to commit files of the working copy, `jj commit <paths>`: they keep @ and its
// description, and the rest moves to a fresh change on top.
type CommitMsg struct {
	Revision string
	// Message is the working copy's description, offered as the commit message.
	Message string
	Paths   []string
}

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// entry is one row: a file and the revision it belongs to.
type entry struct {
	revision string
	jj.FileChange
}

// Model is the panel's state: the files it lists, the revision they belong to, and which are checked.
type Model struct {
	skin     ui.Skin
	revision string
	// message is the revision's description, which a commit or a split offers to start from.
	message string
	checked map[string]bool
	list    listview.Model[entry]
	hints   Hints
	// workingCopy offers c, which jj only runs on @.
	workingCopy bool
}

// Hints are the footer's keys by role, which the caller builds from its keymap so a rebind reaches
// the footer. A role left empty keeps the default key.
type Hints struct {
	Diff, Check, Squash, Split, Commit, Restore, Resolve, Close keyhint.Hint
}

// DefaultHints are the keys the panel answers, which jojo's keymap binds by default.
func DefaultHints() Hints {
	return Hints{
		Diff:    keyhint.Hint{Key: "enter", What: "diff"},
		Check:   keyhint.Hint{Key: "space", What: "check"},
		Squash:  keyhint.Hint{Key: "s", What: "squash…"},
		Split:   keyhint.Hint{Key: "S", What: "split…"},
		Commit:  keyhint.Hint{Key: "c", What: "commit checked"},
		Restore: keyhint.Hint{Key: "R", What: "restore"},
		Resolve: keyhint.Hint{Key: "x", What: "resolve"},
		Close:   keyhint.Hint{Key: "esc", What: "close"},
	}
}

// SetHints is the footer's keys as the user has them bound.
func (m *Model) SetHints(h Hints) { m.hints = h }

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[entry](skin), hints: DefaultHints(), checked: map[string]bool{}}
}

// Open shows the files of revision, with the cursor on the first conflict when there is one, since
// a conflict is the only entry here that blocks the rest of the work. The message is the
// revision's description, which a split offers to start from.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(revision, message string, files []jj.FileChange) tea.Cmd {
	m.show(revision, message, files, false)

	return nil
}

// OpenWorkingCopy is Open for the working copy, where c commits the checked files as well.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) OpenWorkingCopy(revision, message string, files []jj.FileChange) tea.Cmd {
	m.show(revision, message, files, true)

	return nil
}

func (m *Model) show(revision, message string, files []jj.FileChange, workingCopy bool) {
	m.revision, m.message, m.workingCopy = revision, message, workingCopy
	clear(m.checked)

	entries := entriesOf(revision, files)
	m.list.Open("files in "+revision, entries, firstConflict(entries))
}

func entriesOf(revision string, files []jj.FileChange) []entry {
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		entries = append(entries, entry{revision: revision, FileChange: f})
	}

	return entries
}

func firstConflict(entries []entry) int {
	for i, e := range entries {
		if e.Conflict {
			return i
		}
	}

	return 0
}

// Close hides the panel.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the panel is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the panel is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	name := key.String()
	if m.list.MoveKey(name) {
		return nil
	}

	switch name {
	case "enter":
		return m.send(func(revision, path string) tea.Msg { return DiffMsg{Revision: revision, Path: path} })
	case "R":
		return m.send(func(revision, path string) tea.Msg { return RestoreMsg{Revision: revision, Path: path} })
	case "s":
		return m.sendPaths(func(revision string, paths []string) tea.Msg {
			return MoveMsg{Revision: revision, Paths: paths}
		})
	case "S":
		message := m.message

		return m.sendPaths(func(revision string, paths []string) tea.Msg {
			return SplitMsg{Revision: revision, Paths: paths, Message: message}
		})
	case "x":
		return m.resolve()
	case "space":
		m.toggleChecked()

		return nil
	case "c":
		return m.commit()
	case "esc", "q", "h":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) send(build func(revision, path string) tea.Msg) tea.Cmd {
	file, ok := m.list.Selected()
	if !ok {
		return nil
	}

	revision, path := file.revision, file.Path

	return func() tea.Msg { return build(revision, path) }
}

// StartLabels labels the files on screen for a label jump, false when none is drawn.
func (m *Model) StartLabels() bool { return m.list.StartLabels() }

// Labeling reports whether a label jump is waiting for its label.
func (m Model) Labeling() bool { return m.list.Labeling() }

// LabelKey takes a key while the labels are up (see listview.Model.LabelKey).
func (m *Model) LabelKey(key string) (bool, string) { return m.list.LabelKey(key) }

// JumpBack and JumpForward walk the list's jump list (ctrl+o and ctrl+i).
func (m *Model) JumpBack() bool { return m.list.JumpBack() }

// JumpForward walks the jump list forward again.
func (m *Model) JumpForward() bool { return m.list.JumpForward() }

// Selected is the revision and path of the file under the cursor, as jj's summary prints the path.
func (m Model) Selected() (string, string, bool) {
	file, ok := m.list.Selected()

	return file.revision, file.Path, ok
}

// toggleChecked checks the file under the cursor, or unchecks a checked one. It never writes: the
// verbs that follow act on what is checked.
func (m *Model) toggleChecked() {
	file, ok := m.list.Selected()
	if !ok {
		return
	}

	if m.checked[file.Path] {
		delete(m.checked, file.Path)

		return
	}

	m.checked[file.Path] = true
}

// Checked is the paths checked, in list order.
func (m Model) Checked() []string {
	var out []string

	for _, file := range m.list.Items() {
		if m.checked[file.Path] {
			out = append(out, file.Path)
		}
	}

	return out
}

// targets is what a verb acts on: the checked files, else the file under the cursor.
func (m Model) targets() []string {
	if checked := m.Checked(); len(checked) > 0 {
		return checked
	}

	if file, ok := m.list.Selected(); ok {
		return []string{file.Path}
	}

	return nil
}

func (m *Model) sendPaths(build func(revision string, paths []string) tea.Msg) tea.Cmd {
	paths := m.targets()
	if len(paths) == 0 {
		return nil
	}

	revision := m.revision

	return func() tea.Msg { return build(revision, paths) }
}

func (m *Model) commit() tea.Cmd {
	if !m.workingCopy {
		m.list.Warn("commit works in the files of the working copy, @; S splits files out here")

		return nil
	}

	message := m.message

	return m.sendPaths(func(revision string, paths []string) tea.Msg {
		return CommitMsg{Revision: revision, Paths: paths, Message: message}
	})
}

func (m *Model) resolve() tea.Cmd {
	file, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if !file.Conflict {
		m.list.Warn("only a conflicted file has anything to resolve")

		return nil
	}

	revision, path := file.revision, file.Path

	return func() tea.Msg { return ResolveMsg{Revision: revision, Path: path} }
}

// View renders one row per file, centered in a width by height frame.
func (m Model) View(width, height int) string {
	h := m.hints
	hints := []keyhint.Hint{h.Diff, h.Check, h.Squash}

	if m.workingCopy {
		hints = append(hints, h.Commit)
	}

	hints = append(hints, h.Split, h.Restore, h.Resolve, h.Close)

	return m.list.View(width, height, m.row, hints)
}

// statusWidth keeps the paths in a column past jj's one-letter status.
const statusWidth = 2

// conflictMark replaces the status letter jj leaves empty for a file that only the conflict list
// names, which is every conflict in a merge.
const conflictMark = "!"

// checkMark leads a checked row, the same mark the log's checked revisions carry, and checkWidth
// keeps every path in one column beside it.
const (
	checkMark  = "●"
	checkWidth = 2
)

func (m Model) row(file entry, selected bool) listview.Row {
	status, style := file.Status, m.skin.Body

	switch {
	case file.Conflict:
		status, style = conflictMark, m.skin.Conflicted
	case status == "A":
		style = m.skin.Added
	case status == "D":
		style = m.skin.Removed
	}

	if selected {
		style = m.skin.Selected
	}

	mark := ""
	if m.checked[file.Path] {
		mark = checkMark
	}

	text := table.Pad(mark, checkWidth, table.AlignLeft) + table.Pad(status, statusWidth, table.AlignLeft) + " "

	out := listview.Row{Text: text + file.Path, Style: style}
	if !selected {
		out.Drawn = m.drawn(file, status, style, mark)
	}

	return out
}

// drawn is a file's row in pieces: the check mark in the accent, its status in the status's color,
// the file type's icon, and the path with its directory muted so the eye lands on the name.
func (m Model) drawn(file entry, status string, style lipgloss.Style, mark string) string {
	var b strings.Builder

	b.WriteString(m.skin.Accent.Render(table.Pad(mark, checkWidth, table.AlignLeft)))
	b.WriteString(style.Render(table.Pad(status, statusWidth, table.AlignLeft)) + " ")

	if g := m.skin.Icons.File(file.Path); g.Text != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(g.Color).Render(g.Prefix()))
	}

	cut := strings.LastIndex(file.Path, "/") + 1
	dir, name := file.Path[:cut], file.Path[cut:]
	b.WriteString(m.skin.Muted.Render(dir) + m.skin.Body.Render(name))

	return b.String()
}
