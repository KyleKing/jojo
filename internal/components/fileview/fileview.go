// Package fileview is the panel listing the files one revision touches, with the diff, the restore,
// and the conflict resolution that only make sense per file. The file list of the working copy also
// speaks git: it lists the parent's files as staged, and stages, unstages, and commits with the
// parent standing in for the index. The parent passes the files it already read and performs the jj
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

// MoveMsg asks to move one file into another revision, leaving the rest of this one alone.
type MoveMsg struct {
	Revision string
	Path     string
}

// ResolveMsg asks to run the merge tool over one conflicted file.
type ResolveMsg struct {
	Revision string
	Path     string
}

// StageMsg asks to move one file from the working copy into its parent, git's `add`.
type StageMsg struct {
	Revision string
	Path     string
}

// UnstageMsg asks to move one file from the working copy's parent back into it, git's
// `restore --staged`.
type UnstageMsg struct {
	Revision string
	Path     string
}

// CommitMsg asks to describe the index and start a fresh one, git's `commit`.
type CommitMsg struct {
	Revision string
	// Message is the index's description, offered as the commit message.
	Message string
}

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// Staging is what the file list of @ adds when it speaks git: the parent standing in for the index
// and the files staged in it, or why the parent cannot stand in for one.
type Staging struct {
	// Index is the change id of @-, empty when Refused says why there is none.
	Index string
	// Refused is shown in place of acting when a staging key is pressed, so the vocabulary never
	// rewrites a revision past the working copy.
	Refused string
	// Message is the index's description, which a commit offers to keep.
	Message string
	Files   []jj.FileChange
}

// entry is one row: a file and the revision it belongs to, which in the file list of @ is either the
// working copy or the index below it.
type entry struct {
	revision string
	jj.FileChange
	staged bool
}

// Model is the panel's state: the files it lists and the revision they belong to.
type Model struct {
	skin     ui.Skin
	revision string
	staging  *Staging
	list     listview.Model[entry]
	hints    Hints
}

// Hints are the footer's keys by role, which the caller builds from its keymap so a rebind reaches
// the footer. A role left empty keeps the default key.
type Hints struct {
	Diff, Restore, Squash, Resolve, Stage, Commit, Close keyhint.Hint
}

// DefaultHints are the keys the panel answers, which jojo's keymap binds by default.
func DefaultHints() Hints {
	return Hints{
		Diff:    keyhint.Hint{Key: "enter", What: "diff"},
		Restore: keyhint.Hint{Key: "R", What: "restore"},
		Squash:  keyhint.Hint{Key: "s", What: "squash"},
		Resolve: keyhint.Hint{Key: "x", What: "resolve"},
		Stage:   keyhint.Hint{Key: "space", What: "stage"},
		Commit:  keyhint.Hint{Key: "c", What: "commit"},
		Close:   keyhint.Hint{Key: "esc", What: "close"},
	}
}

// SetHints is the footer's keys as the user has them bound.
func (m *Model) SetHints(h Hints) { m.hints = h }

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[entry](skin), hints: DefaultHints()}
}

// Open shows the files of revision, with the cursor on the first conflict when there is one, since
// a conflict is the only entry here that blocks the rest of the work.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(revision string, files []jj.FileChange) tea.Cmd {
	m.revision, m.staging = revision, nil
	entries := entriesOf(revision, files, false)
	m.list.Open("files in "+revision, entries, firstConflict(entries))

	return nil
}

// OpenWorkingCopy shows the files of the working copy revision as unstaged, followed by the files of
// its parent as staged. Reopening the list it already shows keeps the cursor where it was, so staging
// file after file does not send it back to the top each time.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) OpenWorkingCopy(revision string, files []jj.FileChange, staging Staging) tea.Cmd {
	cursor := -1
	if m.list.IsOpen() && m.staging != nil && m.revision == revision {
		cursor = m.list.Cursor()
	}

	m.revision, m.staging = revision, &staging
	entries := append(entriesOf(revision, files, false), entriesOf(staging.Index, staging.Files, true)...)

	title := "files in " + revision
	if staging.Index != "" {
		title += ", staged in " + staging.Index
	}

	if cursor < 0 {
		cursor = firstConflict(entries)
	}

	m.list.Open(title, entries, cursor)

	return nil
}

