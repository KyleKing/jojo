// Package oplogview is a scrollable modal over `jj op log`, used to pick an
// operation to restore to.
package oplogview

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

// RestoreMsg is sent when Enter or r confirms an operation to restore to.
type RestoreMsg struct {
	Operation jj.Operation
}

// ShowMsg is sent when d asks to see what an operation changed.
type ShowMsg struct {
	Operation jj.Operation
}

// RevertMsg is sent when v asks to undo one operation while keeping every later one.
type RevertMsg struct {
	Operation jj.Operation
}

// CancelledMsg is sent when Esc, q, or o dismisses the view without picking one.
type CancelledMsg struct{}

// Model is the operation log's state: the operations, the cursor into them,
// and whether the view is open.
type Model struct {
	skin    ui.Skin
	warning string
	ops     []jj.Operation
	cursor  int
	open    bool
}

// New returns a closed view drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the view over ops, with the cursor on the current operation, or
// the first entry when none is marked current.
//
//nolint:unparam // matches prompt.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(ops []jj.Operation) tea.Cmd {
	m.ops = ops
	m.warning = ""
	m.cursor = 0
	m.open = true

	for i, op := range ops {
		if op.Current {
			m.cursor = i

			break
		}
	}

	return nil
}

// Close hides the view.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the view is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the view is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	return m.handleKey(key)
}

// pageSize is how far pgup/pgdown move the cursor, since the view has no
// stored height to derive one from.
const pageSize = 10

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc", "q", "o":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter", "r":
		return m.restore()
	case "d":
		return m.emit(func(op jj.Operation) tea.Msg { return ShowMsg{Operation: op} })
	case "v":
		return m.revert()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-pageSize)
	case "pgdown", "ctrl+d":
		m.moveCursor(pageSize)
	case "g", "home":
		m.setCursor(0)
	case "G", "end":
		m.setCursor(len(m.ops) - 1)
	}

	return nil
}

// emit reports the operation under the cursor without closing, so several can be inspected in a row.
func (m *Model) emit(build func(jj.Operation) tea.Msg) tea.Cmd {
	op, ok := m.selected()
	if !ok {
		return nil
	}

	return func() tea.Msg { return build(op) }
}

// revert refuses the current operation, which has nothing after it to keep and would only be a
// slower undo.
func (m *Model) revert() tea.Cmd {
	op, ok := m.selected()
	if !ok {
		return nil
	}

	if op.Current {
		m.warning = "press u to undo the current operation"

		return nil
	}

	m.Close()

	return func() tea.Msg { return RevertMsg{Operation: op} }
}

func (m *Model) restore() tea.Cmd {
	op, ok := m.selected()
	if !ok {
		return nil
	}

	if op.Current {
		m.warning = "already at this operation"

		return nil
	}

	m.Close()

	return func() tea.Msg { return RestoreMsg{Operation: op} }
}

func (m Model) selected() (jj.Operation, bool) {
	if m.cursor < 0 || m.cursor >= len(m.ops) {
		return jj.Operation{}, false
	}

	return m.ops[m.cursor], true
}

func (m *Model) moveCursor(delta int) {
	m.setCursor(m.cursor + delta)
}

func (m *Model) setCursor(n int) {
	if len(m.ops) == 0 {
		return
	}

	m.cursor = clamp(n, 0, len(m.ops)-1)
	m.warning = ""
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of operations to render, keeping
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

// currentMarker flags the operation the working copy is currently at; other
// rows get the same width in spaces so ids line up.
const currentMarker = "@ "

var gutterSpace = strings.Repeat(" ", len(currentMarker))

// ageWidth keeps the operation ids and descriptions in a column, since jj's ages run from "2
// minutes ago" to "3 months ago".
const ageWidth = 14

// View renders the title, an optional warning, the operation rows, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{m.skin.Title.Render("operation log")}

	if m.warning != "" {
		lines = append(lines, m.skin.Error.Render(m.warning))
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "enter", What: "restore"},
		{Key: "d", What: "diff"},
		{Key: "v", What: "revert"},
		{Key: "esc", What: "cancel"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.ops) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.ops), body)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.ops[i], i == m.cursor, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.ops), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) renderRow(op jj.Operation, selected bool, width int) string {
	text := table.Truncate(rowText(op), width)

	if selected {
		return m.skin.Selected.Render(text)
	}

	if op.Current {
		return m.skin.WorkingCopy.Render(text)
	}

	return m.skin.Body.Render(text)
}

func rowText(op jj.Operation) string {
	marker := gutterSpace
	if op.Current {
		marker = currentMarker
	}

	return marker + table.Pad(op.Age, ageWidth, table.AlignLeft) + " " + op.ID + " " + op.Description
}

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
