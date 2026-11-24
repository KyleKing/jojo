package logview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/skin"
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
	// Mark and MarkMuted draw the chips a preview pins on a row: Mark for what the command is about
	// (the destination, what moves), MarkMuted for what it touches on the way (a rewritten descendant).
	Mark      lipgloss.Style
	MarkMuted lipgloss.Style
	// Label draws the hint label a label jump (') puts in a row's gutter.
	Label lipgloss.Style
	// Unread draws a row's count of hunks not yet read, at the end of its description.
	Unread lipgloss.Style
	// Select lays the selection under one cell's face, keeping what the face says beyond color in
	// reverse video; nil lays Selected over the finished row instead.
	Select func(lipgloss.Style) lipgloss.Style
}

// Mark is a chip drawn ahead of a row's description, such as "◂ onto" on the destination a picker
// has under its cursor.
type Mark struct {
	Text  string
	Muted bool
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
	marks         map[string]Mark
	labels        map[string]string
	unread        map[string]string
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
	return Model{styles: styles, checked: map[string]bool{}, unread: map[string]string{}}
}

// SetUnread is the unread count a revision's row ends its description with ("◌3"), "" for none:
// its diff has not been read yet, or everything in it has.
func (m *Model) SetUnread(changeID, label string) {
	if label == "" {
		delete(m.unread, changeID)

		return
	}

	m.unread[changeID] = label
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

// SetLabels draws a label jump's hints in the gutter of each row named, by change id. Nil clears them.
func (m *Model) SetLabels(labels map[string]string) { m.labels = labels }

// Visible is the change ids of the revisions on screen, top to bottom, which a label jump labels.
func (m Model) Visible() []string {
	window := m.Window(0)
	out := make([]string, 0, len(window))

	for _, rev := range window {
		out = append(out, rev.ChangeID)
	}

	return out
}

// SetMarks pins chips on rows by change id, replacing whatever was pinned before. Nil clears them.
func (m *Model) SetMarks(marks map[string]Mark) { m.marks = marks }

// Focus puts the cursor on the revision with changeID and reports whether the log holds one.
func (m *Model) Focus(changeID string) bool {
	for i, row := range m.rows {
		if row.Revision != nil && row.Revision.ChangeID == changeID {
			m.cursor = i
			m.scrollIntoView()

			return true
		}
	}

	return false
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

// Window is the revisions on screen plus the next ahead rows below them, which is what a caller
// filling in per-row detail reads first so the first frame and the next scroll both have it.
func (m Model) Window(ahead int) []*jj.Revision {
	end := min(len(m.rows), m.top+max(m.height, 1)+ahead)
	out := make([]*jj.Revision, 0, max(0, end-m.top))

	for i := m.top; i < end; i++ {
		if rev := m.rows[i].Revision; rev != nil {
			out = append(out, rev)
		}
	}

	return out
}

// ClearChecked unchecks every revision, which a finished batch action and Esc both do, so a check
// never outlives the action it was made for.
func (m *Model) ClearChecked() { clear(m.checked) }

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

	m.cursor = clamp(m.cursor, len(m.rows)-1)
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

	m.top = clamp(m.top, max(0, len(m.rows)-m.height))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

func clamp(n, hi int) int {
	return min(max(n, 0), hi)
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

	on := func(st lipgloss.Style) lipgloss.Style { return st }
	if selected && m.styles.Select != nil {
		on = m.styles.Select
	}

	graph := on(m.styles.Graph).Render(
		table.Pad(strings.TrimRight(row.Revision.Graph, " "), graphWidth, table.AlignLeft))
	body := m.cells(*row.Revision, layout, on)
	gutter := m.gutter(*row.Revision, on)

	// The cursor's row lays the selection under each cell's own face (Styles.Select, aragonite's
	// Skin.Selected), so a tint keeps each cell's colors and reverse video (16 colors, NO_COLOR) drops
	// them while keeping weight, slant, and underline. A search match is laid over the finished line
	// with skin.Lit, which keeps the colors on its band.
	switch {
	case selected && m.styles.Select != nil:
		line := graph + gutter + body
		if rest := m.width - lipgloss.Width(line); rest > 0 {
			line += on(lipgloss.NewStyle()).Render(strings.Repeat(" ", rest))
		}

		return line
	case selected:
		return skin.Lit(graph+gutter+body, m.width, m.styles.Selected)
	case m.searchPattern != "" && matchesSearch(*row.Revision, m.searchPattern):
		return skin.Lit(graph+gutter+body, m.width, m.styles.Match)
	default:
		return graph + gutter + body
	}
}

// gutter spends the space between the graph and the columns on the multi-select
// marker, so a checked row reads without a column of its own.
func (m Model) gutter(rev jj.Revision, on face) string {
	blank := on(lipgloss.NewStyle())
	if label, ok := m.labels[rev.ChangeID]; ok {
		return m.styles.Label.Render(label) + blank.Render(strings.Repeat(" ", max(0, table.Gutter-len(label))))
	}

	if !m.checked[rev.ChangeID] {
		return blank.Render(strings.Repeat(" ", table.Gutter))
	}

	return on(m.styles.Checked).Render(CheckMark) + blank.Render(strings.Repeat(" ", max(0, table.Gutter-1)))
}

// face is a cell's style as its row draws it: itself, or under the selection on the cursor's row.
type face func(lipgloss.Style) lipgloss.Style

func (m Model) cells(rev jj.Revision, layout table.Layout, on face) string {
	text := Cells(rev)
	rendered := make([]string, 0, len(layout.Columns))

	for _, col := range layout.Columns {
		width := layout.Width(col.Key)

		if col.Key == ColSize && !rev.Conflict && !rev.Immutable {
			rendered = append(rendered, m.sizeCell(rev, width, on))

			continue
		}

		if mark, ok := m.marks[rev.ChangeID]; ok && col.Key == ColDesc {
			rendered = append(rendered, m.markedCell(mark, text[col.Key], width, m.style(rev, col.Key), on))

			continue
		}

		if label := m.unread[rev.ChangeID]; label != "" && col.Key == ColDesc {
			rendered = append(rendered, m.unreadCell(label, text[col.Key], width, m.style(rev, col.Key), on))

			continue
		}

		cell := table.PadTrim(text[col.Key], width, col.Align, col.Trim)
		rendered = append(rendered, on(m.style(rev, col.Key)).Render(cell))
	}

	return strings.Join(rendered, on(lipgloss.NewStyle()).Render(strings.Repeat(" ", table.Gutter)))
}

// markedCell spends the start of the description on the chip, which survives a narrow row better
// than the description does: at 80 columns the chip is the one thing a preview has to say.
func (m Model) markedCell(mark Mark, desc string, width int, descFace lipgloss.Style, on face) string {
	chipFace := m.styles.Mark
	if mark.Muted {
		chipFace = m.styles.MarkMuted
	}

	chip := table.Truncate(mark.Text, width)
	room := width - lipgloss.Width(chip) - 1

	if room <= 0 {
		return on(chipFace).Render(table.Pad(chip, width, table.AlignLeft))
	}

	return on(chipFace).Render(chip) + on(lipgloss.NewStyle()).Render(" ") +
		on(descFace).Render(table.PadTrim(desc, room, table.AlignLeft, table.TrimRight))
}

// unreadCell ends the description with the row's unread count, which a narrow row keeps over the
// description's tail: it is what ]u steps to.
func (m Model) unreadCell(label, desc string, width int, descFace lipgloss.Style, on face) string {
	chip := table.Truncate(label, width)
	room := width - lipgloss.Width(chip) - 1

	if room <= 0 {
		return on(m.styles.Unread).Render(table.Pad(chip, width, table.AlignRight))
	}

	return on(descFace).Render(table.PadTrim(desc, room, table.AlignLeft, table.TrimRight)) +
		on(lipgloss.NewStyle()).Render(" ") + on(m.styles.Unread).Render(chip)
}

// sizeCell colors the added and removed counts apart from the file count,
// then pads to width by display cells because the styled string carries escape
// bytes that table.Pad would count.
func (m Model) sizeCell(rev jj.Revision, width int, on face) string {
	blank := on(lipgloss.NewStyle())

	plain := size(rev)
	if plain == "" || lipgloss.Width(plain) > width {
		return blank.Render(table.PadTrim(plain, width, table.AlignRight, table.TrimRight))
	}

	files, rest, _ := strings.Cut(plain, " ")
	added, removed, _ := strings.Cut(rest, " ")

	styled := on(m.styles.Files).Render(files) + blank.Render(" ") +
		on(m.styles.Added).Render(added) + blank.Render(" ") +
		on(m.styles.Removed).Render(removed)

	return blank.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(plain)))) + styled
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

