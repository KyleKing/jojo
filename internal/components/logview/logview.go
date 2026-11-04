package logview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
)

// Styles is the vocabulary the log renders in. The caller owns it, so this
// package never learns a palette.
type Styles struct {
	Graph     lipgloss.Style
	Change    lipgloss.Style
	Bookmark  lipgloss.Style
	Desc      lipgloss.Style
	Added     lipgloss.Style
	Removed   lipgloss.Style
	Files     lipgloss.Style
	Author    lipgloss.Style
	Age       lipgloss.Style
	Selected  lipgloss.Style
	Immutable lipgloss.Style
	Conflict  lipgloss.Style
	Divergent lipgloss.Style
	Working   lipgloss.Style
	Checked   lipgloss.Style
	Header    lipgloss.Style
	Match     lipgloss.Style
}

// SelectedMsg announces the row under the cursor so the forge pane and the
// detail view can follow it.
type SelectedMsg struct {
	Revision *jj.Revision
}

// Model is the log list: rows, a cursor that skips the graph-only lines, and a
// multi-select set for batch actions.
type Model struct {
	styles        Styles
	checked       map[string]bool
	searchPattern string
	rows          []jj.LogRow
	matchRows     []int
	cursor        int
	top           int
	width         int
	height        int
}

// New returns an empty log. Call SetRows once jj has answered.
func New(styles Styles) Model {
	return Model{styles: styles, checked: map[string]bool{}}
}

// SetRows replaces the log, holding the cursor on the same change id when that
// change survives the new revset.
func (m *Model) SetRows(rows []jj.LogRow) {
	held := ""
	if rev := m.Selected(); rev != nil {
		held = rev.ChangeID
	}

	m.rows = rows
	m.cursor = 0

	if held != "" {
		for i, row := range rows {
			if row.Revision != nil && row.Revision.ChangeID == held {
				m.cursor = i

				break
			}
		}
	}

	if len(m.rows) > 0 && m.rows[m.cursor].Revision == nil {
		m.moveCursor(1)
	}

	m.recomputeMatches()
	m.scrollIntoView()
}

// SetSize records the space the log renders into.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.scrollIntoView()
}

// Selected returns the revision under the cursor, or nil when the log is empty.
func (m Model) Selected() *jj.Revision {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}

	return m.rows[m.cursor].Revision
}

// AtEnd reports whether no revision sits below the cursor, which is how the caller learns that
// scrolling ran out of the rows it has loaded.
func (m Model) AtEnd() bool { return len(m.rows) > 0 && m.nextRevision(m.cursor, 1) == m.cursor }

// Checked returns the change ids marked for a batch action, in log order.
func (m Model) Checked() []string {
	ids := make([]string, 0, len(m.checked))

	for _, row := range m.rows {
		if row.Revision != nil && m.checked[row.Revision.ChangeID] {
			ids = append(ids, row.Revision.ChangeID)
		}
	}

	return ids
}

// SetSearch records pattern and moves the cursor to the first matching row at
// or after its current position, wrapping to the first match when there is
// none after it. An empty pattern clears the search. It returns the match
// count and the cursor's 1-based position in it, or 0 when nothing matched.
func (m *Model) SetSearch(pattern string) (int, int) {
	m.searchPattern = pattern
	m.recomputeMatches()

	return m.landOnMatch()
}

// StepMatch moves the cursor to the next (delta 1) or previous (delta -1)
// match, wrapping at both ends: stepping through results is the one place a
// user expects to come back around, unlike the clamped movement elsewhere.
// It returns the match count and the cursor's new 1-based position in it.
func (m *Model) StepMatch(delta int) (int, int) {
	n := len(m.matchRows)
	if n == 0 {
		return 0, 0
	}

	pos := m.currentMatchPos()
	if pos < 0 {
		pos = 0
	} else {
		pos = ((pos+delta)%n + n) % n
	}

	m.cursor = m.matchRows[pos]
	m.scrollIntoView()

	return n, pos + 1
}

func (m *Model) landOnMatch() (int, int) {
	if len(m.matchRows) == 0 {
		return 0, 0
	}

	for i, rowIdx := range m.matchRows {
		if rowIdx >= m.cursor {
			m.cursor = rowIdx
			m.scrollIntoView()

			return len(m.matchRows), i + 1
		}
	}

	m.cursor = m.matchRows[0]
	m.scrollIntoView()

	return len(m.matchRows), 1
}

func (m Model) currentMatchPos() int {
	for i, rowIdx := range m.matchRows {
		if rowIdx == m.cursor {
			return i
		}
	}

	return -1
}

func (m *Model) recomputeMatches() {
	if m.searchPattern == "" {
		m.matchRows = nil

		return
	}

	matches := make([]int, 0, len(m.rows))

	for i, row := range m.rows {
		if row.Revision != nil && matchesSearch(*row.Revision, m.searchPattern) {
			matches = append(matches, i)
		}
	}

	m.matchRows = matches
}

func matchesSearch(rev jj.Revision, pattern string) bool {
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

// Update handles the log's own navigation. The parent routes everything else.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	before := m.cursor

	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-m.page())
	case "pgdown", "ctrl+d":
		m.moveCursor(m.page())
	case "g", "home":
		m.cursor = 0
		m.snapToRevision(1)
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.snapToRevision(-1)
	case "@":
		m.jumpToWorkingCopy()
	case "space":
		m.toggleChecked()
	default:
		return nil
	}

	m.scrollIntoView()

	if m.cursor == before {
		return nil
	}

	selected := m.Selected()

	return func() tea.Msg { return SelectedMsg{Revision: selected} }
}

