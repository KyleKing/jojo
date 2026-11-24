// Package trailview is `gm`: every jj command this session ran, newest first, as aragonite's trail
// draws it. It is how the TUI teaches the command line behind a key, and what a bug report quotes:
// the exact command, how it ended, all of its stderr, a line to paste into a shell, and the way back
// from a write.
package trailview

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/trail"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// ShowMsg asks for a call's whole stderr, which a row has room for one line of.
type ShowMsg struct{ Call trail.Call }

// UndoMsg asks to undo a write: the newest with jj undo, an older one by reverting its operation.
type UndoMsg struct {
	Call   trail.Call
	Newest bool
}

// CopiedMsg says a call's shell line went to the clipboard.
type CopiedMsg struct{ Line string }

// CancelledMsg is sent when the list is dismissed.
type CancelledMsg struct{}

// Model is the list's state: the calls it was opened with, which of them the cursor is on, and
// whether reads are listed beside the writes.
type Model struct {
	skin   ui.Skin
	log    *trail.Trail
	calls  []trail.Call
	cursor int
	reads  bool
	open   bool
}

// New returns a closed list drawn in skin.
func New(skin ui.Skin) Model { return Model{skin: skin} }

// Open lists log's calls, writes only until r shows the reads, with the cursor on the newest
// failure when failed is set and on the newest call otherwise.
func (m *Model) Open(log *trail.Trail, failed bool) {
	m.log, m.open, m.reads, m.cursor = log, true, false, 0
	m.load()

	if !failed {
		return
	}

	for i := range m.calls {
		if m.calls[i].Failed() {
			m.cursor = i

			return
		}
	}
}

// load reads the calls again, keeping the cursor in range.
func (m *Model) load() {
	m.calls = m.log.Calls(!m.reads)
	m.cursor = min(max(0, m.cursor), max(0, len(m.calls)-1))
}

// Close hides the list.
func (m *Model) Close() { m.open = false }

// IsOpen reports whether the list is taking keys.
func (m Model) IsOpen() bool { return m.open }

// Selected is the call under the cursor, false when the list is empty.
func (m Model) Selected() (trail.Call, bool) {
	if m.cursor >= len(m.calls) {
		return trail.Call{}, false
	}

	return m.calls[m.cursor], true
}

// newestWrite reports whether the call under the cursor is the newest write, which jj undo reverses.
func (m Model) newestWrite() bool {
	newest, ok := m.log.Last(func(c *trail.Call) bool { return c.Write && !c.Failed() })
	sel, has := m.Selected()

	return ok && has && newest.Start.Equal(sel.Start) && newest.Shell() == sel.Shell()
}

// Update moves the cursor, toggles reads, and answers enter, Y, and u with the message the parent
// acts on.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	switch key.String() {
	case "esc", "q":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, len(m.calls)-1))
	case "k", "up":
		m.cursor = max(0, m.cursor-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.calls)-1)
	case "r":
		m.reads = !m.reads
		m.load()
	case "enter":
		return m.answer(func(c trail.Call) tea.Msg { return ShowMsg{Call: c} })
	case "Y":
		return m.answer(func(c trail.Call) tea.Msg { return CopiedMsg{Line: c.Shell()} })
	case "u":
		return m.undo()
	}

	return nil
}

// answer sends what the call under the cursor asks for, nothing when the list is empty.
func (m Model) answer(msg func(trail.Call) tea.Msg) tea.Cmd {
	c, ok := m.Selected()
	if !ok {
		return nil
	}

	return func() tea.Msg { return msg(c) }
}

// undo answers u on a write that went through.
func (m Model) undo() tea.Cmd {
	c, ok := m.Selected()
	if !ok || !c.Write || c.Failed() {
		return nil
	}

	newest := m.newestWrite()

	return func() tea.Msg { return UndoMsg{Call: c, Newest: newest} }
}

// Hints is what the list's own footer offers.
func (m Model) Hints() []keyhint.Hint {
	reads := "show reads"
	if m.reads {
		reads = "writes only"
	}

	return []keyhint.Hint{
		{Key: "enter", What: "stderr"},
		{Key: "Y", What: "copy as shell"},
		{Key: "u", What: "undo"},
		{Key: "r", What: reads},
		{Key: "esc", What: "close"},
	}
}

// minRows is the least the list draws in: a rule, a row, and the hints.
const minRows = 3

// View draws the list centered in a width by height frame: a labeled rule, the calls around the
// cursor as the trail draws them (a failure takes a second row for its stderr), and the keys.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	inner := overlay.ContentWidth(width, styles)
	rows := overlay.ContentHeight(height, styles)

	if inner < table.Gutter || rows < minRows {
		return ui.TooSmall(width, height)
	}

	lines := []string{m.skin.Rule("jj commands · "+m.countText(), "", inner)}
	body := rows - 2 //nolint:mnd // the rule and the hints

	if len(m.calls) == 0 {
		lines = append(lines, m.skin.Muted.Render(table.Truncate("nothing has run yet; r shows reads", inner)))
	}

	lines = append(lines, m.window(inner, body)...)
	for len(lines) < rows-1 {
		lines = append(lines, "")
	}

	lines = append(lines, ui.FitHints(m.skin.KeyHintStyles(), m.Hints(), inner))

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

// window is the drawn calls that fit body rows, scrolled so the cursor's call is whole.
func (m Model) window(width, body int) []string {
	drawn := make([][]string, len(m.calls))
	for i := range m.calls {
		sel := -1
		if i == m.cursor {
			sel = 0
		}

		drawn[i] = trail.Lines(m.skin.Skin, m.calls[i:i+1], width, sel)
	}

	start := 0

	for start < m.cursor && rowsFrom(drawn, start, m.cursor) > body {
		start++
	}

	var out []string

	for i := start; i < len(drawn) && len(out)+len(drawn[i]) <= body; i++ {
		out = append(out, drawn[i]...)
	}

	return out
}

// rowsFrom is how many rows the drawn calls from first through last take.
func rowsFrom(drawn [][]string, first, last int) int {
	n := 0
	for i := first; i <= last; i++ {
		n += len(drawn[i])
	}

	return n
}

// countText counts what the list shows: writes until r shows the reads too.
func (m Model) countText() string {
	noun := "write"
	if m.reads {
		noun = "call"
	}

	if len(m.calls) != 1 {
		noun += "s"
	}

	return strconv.Itoa(len(m.calls)) + " " + noun
}
