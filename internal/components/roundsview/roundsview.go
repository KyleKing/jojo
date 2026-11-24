// Package roundsview is the rounds box H opens: the versions a change passed through, folded into
// rounds, newest first, to choose the one the docked diff shows. The parent reads the evolution and
// owns the dock, so this package only draws and picks.
package roundsview

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/rounds"
	"github.com/kyleking/jojo/internal/ui"
)

// PickedMsg asks the docked diff to show the round at Index, which counts from the oldest.
type PickedMsg struct {
	Index int
}

// WholeMsg asks the docked diff to show the change as it stands rather than one round of it.
type WholeMsg struct{}

// RestoreMsg asks for the change's content to be put back to the round at Index as it ended.
type RestoreMsg struct {
	Round rounds.Round
}

// CancelledMsg is sent when the box closes without a pick.
type CancelledMsg struct{}

// entry is one row: a round and where it sits, or the change as a whole above them all.
type entry struct {
	round rounds.Round
	index int
	whole bool
}

// Model is the box's state.
type Model struct {
	skin  ui.Skin
	list  listview.Model[entry]
	total int
}

// New returns a closed box drawn in skin.
func New(skin ui.Skin) Model { return Model{skin: skin, list: listview.New[entry](skin)} }

// Open shows the rounds of changeID, oldest first as rounds.Group returns them, out of total
// versions, with the cursor on the round the dock shows (-1 for the whole change).
func (m *Model) Open(changeID string, list []rounds.Round, total, showing int) {
	entries := []entry{{whole: true, index: -1}}

	for i := len(list) - 1; i >= 0; i-- {
		entries = append(entries, entry{round: list[i], index: i})
	}

	cursor := 0
	if showing >= 0 {
		cursor = len(list) - showing
	}

	m.total = total
	m.list.Open("rounds of "+changeID, entries, cursor)
}

// IsOpen reports whether the box takes keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Close hides the box.
func (m *Model) Close() { m.list.Close() }

// Update handles one key while the box is open.
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
		e, ok := m.list.Selected()
		if !ok {
			return nil
		}

		m.Close()

		if e.whole {
			return func() tea.Msg { return WholeMsg{} }
		}

		return func() tea.Msg { return PickedMsg{Index: e.index} }
	case "R":
		e, ok := m.list.Selected()
		if !ok || e.whole {
			return nil
		}

		m.Close()

		return func() tea.Msg { return RestoreMsg{Round: e.round} }
	case "esc", "q", "H":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

// View draws the box in a width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "j k", What: "move"},
		{Key: "enter", What: "show in the dock"},
		{Key: "R", What: "restore to it"},
		{Key: "esc", What: "close"},
	})
}

// Column widths of a row.
const (
	spanWidth = 7
	verbWidth = 28
	ageWidth  = 15
)

func (m Model) row(e entry, selected bool) listview.Row {
	if e.whole {
		return listview.Row{Text: "  now     the change as it stands", Style: m.face(selected, m.skin.Body)}
	}

	r := e.round

	span := "v" + strconv.Itoa(r.First)
	if r.Last != r.First {
		span += "–" + strconv.Itoa(r.Last)
	}

	face := m.skin.Body
	if r.Moved {
		face = m.skin.Muted
	}

	text := "  " + table.Pad(span, spanWidth, table.AlignLeft) + " " +
		table.PadTrim(r.Label(), verbWidth, table.AlignLeft, table.TrimRight) + " " +
		table.PadTrim(r.Age, ageWidth, table.AlignLeft, table.TrimRight) + " " + r.Size()

	return listview.Row{Text: text, Style: m.face(selected, face)}
}

func (m Model) face(selected bool, face lipgloss.Style) lipgloss.Style {
	if selected {
		return m.skin.Selected
	}

	return face
}
