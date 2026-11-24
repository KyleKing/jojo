// Package outlineview draws the stack outline: the revisions of one stack as chapters, parent first,
// each a row that says how big it is, whether it is empty or conflicted, where it is bookmarked, what
// the forge last said about it, and how much of it has been read. The parent owns every read; this
// package only draws what it is handed.
package outlineview

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/empty"
	"github.com/kyleking/aragonite/tui/glyph"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/readmark"
	"github.com/kyleking/jojo/internal/ui"
)

// Chapter is one revision of the stack with what the parent knows about it.
type Chapter struct {
	Rev *jj.Revision
	// Forge is the pull request and CI state the forge pane already holds for the chapter's bookmark,
	// "" when it holds none. The outline never asks the forge itself.
	Forge string
	Read  readmark.Strip
}

// Column keys.
const (
	colNumber    = "number"
	colChange    = "change"
	colDesc      = "desc"
	colBookmarks = "bookmarks"
	colSize      = "size"
	colForge     = "forge"
	colRead      = "read"
)

// Collapse order: what the outline is read for, the description and the read strip, holds longest,
// then the pull request, which a compact form keeps to a few cells. A size goes first, since the
// dock beside the outline shows the change itself, then a bookmark's name; the inspector says both
// once their columns have gone.
const (
	prioSize = iota + 1
	prioBookmarks
	prioForge
	prioRead
)

// Column widths, in display cells: the change id is pinned the way the log pins it, with its flag
// glyph a space after it, and the rest bound how much a row's metadata may take before it collapses.
// The description keeps descMin cells, beside the dock too, before a metadata column takes one from
// it: a subject cut to a dozen cells names nothing. Where the pull request and the read strip fit
// beside it, it is drawn whole up to descRoomy cells before the size or the bookmarks take a cell.
// No metadata column is ever wider than its widest cell, so every cell it would pad goes to the
// description instead.
const (
	changeWidth  = 8 + len(" ") + 1
	descMin      = 24
	descRoomy    = 40
	descWeight   = 4
	bookmarksMin = 6
	bookmarksMax = 18
	sizeMax      = 14
	forgeMin     = 8
	forgeMax     = 22
)

// Model is the outline's state: the chapters, which one the cursor is on, and the room it draws in.
type Model struct {
	skin     ui.Skin
	chapters []Chapter
	// emptyHints are the keys out of an empty outline, which the parent names from its keymap.
	emptyHints []keyhint.Hint
	cursor     int
	top        int
	width      int
	height     int
}

// SetEmptyHints is the keys an empty outline's card offers, named by the parent's keymap so a
// rebind reaches it.
func (m *Model) SetEmptyHints(hints []keyhint.Hint) { m.emptyHints = hints }

// New returns an empty outline drawn in skin.
func New(skin ui.Skin) Model { return Model{skin: skin} }

// columns is the outline's table, less the bookmark and forge columns when no chapter has anything
// to put in them, so a stack with no branches yet spends no width on blanks. Each metadata column is
// capped at its widest cell, so the description takes every cell the rest leave.
func (m Model) columns() []table.Column {
	digits := len(strconv.Itoa(len(m.chapters)))
	cols := []table.Column{
		{Key: colNumber, Min: digits, Max: digits, Align: table.AlignRight},
		{Key: colChange, Min: changeWidth, Max: changeWidth},
		{Key: colDesc, Min: m.descLeast(), Weight: descWeight},
	}

	if widest := m.widest(bookmarkText); widest > 0 {
		lo, hi := fitBounds(bookmarksMin, bookmarksMax, widest)
		cols = append(cols, table.Column{
			Key: colBookmarks, Min: lo, Max: hi, Weight: 1, Priority: prioBookmarks,
		})
	}

	// A size is read whole or not at all, so its column is as wide as the widest one: it collapses
	// before it cuts.
	size := min(sizeMax, max(1, m.widest(func(c Chapter) string { return sizeText(c.Rev) })))
	cols = append(cols, table.Column{
		Key: colSize, Min: size, Max: size, Priority: prioSize, Align: table.AlignRight,
	})

	// The forge column falls back to its compact form ("#41 ○✓") before it cuts or drops, so its
	// least is the widest compact cell.
	if widest := m.widest(func(c Chapter) string { return c.Forge }); widest > 0 {
		_, hi := fitBounds(forgeMin, forgeMax, widest)
		lo := min(hi, m.widest(func(c Chapter) string { return CompactForge(c.Forge) }))
		cols = append(cols, table.Column{Key: colForge, Min: lo, Max: hi, Weight: 1, Priority: prioForge})
	}

	read := max(readmark.Width, m.widest(readText))
	least := min(read, m.widest(compactRead))

	return append(cols, table.Column{Key: colRead, Min: least, Max: read, Priority: prioRead})
}

