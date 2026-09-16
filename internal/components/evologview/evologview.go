// Package evologview is the evolution timeline and interdiff picker overlay,
// rendering the versions a change passed through (`jj evolog`). The parent
// passes the entries it already fetched, so this never calls jj.
package evologview

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
	warning  string
	entries  []jj.Evolution
	cursor   int
	markIdx  int
	markSet  bool
	open     bool
}

// New returns a closed view drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the timeline for changeID from entries, newest first as
// Client.Evolog returns them, with the cursor on the newest and no mark set.
//
//nolint:unparam // matches destpicker.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(changeID string, entries []jj.Evolution) tea.Cmd {
	m.changeID = changeID
	m.warning = ""
	m.markSet = false
	m.open = true
	m.entries = entries
	m.cursor = 0

	return nil
}

// ChangeID names the change the open timeline belongs to.
func (m Model) ChangeID() string {
	return m.changeID
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

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)

		return nil
	case "down", "j":
		m.moveCursor(1)

		return nil
	case "g", "home":
		m.jumpTo(0)

		return nil
	case "G", "end":
		m.jumpTo(len(m.entries) - 1)

		return nil
	case "enter":
		return m.show()
	case "space":
		m.toggleMark()

		return nil
	case "d":
		return m.interdiff()
	case "r":
		return m.restore()
	case "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	return nil
}

func (m *Model) moveCursor(delta int) {
	m.jumpTo(m.cursor + delta)
}

func (m *Model) jumpTo(idx int) {
	if len(m.entries) == 0 {
		return
	}

	m.cursor = clamp(idx, 0, len(m.entries)-1)
	m.warning = ""
}

func (m *Model) toggleMark() {
	if m.markSet && m.markIdx == m.cursor {
		m.markSet = false

		return
	}

	m.markSet = true
	m.markIdx = m.cursor
}

func (m *Model) show() tea.Cmd {
	id := m.entries[m.cursor].CommitID

	return func() tea.Msg { return ShowMsg{CommitID: id} }
}

func (m *Model) restore() tea.Cmd {
	id := m.entries[m.cursor].CommitID

	return func() tea.Msg { return RestoreMsg{CommitID: id} }
}

func (m *Model) interdiff() tea.Cmd {
	if !m.markSet || m.markIdx == m.cursor {
		m.warning = markMsg

		return nil
	}

	// The entries run newest first, so the larger index is the older version and has to be From.
	from, to := max(m.markIdx, m.cursor), min(m.markIdx, m.cursor)

	fromID, toID := m.entries[from].CommitID, m.entries[to].CommitID

	return func() tea.Msg { return InterdiffMsg{From: fromID, To: toID} }
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of entries to render, keeping
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

// View renders the title, an optional warning, one line per entry, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{m.skin.Title.Render("evolution of " + m.changeID)}
	if m.warning != "" {
		lines = append(lines, m.skin.Error.Render(m.warning))
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "enter", What: "view diff"},
		{Key: "space", What: "mark"},
		{Key: "d", What: "interdiff"},
		{Key: "r", What: "restore"},
		{Key: "esc", What: "cancel"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.entries) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.entries), body)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(i, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.entries), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) renderRow(i, width int) string {
	text := table.Truncate(rowText(len(m.entries)-i, m.entries[i]), width)
	if m.markSet && m.markIdx == i {
		text = "* " + text
	} else {
		text = "  " + text
	}

	if i == m.cursor {
		return m.skin.Selected.Render(text)
	}

	return m.skin.Body.Render(text)
}

// rowText numbers from the oldest version, so a version keeps its number as newer ones arrive.
func rowText(number int, e jj.Evolution) string {
	description := e.Description
	if description == "" {
		description = noDescription
	}

	return fmt.Sprintf("[%d] %s  %s  %s", number, e.Age, e.Operation, description)
}

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
