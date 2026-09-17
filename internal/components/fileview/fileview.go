// Package fileview is the panel listing the files one revision touches, with the diff, the restore,
// and the conflict resolution that only make sense per file. The parent passes the files it already
// read and performs the jj calls; this package never calls jj.
package fileview

import (
	tea "charm.land/bubbletea/v2"
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

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// Model is the panel's state: the files it lists and the revision they belong to.
type Model struct {
	skin     ui.Skin
	revision string
	list     listview.Model[jj.FileChange]
}

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[jj.FileChange](skin)}
}

// Open shows the files of revision, with the cursor on the first conflict when there is one, since
// a conflict is the only entry here that blocks the rest of the work.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(revision string, files []jj.FileChange) tea.Cmd {
	m.revision = revision
	m.list.Open("files in "+revision, files, firstConflict(files))

	return nil
}

func firstConflict(files []jj.FileChange) int {
	for i, f := range files {
		if f.Conflict {
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
		return m.send(func(path string) tea.Msg { return DiffMsg{Revision: m.revision, Path: path} })
	case "r":
		return m.send(func(path string) tea.Msg { return RestoreMsg{Revision: m.revision, Path: path} })
	case "m":
		return m.send(func(path string) tea.Msg { return MoveMsg{Revision: m.revision, Path: path} })
	case "c":
		return m.resolve()
	case "esc", "q", "l":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) send(build func(path string) tea.Msg) tea.Cmd {
	file, ok := m.list.Selected()
	if !ok {
		return nil
	}

	path := file.Path

	return func() tea.Msg { return build(path) }
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

	path := file.Path

	return func() tea.Msg { return ResolveMsg{Revision: m.revision, Path: path} }
}

// View renders one row per file, centered in a width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "enter", What: "diff"},
		{Key: "r", What: "restore"},
		{Key: "m", What: "move"},
		{Key: "c", What: "resolve"},
		{Key: "esc", What: "close"},
	})
}

// statusWidth keeps the paths in a column past jj's one-letter status.
const statusWidth = 2

// conflictMark replaces the status letter jj leaves empty for a file that only the conflict list
// names, which is every conflict in a merge.
const conflictMark = "!"

func (m Model) row(file jj.FileChange, selected bool) listview.Row {
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

	return listview.Row{
		Text:  table.Pad(status, statusWidth, table.AlignLeft) + " " + file.Path,
		Style: style,
	}
}