// compactRead is a chapter's read column with no room for the strip: the unread count, a check once
// it is all read, a dot while it is unknown.
func compactRead(c Chapter) string {
	switch {
	case !c.Read.Known():
		return "·"
	case c.Read.Unread() == 0:
		return "✓"
	}

	return c.Read.Label()
}

// readText is a chapter's read column: the strip, then the unread count when anything is left.
func readText(c Chapter) string {
	if label := c.Read.Label(); label != "" {
		return c.Read.String() + " " + label
	}

	return c.Read.String()
}

// layout fits the columns to the outline's width, the description first: it takes the spare cells
// until the widest description is whole, then the metadata columns share what is left up to their
// widest cells, and whatever still remains goes back to the description so a row spans the width.
func (m Model) layout() table.Layout {
	width := m.width - table.Gutter
	cols := m.columns()
	whole := max(m.descLeast(), m.widest(description))

	first := slices.Clone(cols)
	for i := range first {
		first[i].Weight = 0
		if first[i].Key == colDesc {
			first[i].Weight, first[i].Max = 1, whole
		}
	}

	l := table.FitCompact(first, width)
	if l.Width(colDesc) < whole || l.Total() >= width {
		if roomy, ok := m.roomy(width); ok {
			return spread(roomy, width)
		}

		return l
	}

	second := slices.Clone(cols)
	for i := range second {
		if second[i].Key == colDesc {
			second[i].Min, second[i].Max, second[i].Weight = whole, whole, 0
		}
	}

	return spread(table.FitCompact(second, width), width)
}

// roomy is the layout that keeps the description whole, up to descRoomy cells, by dropping the size
// and then the bookmarks, when the pull request and the read strip still fit beside it. It answers
// false when they do not, or when the description needs no more than its least anyway.
func (m Model) roomy(width int) (table.Layout, bool) {
	want := min(m.widest(description), descRoomy)
	if want <= m.descLeast() {
		return table.Layout{}, false
	}

	cols := m.columns()
	for i := range cols {
		cols[i].Weight = 0
		if cols[i].Key == colDesc {
			cols[i].Min, cols[i].Max = want, want
		}
	}

	l := table.FitCompact(cols, width)

	for _, c := range cols {
		if (c.Priority == 0 || c.Priority >= prioForge) && l.Width(c.Key) == 0 {
			return l, false
		}
	}

	return l, true
}

// spread gives the cells l leaves over to the description, so a row spans the width.
func spread(l table.Layout, width int) table.Layout {
	spare := width - l.Total()
	if spare <= 0 {
		return l
	}

	cols := make([]table.Column, 0, len(l.Columns))
	for _, c := range l.Columns {
		w := l.Width(c.Key)
		if c.Key == colDesc {
			w += spare
		}

		c.Min, c.Max, c.Weight = w, w, 0
		cols = append(cols, c)
	}

	return table.FitCompact(cols, width)
}

// fitBounds caps a column's bounds at the widest cell it holds, so it never pads past its content:
// a column whose content is narrower than its least is exactly that wide.
func fitBounds(least, most, widest int) (int, int) {
	most = min(most, widest)

	return min(least, most), most
}

