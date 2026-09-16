// Package workspaceview is the panel listing `jj workspace list` entries, letting
// the user add or forget a workspace. The parent passes the rows it already has
// and performs the jj calls; this package never calls jj.
package workspaceview

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

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
	skin    ui.Skin
	warning string
	spaces  []jj.Workspace
	cursor  int
	open    bool
}

// New returns a closed panel drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the panel over spaces, with the cursor on the current workspace
// or the first entry when none is marked current.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(spaces []jj.Workspace) tea.Cmd {
	m.spaces = spaces
	m.warning = ""
	m.open = true
	m.cursor = 0

	for i, ws := range spaces {
		if ws.Current {
			m.cursor = i

			break
		}
	}

	return nil
}

// Close hides the panel.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the panel is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the panel is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "g", "home":
		m.jumpTo(0)
	case "G", "end":
		m.jumpTo(len(m.spaces) - 1)
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
	ws, ok := m.selected()
	if !ok {
		return nil
	}

	if ws.Current {
		m.warning = "cannot forget the workspace you are in"

		return nil
	}

	return func() tea.Msg { return ForgetMsg{Name: ws.Name} }
}

func (m Model) selected() (jj.Workspace, bool) {
	if m.cursor < 0 || m.cursor >= len(m.spaces) {
		return jj.Workspace{}, false
	}

	return m.spaces[m.cursor], true
}

func (m *Model) moveCursor(delta int) {
	if len(m.spaces) == 0 {
		return
	}

	m.jumpTo(clamp(m.cursor+delta, 0, len(m.spaces)-1))
}

func (m *Model) jumpTo(cursor int) {
	if len(m.spaces) == 0 {
		return
	}

	m.cursor = clamp(cursor, 0, len(m.spaces)-1)
	m.warning = ""
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of workspaces to render, keeping
// cursor inside it, when body rows are available for total items.
func scrollWindow(cursor, total, body int) (int, int) {
	if body <= 0 || total <= body {
		return 0, total
	}

	start := clamp(cursor-body/scrollCenterDivisor, 0, total-body)

	return start, start + body
}

// footerRows is how much of the frame a clipped list gives up to the
// scroll-position line.
const footerRows = 1

// noDescription is shown in place of an empty workspace description.
const noDescription = "(no description set)"

// View renders the title, a warning line when set, one row per workspace, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{m.skin.Title.Render("workspaces")}

	if m.warning != "" {
		lines = append(lines, m.skin.Error.Render(m.warning))
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "a", What: "add"},
		{Key: "f", What: "forget"},
		{Key: "esc", What: "close"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.spaces) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.spaces), body)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.spaces[i], i == m.cursor, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.spaces), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) renderRow(ws jj.Workspace, selected bool, width int) string {
	gutter := "  "
	if ws.Current {
		gutter = "@ "
	}

	text := table.Truncate(gutter+rowText(ws), width)

	if selected {
		return m.skin.Selected.Render(text)
	}

	if ws.Current {
		return m.skin.WorkingCopy.Render(text)
	}

	return m.skin.Body.Render(text)
}

// nameWidth and changeIDWidth keep the descriptions in a column, since workspace names vary and jj
// shortens a change id to whatever is unambiguous.
const (
	nameWidth     = 12
	changeIDWidth = 8
)

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

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