// Overflow is what the row under the cursor had no room for at the current width: each column's
// whole text where its column collapsed or cut it. The author is said as the whole email, since the
// column only ever had room for the handle.
func (m Model) Overflow() []string {
	rev := m.Selected()
	if rev == nil || m.width <= 0 {
		return nil
	}

	layout := Fit(m.width, GraphWidth(m.rows))
	text := Cells(*rev)
	whole := map[string]string{
		ColDesc:      rev.Description,
		ColBookmarks: text[ColBookmarks],
		ColAuthor:    rev.AuthorEmail,
		ColAge:       rev.Age,
	}

	var out []string

	for _, key := range []string{ColDesc, ColBookmarks, ColAuthor, ColAge} {
		full := whole[key]
		if full == "" {
			continue
		}

		shown := text[key]
		if key == ColAuthor || key == ColAge {
			// These columns abbreviate on purpose; only a collapsed column has lost them.
			shown = ""
		}

		if hidden(layout, key) || (shown != "" && lipgloss.Width(shown) > layout.Width(key)) {
			out = append(out, full)
		}
	}

	return out
}

// hidden reports whether the layout collapsed the column out of the row.
func hidden(layout table.Layout, key string) bool {
	return slices.Contains(layout.Dropped, key) || layout.Width(key) == 0
}