// descLeast is the narrowest the description goes before a metadata column is dropped: descMin, or
// the widest description when every one is shorter, which needs no room past its own text.
func (m Model) descLeast() int { return min(descMin, max(1, m.widest(description))) }

// bookmarkText is a chapter's bookmarks as its row writes them.
func bookmarkText(c Chapter) string { return strings.Join(c.Rev.Bookmarks, " ") }

// widest is the widest text any chapter has for a column, 0 when none has any.
func (m Model) widest(text func(Chapter) string) int {
	w := 0
	for _, c := range m.chapters {
		w = max(w, lipgloss.Width(text(c)))
	}

	return w
}

// ContentWidth is how wide the outline is when no row is cut: each column as wide as its widest
// cell, within its bounds, and the title. A caller laying the outline beside something else gives
// it this much and the rest to its neighbor, rather than a share its short rows would leave blank.
func (m Model) ContentWidth() int {
	natural := map[string]int{
		colNumber:    len(strconv.Itoa(len(m.chapters))),
		colChange:    changeWidth,
		colDesc:      m.widest(description),
		colBookmarks: m.widest(bookmarkText),
		colSize:      m.widest(func(c Chapter) string { return sizeText(c.Rev) }),
		colForge:     m.widest(func(c Chapter) string { return c.Forge }),
		colRead:      m.widest(readText),
	}

	total := table.Gutter

	for i, col := range m.columns() {
		w := max(col.Min, natural[col.Key])
		if col.Max > 0 {
			w = min(w, col.Max)
		}

		if i > 0 {
			total += table.Gutter
		}

		total += w
	}

	return max(total, lipgloss.Width(m.titleText()))
}

// MinWidth is the narrowest the outline draws every column it has at, each at its least, so a
// caller squeezing it can tell where its rows would start to lose a column.
func (m Model) MinWidth() int {
	return sumWidth(m.columns())
}

// EssentialWidth is the narrowest the outline draws what it is read for at: the number, the change,
// the description, the pull request, and the read strip, each at its least, with the size and the
// bookmarks dropped. A caller squeezing it below this cuts the description short or loses a column
// nothing else on screen carries.
func (m Model) EssentialWidth() int {
	cols := slices.DeleteFunc(m.columns(), func(c table.Column) bool {
		return c.Priority > 0 && c.Priority < prioForge
	})

	return sumWidth(cols)
}

// sumWidth is how wide cols draw, each at its least, with the gutters around them.
func sumWidth(cols []table.Column) int {
	total := table.Gutter

	for i, col := range cols {
		if i > 0 {
			total += table.Gutter
		}

		total += col.Min
	}

	return total
}

// description is a chapter's description as its row draws it.
func description(c Chapter) string {
	if c.Rev.Description == "" {
		return noDescription
	}

	return c.Rev.Description
}

// noDescription stands in for a chapter with no description yet.
const noDescription = "(no description set)"

// SetChapters replaces the chapters, keeping the cursor on the change it was on when the stack
// still holds it, and otherwise on the chapter at the working copy, else the first.
func (m *Model) SetChapters(chapters []Chapter) {
	keep := ""
	if rev := m.Selected(); rev != nil {
		keep = rev.ChangeID
	}

	m.chapters, m.cursor, m.top = chapters, 0, 0

	if keep != "" && m.Focus(keep) {
		return
	}

	for i, c := range chapters {
		if c.Rev.WorkingCopy {
			m.cursor = i
		}
	}

	m.scrollIntoView()
}

// Chapters is what the outline draws, parent first.
func (m Model) Chapters() []Chapter { return m.chapters }

// SetRead updates one chapter's read strip, the one thing that changes as the docked diff is read.
func (m *Model) SetRead(changeID string, strip readmark.Strip) {
	for i := range m.chapters {
		if m.chapters[i].Rev.ChangeID == changeID {
			m.chapters[i].Read = strip
		}
	}
}

