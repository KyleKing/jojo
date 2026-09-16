// Package destpicker is a scrollable revision picker modal, used to choose the
// destination of a jj squash --into or a jj rebase --onto. The parent passes
// the rows it already has, so this never calls jj.
package destpicker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// PickedMsg is sent when Enter confirms a mutable revision.
type PickedMsg struct {
	Title    string
	Revision jj.Revision
}

// CancelledMsg is sent when Esc dismisses the picker without picking one.
type CancelledMsg struct{}

// Option configures a Model at construction.
type Option func(*Model)

// Model is the picker's state: the candidate revisions, the live filter over
// them, and the cursor into the filtered list.
type Model struct {
	skin      ui.Skin
	title     string
	warning   string
	filter    string
	revisions []jj.Revision
	matches   []jj.Revision
	cursor    int
	open      bool
}

// New returns a closed picker drawn in skin.
func New(skin ui.Skin, opts ...Option) Model {
	m := Model{skin: skin}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Open shows the picker over rows, omitting graph-only lines and the
// revision whose change id is exclude, with the filter cleared.
//
//nolint:unparam // matches prompt.Model.Open's signature; nothing here needs a command yet.
func (m *Model) Open(title string, rows []jj.LogRow, exclude string) tea.Cmd {
	m.title = title
	m.warning = ""
	m.filter = ""
	m.open = true

	revisions := make([]jj.Revision, 0, len(rows))

	for _, row := range rows {
		if row.Revision == nil || row.Revision.ChangeID == exclude {
			continue
		}

		revisions = append(revisions, *row.Revision)
	}

	m.revisions = revisions
	m.recomputeMatches()

	return nil
}

// Close hides the picker.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the picker is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the picker is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	return m.handleKey(key)
}

// handleKey reserves j and k for moving the cursor, matching every other list
// in this app, so a description containing either letter is only reachable by
// filtering on other text.
func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		return m.pick()
	case "up", "k":
		m.moveCursor(-1)

		return nil
	case "down", "j":
		m.moveCursor(1)

		return nil
	case "backspace":
		m.setFilter(trimLastRune(m.filter))

		return nil
	}

	if key.Text != "" {
		m.setFilter(m.filter + key.Text)
	}

	return nil
}

func (m *Model) pick() tea.Cmd {
	rev, ok := m.selected()
	if !ok {
		return nil
	}

	if rev.Immutable {
		m.warning = "cannot pick an immutable revision"

		return nil
	}

	title := m.title
	m.Close()

	return func() tea.Msg { return PickedMsg{Title: title, Revision: rev} }
}

func (m Model) selected() (jj.Revision, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return jj.Revision{}, false
	}

	return m.matches[m.cursor], true
}

func (m *Model) moveCursor(delta int) {
	if len(m.matches) == 0 {
		return
	}

	n := len(m.matches)
	m.cursor = ((m.cursor+delta)%n + n) % n
}

func (m *Model) setFilter(s string) {
	m.filter = s
	m.warning = ""
	m.recomputeMatches()
}

func (m *Model) recomputeMatches() {
	pattern := m.filter

	m.matches = filter.Keep(m.revisions, func(rev jj.Revision) bool {
		return matchesFilter(rev, pattern)
	})
	m.cursor = 0
}

func matchesFilter(rev jj.Revision, pattern string) bool {
	if pattern == "" {
		return true
	}

	if filter.Match(pattern, rev.ChangeID) || filter.Match(pattern, rev.Description) {
		return true
	}

	for _, bookmark := range rev.Bookmarks {
		if filter.Match(pattern, bookmark) {
			return true
		}
	}

	return false
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}

	return string(runes[:len(runes)-1])
}

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of matches to render, keeping
// cursor inside it, when body rows are available for total items.
func scrollWindow(cursor, total, body int) (int, int) {
	if body <= 0 || total <= body {
		return 0, total
	}

	start := clamp(cursor-body/scrollCenterDivisor, 0, total-body)

	return start, start + body
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// footerRows is how much of the frame a clipped list gives up to the
// scroll-position line.
const footerRows = 1

// View renders the title, filter, matching rows, a scroll-position footer
// when clipped, and a key hint line, centered in a width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{
		m.skin.Title.Render(m.title),
		m.skin.Body.Render("/" + m.filter),
	}

	if m.warning != "" {
		lines = append(lines, m.skin.Error.Render(m.warning))
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "enter", What: "pick"},
		{Key: "esc", What: "cancel"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.matches) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.matches), body)
	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.matches[i], i == m.cursor, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.matches), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
}

func (m Model) renderRow(rev jj.Revision, selected bool, width int) string {
	text := table.Truncate(rowText(rev), width)

	if selected {
		return m.skin.Selected.Render(text)
	}

	if rev.Immutable {
		return m.skin.Immutable.Render(text)
	}

	return m.skin.Body.Render(text)
}

func rowText(rev jj.Revision) string {
	var b strings.Builder

	b.WriteString(rev.ChangeID)

	if len(rev.Bookmarks) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(rev.Bookmarks, ","))
	}

	if rev.Description != "" {
		b.WriteString(" ")
		b.WriteString(rev.Description)
	}

	return b.String()
}

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
