// Package evologview is the evolution timeline and interdiff picker overlay,
// rendering the versions a change passed through (`jj evolog`). The parent
// passes the entries it already fetched, so this never calls jj.
package evologview

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// noDescription is shown in place of an entry's blank description.
const noDescription = "(no description set)"

// markMsg is shown when d is pressed without two distinct marked versions.
const markMsg = "mark a second version with space"

// ShowMsg is sent when Enter asks to view a version's diff.
type ShowMsg struct {
	CommitID string
}

// InterdiffMsg is sent when d asks for the diff between two versions. From is
// always the older of the marked and cursor entries, To the newer.
type InterdiffMsg struct {
	From string
	To   string
}

// RestoreMsg is sent when r asks to restore a version.
type RestoreMsg struct {
	CommitID string
}

// CancelledMsg is sent when Esc or q dismisses the view.
type CancelledMsg struct{}

// Model is the timeline's state: the entries for one change, newest first as
// jj lists them, the cursor into them, and at most one mark for an interdiff.
type Model struct {
	skin     ui.Skin
	changeID string
	list     listview.Model[jj.Evolution]
	markIdx  int
	markSet  bool
}

// New returns a closed view drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[jj.Evolution](skin)}
}

// Open shows the timeline for changeID from entries, newest first as
// Client.Evolog returns them, with the cursor on the newest and no mark set.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(changeID string, entries []jj.Evolution) tea.Cmd {
	m.changeID = changeID
	m.markSet = false
	m.list.Open("evolution of "+changeID, entries, 0)

	return nil
}

// ChangeID names the change the open timeline belongs to.
func (m Model) ChangeID() string {
	return m.changeID
}

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
	if m.list.MoveKey(name) {
		return nil
	}

	switch name {
	case "enter":
		return m.emit(func(e jj.Evolution) tea.Msg { return ShowMsg{CommitID: e.CommitID} })
	case "space":
		m.toggleMark()

		return nil
	case "d":
		return m.interdiff()
	case "r":
		return m.emit(func(e jj.Evolution) tea.Msg { return RestoreMsg{CommitID: e.CommitID} })
	case "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) emit(build func(jj.Evolution) tea.Msg) tea.Cmd {
	e, ok := m.list.Selected()
	if !ok {
		return nil
	}

	return func() tea.Msg { return build(e) }
}

func (m *Model) toggleMark() {
	cursor := m.list.Cursor()
	if m.markSet && m.markIdx == cursor {
		m.markSet = false

		return
	}

	m.markSet = true
	m.markIdx = cursor
}

func (m *Model) interdiff() tea.Cmd {
	cursor := m.list.Cursor()
	if !m.markSet || m.markIdx == cursor {
		m.list.Warn(markMsg)

		return nil
	}

	// The entries run newest first, so the larger index is the older version and has to be From.
	items := m.list.Items()
	from, to := max(m.markIdx, cursor), min(m.markIdx, cursor)
	fromID, toID := items[from].CommitID, items[to].CommitID

	return func() tea.Msg { return InterdiffMsg{From: fromID, To: toID} }
}

// View renders the title, an optional warning, one line per entry, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "enter", What: "view diff"},
		{Key: "space", What: "mark"},
		{Key: "d", What: "interdiff"},
		{Key: "r", What: "restore"},
		{Key: "esc", What: "cancel"},
	})
}

func (m Model) row(e jj.Evolution, selected bool) listview.Row {
	items := m.list.Items()
	idx := indexOf(items, e)

	marker := "  "
	if m.markSet && idx == m.markIdx {
		marker = "* "
	}

	style := m.skin.Body
	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: marker + rowText(len(items)-idx, e), Style: style}
}

func indexOf(items []jj.Evolution, e jj.Evolution) int {
	for i, it := range items {
		if it.CommitID == e.CommitID {
			return i
		}
	}

	return -1
}

// rowText numbers from the oldest version, so a version keeps its number as newer ones arrive.
func rowText(number int, e jj.Evolution) string {
	description := e.Description
	if description == "" {
		description = noDescription
	}

	return fmt.Sprintf("[%d] %s  %s  %s", number, e.Age, e.Operation, description)
}