func entriesOf(revision string, files []jj.FileChange, staged bool) []entry {
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		entries = append(entries, entry{revision: revision, FileChange: f, staged: staged})
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
		return m.send(func(revision, path string) tea.Msg { return MoveMsg{Revision: revision, Path: path} })
	case "x":
		return m.resolve()
	case "space":
		return m.toggleStaged()
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

// stagingRefusal is why a staging key does nothing here, "" when it can act.
func (m *Model) stagingRefusal() string {
	switch {
	case m.staging == nil:
		return "staging works in the files of the working copy, @"
	case m.staging.Refused != "":
		return m.staging.Refused
	}

	return ""
}

// toggleStaged stages a working-copy file and unstages a staged one, as space does in lazygit.
func (m *Model) toggleStaged() tea.Cmd {
	if why := m.stagingRefusal(); why != "" {
		m.list.Warn(why)

		return nil
	}

	file, ok := m.list.Selected()
	if !ok {
		return nil
	}

	revision, path := m.revision, file.Path
	if file.staged {
		return func() tea.Msg { return UnstageMsg{Revision: revision, Path: path} }
	}

	return func() tea.Msg { return StageMsg{Revision: revision, Path: path} }
}

func (m *Model) commit() tea.Cmd {
	if why := m.stagingRefusal(); why != "" {
		m.list.Warn(why)

		return nil
	}

	if len(m.staging.Files) == 0 {
		m.list.Warn("nothing staged: space stages the file under the cursor")

		return nil
	}

	revision, message := m.revision, m.staging.Message

	return func() tea.Msg { return CommitMsg{Revision: revision, Message: message} }
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

// View renders one row per file, centered in a width by height frame. The file list of @ leads with
// git's words, since that is who the staging keys are for.
func (m Model) View(width, height int) string {
	h := m.hints
	hints := []keyhint.Hint{h.Diff, h.Restore, h.Squash, h.Resolve, h.Close}

	if m.staging != nil {
		stage := h.Stage
		stage.What = m.spaceWord()
		hints = append([]keyhint.Hint{stage, h.Commit}, hints...)
	}

	return m.list.View(width, height, m.row, hints)
}

// spaceWord names what space does to the row under the cursor.
func (m Model) spaceWord() string {
	if file, ok := m.list.Selected(); ok && file.staged {
		return "unstage"
	}

	return "stage"
}

// statusWidth keeps the paths in a column past jj's one-letter status.
const statusWidth = 2

// conflictMark replaces the status letter jj leaves empty for a file that only the conflict list
// names, which is every conflict in a merge.
const conflictMark = "!"

// stagedTag marks a row of the index in the file list of @, and stagedWidth keeps the working
// copy's paths in the same column beside it.
const (
	stagedTag   = "staged"
	stagedWidth = len(stagedTag) + 1
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

	text := table.Pad(status, statusWidth, table.AlignLeft) + " "

	tag := ""
	if m.staging != nil {
		if file.staged {
			tag = stagedTag
		}

		text += table.Pad(tag, stagedWidth, table.AlignLeft)
	}

	out := listview.Row{Text: text + file.Path, Style: style}
	if !selected {
		out.Drawn = m.drawn(file, status, style, tag)
	}

	return out
}

// drawn is a file's row in pieces: its status in the status's color, the
// index's tag a step back, the file type's icon, and the path with its
// directory muted so the eye lands on the name.
func (m Model) drawn(file entry, status string, style lipgloss.Style, tag string) string {
	var b strings.Builder

	b.WriteString(style.Render(table.Pad(status, statusWidth, table.AlignLeft)) + " ")

	if m.staging != nil {
		b.WriteString(m.skin.Muted.Render(table.Pad(tag, stagedWidth, table.AlignLeft)))
	}

	if g := m.skin.Icons.File(file.Path); g.Text != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(g.Color).Render(g.Prefix()))
	}

	cut := strings.LastIndex(file.Path, "/") + 1
	dir, name := file.Path[:cut], file.Path[cut:]
	b.WriteString(m.skin.Muted.Render(dir) + m.skin.Body.Render(name))

	return b.String()
}