// Focus puts the cursor on changeID, reporting whether the stack holds it.
func (m *Model) Focus(changeID string) bool {
	for i, c := range m.chapters {
		if c.Rev.ChangeID == changeID {
			m.cursor = i
			m.scrollIntoView()

			return true
		}
	}

	return false
}

// SetSize records the room the outline draws in, title row included.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.scrollIntoView()
}

// Selected is the revision under the cursor, nil for an empty outline.
func (m Model) Selected() *jj.Revision {
	if m.cursor < 0 || m.cursor >= len(m.chapters) {
		return nil
	}

	return m.chapters[m.cursor].Rev
}

// Move steps the cursor delta chapters, clamped at both ends, and reports whether it moved.
func (m *Model) Move(delta int) bool {
	next := min(max(0, m.cursor+delta), len(m.chapters)-1)
	if next < 0 || next == m.cursor {
		return false
	}

	m.cursor = next
	m.scrollIntoView()

	return true
}

// MoveKey moves the cursor for one of the list's own movement keys, reporting whether key was one.
// The bool says the key was a movement key, whether or not the cursor could move.
func (m *Model) MoveKey(key string) (bool, bool) {
	switch key {
	case "j", "down":
		return true, m.Move(1)
	case "k", "up":
		return true, m.Move(-1)
	case "g", "home":
		return true, m.Move(-len(m.chapters))
	case "G", "end":
		return true, m.Move(len(m.chapters))
	case "ctrl+d", "pgdown":
		return true, m.Move(max(1, m.body()/2)) //nolint:mnd // half a page
	case "ctrl+u", "pgup":
		return true, m.Move(-max(1, m.body()/2)) //nolint:mnd // half a page
	}

	return false, false
}

// body is the rows below the title.
func (m Model) body() int { return max(0, m.height-1) }

func (m *Model) scrollIntoView() {
	body := m.body()
	if body == 0 {
		return
	}

	if m.cursor < m.top {
		m.top = m.cursor
	}

	if m.cursor >= m.top+body {
		m.top = m.cursor - body + 1
	}

	m.top = max(0, min(m.top, len(m.chapters)-body))
}

// View draws the title row and a row per chapter into exactly the outline's height.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	lines := []string{m.titleLine()}

	if len(m.chapters) == 0 {
		card := empty.Empty{
			Title: "No stack here",
			Why: "Nothing sits between trunk() and the revision the outline was opened on: it is on " +
				"trunk, or the outline_revset in config.toml read nothing.",
			Hints: m.emptyHints,
		}

		return strings.Join(append(lines, empty.Card(m.skin.Skin, card, m.width, m.height-1)...), "\n")
	}

	layout := m.layout()

	for i := m.top; i < len(m.chapters) && len(lines) < m.height; i++ {
		lines = append(lines, m.row(i, layout))
	}

	for len(lines) < m.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// titleLine names the stack the way a reader does, by its size and the branch it is for: "Stack · 4 ·
// parse-v2". The revset that read it is the inspector's to show, since it is long and the same for
// every stack read the same way.
func (m Model) titleLine() string {
	title := m.skin.Title.Render(table.Truncate("Stack", m.width))
	rest := m.width - lipgloss.Width(title)

	if rest <= len(" · ") || len(m.chapters) == 0 {
		return title
	}

	return title + m.skin.Muted.Render(table.Truncate(strings.TrimPrefix(m.titleText(), "Stack"), rest))
}

// titleText is the title row unstyled and uncut.
func (m Model) titleText() string {
	if len(m.chapters) == 0 {
		return "Stack"
	}

	return "Stack · " + strconv.Itoa(len(m.chapters)) + " · " + m.Name()
}