func (m *Model) toggleChecked() {
	rev := m.Selected()
	if rev == nil {
		return
	}

	if m.checked[rev.ChangeID] {
		delete(m.checked, rev.ChangeID)

		return
	}

	m.checked[rev.ChangeID] = true
}

func (m *Model) jumpToWorkingCopy() {
	for i, row := range m.rows {
		if row.Revision != nil && row.Revision.WorkingCopy {
			m.cursor = i

			return
		}
	}
}

// moveCursor steps by whole revisions, so the graph-only connector lines
// between two rows are never selectable.
func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}

	step := 1
	if delta < 0 {
		step = -1
	}

	for range abs(delta) {
		next := m.nextRevision(m.cursor, step)
		if next == m.cursor {
			break
		}

		m.cursor = next
	}
}

func (m *Model) nextRevision(from, step int) int {
	for i := from + step; i >= 0 && i < len(m.rows); i += step {
		if m.rows[i].Revision != nil {
			return i
		}
	}

	return from
}

func (m *Model) snapToRevision(step int) {
	if len(m.rows) == 0 {
		m.cursor = 0

		return
	}

	m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
	if m.rows[m.cursor].Revision == nil {
		m.cursor = m.nextRevision(m.cursor, step)
	}
}

func (m Model) page() int {
	if m.height <= 1 {
		return 1
	}

	return m.height - 1
}

func (m *Model) scrollIntoView() {
	if m.height <= 0 {
		return
	}

	if m.cursor < m.top {
		m.top = m.cursor
	}

	if m.cursor >= m.top+m.height {
		m.top = m.cursor - m.height + 1
	}

	m.top = clamp(m.top, 0, max(0, len(m.rows)-m.height))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// View renders the visible window. It returns exactly height lines so the
// panes below it never shift.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	graphWidth := GraphWidth(m.rows)
	layout := Fit(m.width, graphWidth)
	lines := make([]string, 0, m.height)

	for i := m.top; i < len(m.rows) && len(lines) < m.height; i++ {
		lines = append(lines, m.line(m.rows[i], layout, graphWidth, i == m.cursor))
	}

	for len(lines) < m.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (m Model) line(row jj.LogRow, layout table.Layout, graphWidth int, selected bool) string {
	if row.Revision == nil {
		return m.styles.Graph.Render(row.Graph.Graph)
	}

	graph := m.styles.Graph.Render(table.Pad(strings.TrimRight(row.Revision.Graph, " "), graphWidth, table.AlignLeft))
	body := m.cells(*row.Revision, layout)
	gutter := m.gutter(*row.Revision)

	switch {
	case selected:
		return m.styles.Selected.Render(graph + gutter + body)
	case m.searchPattern != "" && matchesSearch(*row.Revision, m.searchPattern):
		return m.styles.Match.Render(graph + gutter + body)
	default:
		return graph + gutter + body
	}
}

// gutter spends the space between the graph and the columns on the multi-select
// marker, so a checked row reads without a column of its own.
func (m Model) gutter(rev jj.Revision) string {
	if !m.checked[rev.ChangeID] {
		return strings.Repeat(" ", table.Gutter)
	}

	return m.styles.Checked.Render(CheckMark) + strings.Repeat(" ", table.Gutter-1)
}

func (m Model) cells(rev jj.Revision, layout table.Layout) string {
	text := Cells(rev)
	rendered := make([]string, 0, len(layout.Columns))

	for _, col := range layout.Columns {
		width := layout.Width(col.Key)

		if col.Key == ColSize && !rev.Conflict && !rev.Immutable {
			rendered = append(rendered, m.sizeCell(rev, width))

			continue
		}

		cell := table.PadTrim(text[col.Key], width, col.Align, col.Trim)
		rendered = append(rendered, m.style(rev, col.Key).Render(cell))
	}

	return table.Join(rendered)
}

// sizeCell colors the added and removed counts apart from the file count,
// then pads to width by display cells because the styled string carries escape
// bytes that table.Pad would count.
func (m Model) sizeCell(rev jj.Revision, width int) string {
	plain := size(rev)
	if plain == "" || lipgloss.Width(plain) > width {
		return table.PadTrim(plain, width, table.AlignRight, table.TrimRight)
	}

	files, rest, _ := strings.Cut(plain, " ")
	added, removed, _ := strings.Cut(rest, " ")

	styled := m.styles.Files.Render(files) + " " +
		m.styles.Added.Render(added) + " " +
		m.styles.Removed.Render(removed)

	return strings.Repeat(" ", width-lipgloss.Width(plain)) + styled
}

// style picks a cell's look from the revision's state first, because an
// immutable or conflicted row has to read as one however its columns are
// colored.
func (m Model) style(rev jj.Revision, key string) lipgloss.Style {
	switch {
	case rev.Conflict:
		return m.styles.Conflict
	case rev.Divergent:
		return m.styles.Divergent
	case rev.Immutable:
		return m.styles.Immutable
	}

	switch key {
	case ColChange:
		if rev.WorkingCopy {
			return m.styles.Working
		}

		return m.styles.Change
	case ColBookmarks:
		return m.styles.Bookmark
	case ColSize:
		return m.styles.Files
	case ColAuthor:
		return m.styles.Author
	case ColAge:
		return m.styles.Age
	default:
		return m.styles.Desc
	}
}
