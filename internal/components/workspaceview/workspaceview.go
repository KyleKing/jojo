// Package workspaceview is the panel listing `jj workspace list` entries, letting
// the user add or forget a workspace. The parent passes the rows it already has
// and performs the jj calls; this package never calls jj.
package workspaceview

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// AddRequestedMsg is sent when the user asks to add a workspace; the parent
// opens a prompt for its name.
type AddRequestedMsg struct{}

// ForgetMsg is sent when the user asks to forget the workspace named Name.
type ForgetMsg struct {
	Name string
}

// CancelledMsg is sent when the panel is dismissed without an action.
type CancelledMsg struct{}

// Model is the panel's state: the workspaces it lists and the cursor into them.
type Model struct {
	skin ui.Skin
	list listview.Model[jj.Workspace]
}

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[jj.Workspace](skin)}
}

// Open shows the panel over spaces, with the cursor on the current workspace
// or the first entry when none is marked current.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(spaces []jj.Workspace) tea.Cmd {
	cursor := 0

	for i, ws := range spaces {
		if ws.Current {
			cursor = i

			break
		}
	}

	m.list.Open("workspaces", spaces, cursor)

	return nil
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
	case "a":
		return func() tea.Msg { return AddRequestedMsg{} }
	case "f":
		return m.forget()
	case "esc", "q", "w":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) forget() tea.Cmd {
	ws, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if ws.Current {
		m.list.Warn("cannot forget the workspace you are in")

		return nil
	}

	return func() tea.Msg { return ForgetMsg{Name: ws.Name} }
}

// noDescription is shown in place of an empty workspace description.
const noDescription = "(no description set)"

// View renders the title, a warning line when set, one row per workspace, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "a", What: "add"},
		{Key: "f", What: "forget"},
		{Key: "esc", What: "close"},
	})
}

// nameWidth and changeIDWidth keep the descriptions in a column, since workspace names vary and jj
// shortens a change id to whatever is unambiguous.
const (
	nameWidth     = 12
	changeIDWidth = 8
)

func (m Model) row(ws jj.Workspace, selected bool) listview.Row {
	gutter := "  "
	style := m.skin.Body

	if ws.Current {
		gutter, style = "@ ", m.skin.WorkingCopy
	}

	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: gutter + rowText(ws), Style: style}
}

func rowText(ws jj.Workspace) string {
	description := ws.Description
	if description == "" {
		description = noDescription
	}

	return fmt.Sprintf("%s %s %s %s",
		table.Pad(ws.Name, nameWidth, table.AlignLeft),
		table.Pad(ws.ChangeID, changeIDWidth, table.AlignLeft),
		description, ws.Root)
}