// Name is what the stack is called: the bookmark nearest its tip, since that is the branch a review
// of it is for, else the tip's change id.
func (m Model) Name() string {
	for i := len(m.chapters) - 1; i >= 0; i-- {
		if names := m.chapters[i].Rev.LocalBookmarks; len(names) > 0 {
			return names[0]
		}
	}

	if len(m.chapters) == 0 {
		return ""
	}

	return m.chapters[len(m.chapters)-1].Rev.ChangeID
}

// row draws chapter i. The cursor's row lays the selection under each cell's own face
// (Skin.Selected) rather than over the finished line, so a cell's weight, slant, and underline
// survive inside a 16-color reverse bar, which drops colors only.
func (m Model) row(i int, layout table.Layout) string {
	c := m.chapters[i]
	cells := make([]string, 0, len(layout.Columns))

	on := func(st lipgloss.Style) lipgloss.Style { return st }
	if i == m.cursor {
		on = m.skin.Skin.Selected
	}

	for _, col := range layout.Columns {
		width := layout.Width(col.Key)
		cells = append(cells, m.cell(i, c, col, width, on))
	}

	blank := on(lipgloss.NewStyle())
	gutter := blank.Render(strings.Repeat(" ", table.Gutter))
	line := gutter + strings.Join(cells, gutter)

	if i == m.cursor {
		if rest := m.width - lipgloss.Width(line); rest > 0 {
			line += blank.Render(strings.Repeat(" ", rest))
		}
	}

	return line
}

// face is a cell's style as its row draws it: itself, or under the selection on the cursor's row.
type face func(lipgloss.Style) lipgloss.Style

func (m Model) cell(i int, c Chapter, col table.Column, width int, on face) string {
	rev := c.Rev
	text, face := "", m.skin.Body

	switch col.Key {
	case colNumber:
		text, face = strconv.Itoa(i+1), m.skin.Muted
	case colChange:
		return m.change(rev, width, on)
	case colDesc:
		text = glyph.Settle(rev.Description)
		if text == "" {
			text, face = noDescription, m.skin.Muted.Italic(true)
		}
	case colBookmarks:
		text, face = glyph.Settle(bookmarkText(c)), m.skin.Bookmark
	case colSize:
		return m.size(rev, width, on)
	case colForge:
		text, face = c.Forge, m.skin.Muted
		if lipgloss.Width(text) > width {
			text = CompactForge(text)
		}
	case colRead:
		text, face = readText(c), m.readFace(c.Read)
		if lipgloss.Width(text) > width {
			text = compactRead(c)
		}
	}

	if rev.Conflict && col.Key == colDesc {
		face = m.skin.Conflicted
	}

	return on(face).Render(table.PadTrim(text, width, col.Align, table.TrimRight))
}

// change is the change id and, a space after it, the one glyph its state comes down to, so the flag
// costs one cell rather than a column and its gutter.
func (m Model) change(rev *jj.Revision, width int, on face) string {
	idFace := m.skin.ChangeIDPrefix
	if rev.WorkingCopy {
		idFace = m.skin.WorkingCopy
	}

	id := table.PadTrim(rev.ChangeID, max(0, width-len(" ")-1), table.AlignLeft, table.TrimRight)

	return on(idFace).Render(id) + on(lipgloss.NewStyle()).Render(" ") + on(m.flagFace(rev)).Render(flags(rev))
}

// flags is the one glyph a chapter's state comes down to: a conflict outranks emptiness, which
// outranks being the working copy.
func flags(rev *jj.Revision) string {
	switch {
	case rev.Conflict:
		return "×"
	case rev.Empty:
		return "∅"
	case rev.WorkingCopy:
		return "@"
	}

	return " "
}

func (m Model) flagFace(rev *jj.Revision) lipgloss.Style {
	switch {
	case rev.Conflict:
		return m.skin.Conflicted
	case rev.Empty:
		return m.skin.Warning
	}

	return m.skin.Accent
}

