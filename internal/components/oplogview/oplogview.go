// Package oplogview is the scoped view over `jj op log`: jj's undo history, browsable rather than
// reachable only through a bare `jj undo`.
package oplogview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
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

// CancelledMsg is sent when Esc, q, or o dismisses the view.
type CancelledMsg struct{}

// MovedMsg is sent when the cursor lands on another operation, so the log beside the list can be
// redrawn as it stood there. Ago counts the operations since, 0 for the current one.
type MovedMsg struct {
	Operation jj.Operation
	Ago       int
}

// Model is the operation log.
type Model struct {
	skin ui.Skin
	list listview.Model[jj.Operation]
}

// New returns a closed view drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[jj.Operation](skin)}
}

// Open shows ops with the cursor on the current operation, or the first entry when none is current.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(ops []jj.Operation) tea.Cmd {
	cursor := 0

	for i, op := range ops {
		if op.Current {
			cursor = i

			break
		}
	}

	m.list.Open("operation log", ops, cursor)

	return nil
}

// OpenAt shows ops with the cursor on index, which is how time travel hands its place to the list.
func (m *Model) OpenAt(ops []jj.Operation, index int) {
	m.list.Open("operation log", ops, index)
}

// Ago is how many operations the one under the cursor is behind the current one.
func (m Model) Ago() int { return m.list.Cursor() }

// Close hides the view.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the view is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the view is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	name := key.String()
	before := m.list.Cursor()

	if m.list.Labeling() {
		moved, missed := m.list.LabelKey(name)
		if missed != "" {
			m.list.Warn("no label " + missed)
		}

		return m.moved(moved && m.list.Cursor() != before)
	}

	if cmd, ok := m.jumpKey(name, before); ok {
		return cmd
	}

	if m.list.MoveKey(name) {
		if m.list.Cursor() == before {
			return nil
		}

		return m.emit(func(op jj.Operation) tea.Msg { return MovedMsg{Operation: op, Ago: m.list.Cursor()} })
	}

	switch name {
	case "esc", "q", "o":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		return m.restore()
	case "p":
		return m.emit(func(op jj.Operation) tea.Msg { return ShowMsg{Operation: op} })
	case "u":
		return m.revert()
	}

	return nil
}

// jumpKey answers the jump keys: ' labels the rows on screen, ctrl+o and ctrl+i (or tab, which
// most terminals send for it) walk the jump list that g, G, and a label fill.
func (m *Model) jumpKey(name string, before int) (tea.Cmd, bool) {
	switch name {
	case "'":
		if !m.list.StartLabels() {
			m.list.Warn("nothing on screen to jump to")
		}

		return nil, true
	case "ctrl+o":
		if !m.list.JumpBack() {
			m.list.Warn("no jump to go back to yet: g, G, and ' each leave one")
		}
	case "ctrl+i", "tab":
		if !m.list.JumpForward() {
			m.list.Warn("nothing to go forward to: ctrl+o goes back first")
		}
	default:
		return nil, false
	}

	return m.moved(m.list.Cursor() != before), true
}

// moved says the cursor moved, so the log behind redraws at the operation now under it.
func (m *Model) moved(did bool) tea.Cmd {
	if !did {
		return nil
	}

	return m.emit(func(op jj.Operation) tea.Msg { return MovedMsg{Operation: op, Ago: m.list.Cursor()} })
}

func (m *Model) emit(build func(jj.Operation) tea.Msg) tea.Cmd {
	op, ok := m.list.Selected()
	if !ok {
		return nil
	}

	return func() tea.Msg { return build(op) }
}

// restore refuses the operation the repository is already at, which would report a move that never
// happened. The list stays open under the confirmation it leads to, so a no comes back to it.
func (m *Model) restore() tea.Cmd {
	op, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if op.Current {
		m.list.Warn("already at this operation")

		return nil
	}

	return func() tea.Msg { return RestoreMsg{Operation: op} }
}

// revert refuses the current operation, which has nothing after it to keep and would only be a
// slower undo.
func (m *Model) revert() tea.Cmd {
	return m.guarded("press u to undo the current operation", func(op jj.Operation) tea.Msg {
		return RevertMsg{Operation: op}
	})
}

func (m *Model) guarded(warning string, build func(jj.Operation) tea.Msg) tea.Cmd {
	op, ok := m.list.Selected()
	if !ok {
		return nil
	}

	if op.Current {
		m.list.Warn(warning)

		return nil
	}

	m.Close()

	return func() tea.Msg { return build(op) }
}

// View renders the operations, newest first, as jj lists them.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "enter", What: "restore"},
		{Key: "p", What: "diff"},
		{Key: "u", What: "revert"},
		{Key: "esc", What: "cancel"},
	})
}

// currentMarker flags the operation the repository is at; other rows keep the same width in spaces
// so the ids line up.
const currentMarker = "@ "

// ageWidth keeps the ids in a column, since jj's ages run from "2 minutes ago" to "3 months ago".
const ageWidth = 14

func (m Model) row(op jj.Operation, selected bool) listview.Row {
	marker := strings.Repeat(" ", len(currentMarker))
	style := m.skin.Body

	if op.Current {
		marker, style = currentMarker, m.skin.WorkingCopy
	}

	if selected {
		style = m.skin.Selected
	}

	text := marker + table.Pad(op.Age, ageWidth, table.AlignLeft) + " " + op.ID + " " + op.Description
	if op.Args == "" || selected {
		return listview.Row{Text: joinArgs(text, op.Args), Style: style}
	}

	// The command line is what a reader recognizes an operation by, the description what jj calls it.
	return listview.Row{Drawn: style.Render(text) + m.skin.Muted.Render(argsGap+op.Args)}
}

// argsGap sets the command line apart from the description it follows.
const argsGap = "  · "

func joinArgs(text, args string) string {
	if args == "" {
		return text
	}

	return text + argsGap + args
}