func (m Model) readFace(s readmark.Strip) lipgloss.Style {
	if s.Known() && s.Read == s.Total {
		return m.skin.Success
	}

	if s.Known() {
		return m.skin.Accent
	}

	return m.skin.Muted
}

func (m Model) size(rev *jj.Revision, width int, on face) string {
	blank := on(lipgloss.NewStyle())
	if rev.Files == 0 && rev.Added == 0 && rev.Removed == 0 {
		return blank.Render(table.Pad("", width, table.AlignRight))
	}

	files := strconv.Itoa(rev.Files) + "f"
	added, removed := "+"+strconv.Itoa(rev.Added), "-"+strconv.Itoa(rev.Removed)
	plain := sizeText(rev)

	if lipgloss.Width(plain) > width {
		return on(m.skin.Muted).Render(table.PadTrim(plain, width, table.AlignRight, table.TrimRight))
	}

	return blank.Render(strings.Repeat(" ", width-lipgloss.Width(plain))) +
		on(m.skin.Muted).Render(files) + blank.Render(" ") + on(m.skin.Added).Render(added) + blank.Render(" ") +
		on(m.skin.Removed).Render(removed)
}

// sizeText is a chapter's size as the row writes it, "3f +7 -3", or nothing for an empty change.
func sizeText(rev *jj.Revision) string {
	if rev.Files == 0 && rev.Added == 0 && rev.Removed == 0 {
		return ""
	}

	return strconv.Itoa(rev.Files) + "f +" + strconv.Itoa(rev.Added) + " -" + strconv.Itoa(rev.Removed)
}

// The compact forge glyphs: the pull request's state, then its CI, then changes requested.
const (
	glyphOpen     = "○"
	glyphDraft    = "◇"
	glyphMerged   = "●"
	glyphClosed   = "×"
	glyphPass     = "✓"
	glyphFail     = "✗"
	glyphPending  = "…"
	glyphChanges  = "!"
	forgeWordsSep = " · "
)

// CompactForge is a forge summary ("#41 open · approved · CI pass", "#7 draft") as a number and a
// glyph or two ("#41 ○✓", "#7 ◇"): the state (○ open, ◇ draft, ● merged, × closed), then CI (✓ ✗ …),
// then a bang when changes were requested. The words stay in the inspector, which says the whole summary
// whenever the row drew this instead.
func CompactForge(summary string) string {
	fields := strings.Fields(strings.ReplaceAll(strings.ToLower(summary), forgeWordsSep, " "))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "#") {
		return summary
	}

	state, ci, changes := "", "", ""

	for i, word := range fields[1:] {
		switch word {
		case "open":
			state = glyphOpen
		case "draft":
			state = glyphDraft
		case "merged":
			state = glyphMerged
		case "closed":
			state = glyphClosed
		case "changes":
			changes = glyphChanges
		case "ci":
			if i+2 < len(fields) {
				ci = ciGlyph(fields[i+2])
			}
		}
	}

	return fields[0] + " " + state + ci + changes
}

func ciGlyph(word string) string {
	switch word {
	case "pass", "passing":
		return glyphPass
	case "fail", "failing":
		return glyphFail
	case "pending", "mixed":
		return glyphPending
	}

	return ""
}

// Overflow is what the chapter under the cursor had no room for: its whole description when the
// row cut it, and its bookmarks, size, and forge state when their columns collapsed or cut them (the
// forge's compact form counts as cut).
func (m Model) Overflow() []string {
	rev := m.Selected()
	if rev == nil {
		return nil
	}

	c := m.chapters[m.cursor]
	layout := m.layout()
	whole := map[string]string{
		colDesc: rev.Description, colBookmarks: strings.Join(rev.Bookmarks, " "), colForge: c.Forge,
		colSize: sizeText(rev),
	}

	var out []string

	for _, key := range []string{colDesc, colBookmarks, colSize, colForge} {
		text := whole[key]
		if text != "" && lipgloss.Width(text) > layout.Width(key) {
			out = append(out, text)
		}
	}

	return out
}
