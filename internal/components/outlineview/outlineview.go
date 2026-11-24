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

// Collapse order: what the outline is read for, the description and the read strip, holds longest.
const (
	prioForge = iota + 1
	prioBookmarks
	prioSize
	prioRead
)

// Column widths, in display cells: the change id is pinned the way the log pins it, with its flag
// glyph a space after it, and the rest bound how much a row's metadata may take before it collapses.
// No metadata column is ever wider than its widest cell, so every cell it would pad goes to the
// description instead.
const (
	changeWidth  = 8 + len(" ") + 1
	descMin      = 24
	descBeside   = 14
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
	beside     bool
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
		{Key: colDesc, Min: m.descMin(), Weight: descWeight},
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

	if widest := m.widest(func(c Chapter) string { return c.Forge }); widest > 0 {
		lo, hi := fitBounds(forgeMin, forgeMax, widest)
		cols = append(cols, table.Column{Key: colForge, Min: lo, Max: hi, Weight: 1, Priority: prioForge})
	}

	return append(cols, table.Column{Key: colRead, Min: readmark.Width, Max: readmark.Width, Priority: prioRead})
}

// layout fits the columns to the outline's width, the description first: it takes the spare cells
// until the widest description is whole, then the metadata columns share what is left up to their
// widest cells, and whatever still remains goes back to the description so a row spans the width.
func (m Model) layout() table.Layout {
	width := m.width - table.Gutter
	cols := m.columns()
	whole := max(m.descMin(), m.widest(description))

	first := slices.Clone(cols)
	for i := range first {
		first[i].Weight = 0
		if first[i].Key == colDesc {
			first[i].Weight, first[i].Max = 1, whole
		}
	}

	l := table.FitCompact(first, width)
	if l.Width(colDesc) < whole || l.Total() >= width {
		return l
	}

	second := slices.Clone(cols)
	for i := range second {
		if second[i].Key == colDesc {
			second[i].Min, second[i].Max, second[i].Weight = whole, whole, 0
		}
	}

	l = table.FitCompact(second, width)

	spare := width - l.Total()
	if spare <= 0 {
		return l
	}

	third := make([]table.Column, 0, len(l.Columns))
	for _, c := range l.Columns {
		w := l.Width(c.Key)
		if c.Key == colDesc {
			w += spare
		}

		c.Min, c.Max, c.Weight = w, w, 0
		third = append(third, c)
	}

	return table.FitCompact(third, width)
}

// fitBounds caps a column's bounds at the widest cell it holds, so it never pads past its content:
// a column whose content is narrower than its least is exactly that wide.
func fitBounds(least, most, widest int) (int, int) {
	most = min(most, widest)

	return min(least, most), most
}

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

// descMin is the narrowest the description goes before a column is dropped. Beside the docked diff
// it gives way sooner, since the inspector carries a cut description whole and nothing carries a
// dropped size or bookmark.
func (m Model) descMin() int {
	if m.beside {
		return descBeside
	}

	return descMin
}

// SetBeside says whether the outline shares the terminal's width with the docked diff.
func (m *Model) SetBeside(beside bool) { m.beside = beside }

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
	total := table.Gutter

	for i, col := range m.columns() {
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

func (m Model) row(i int, layout table.Layout) string {
	c := m.chapters[i]
	cells := make([]string, 0, len(layout.Columns))

	for _, col := range layout.Columns {
		width := layout.Width(col.Key)
		cells = append(cells, m.cell(i, c, col, width))
	}

	line := strings.Repeat(" ", table.Gutter) + table.Join(cells)
	if i == m.cursor {
		return m.skin.Select(line, m.width)
	}

	return line
}

func (m Model) cell(i int, c Chapter, col table.Column, width int) string {
	rev := c.Rev
	text, face := "", m.skin.Body

	switch col.Key {
	case colNumber:
		text, face = strconv.Itoa(i+1), m.skin.Muted
	case colChange:
		return m.change(rev, width)
	case colDesc:
		text = glyph.Settle(rev.Description)
		if text == "" {
			text, face = noDescription, m.skin.Muted.Italic(true)
		}
	case colBookmarks:
		text, face = glyph.Settle(bookmarkText(c)), m.skin.Bookmark
	case colSize:
		return m.size(rev, width)
	case colForge:
		text, face = c.Forge, m.skin.Muted
	case colRead:
		text, face = c.Read.String(), m.readFace(c.Read)
	}

	if rev.Conflict && col.Key == colDesc {
		face = m.skin.Conflicted
	}

	return face.Render(table.PadTrim(text, width, col.Align, table.TrimRight))
}

// change is the change id and, a space after it, the one glyph its state comes down to, so the flag
// costs one cell rather than a column and its gutter.
func (m Model) change(rev *jj.Revision, width int) string {
	idFace := m.skin.ChangeIDPrefix
	if rev.WorkingCopy {
		idFace = m.skin.WorkingCopy
	}

	id := table.PadTrim(rev.ChangeID, max(0, width-len(" ")-1), table.AlignLeft, table.TrimRight)

	return idFace.Render(id) + " " + m.flagFace(rev).Render(flags(rev))
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

func (m Model) size(rev *jj.Revision, width int) string {
	if rev.Files == 0 && rev.Added == 0 && rev.Removed == 0 {
		return table.Pad("", width, table.AlignRight)
	}

	files := strconv.Itoa(rev.Files) + "f"
	added, removed := "+"+strconv.Itoa(rev.Added), "-"+strconv.Itoa(rev.Removed)
	plain := sizeText(rev)

	if lipgloss.Width(plain) > width {
		return m.skin.Muted.Render(table.PadTrim(plain, width, table.AlignRight, table.TrimRight))
	}

	return strings.Repeat(" ", width-lipgloss.Width(plain)) +
		m.skin.Muted.Render(files) + " " + m.skin.Added.Render(added) + " " + m.skin.Removed.Render(removed)
}

// sizeText is a chapter's size as the row writes it, "3f +7 -3", or nothing for an empty change.
func sizeText(rev *jj.Revision) string {
	if rev.Files == 0 && rev.Added == 0 && rev.Removed == 0 {
		return ""
	}

	return strconv.Itoa(rev.Files) + "f +" + strconv.Itoa(rev.Added) + " -" + strconv.Itoa(rev.Removed)
}

// Overflow is what the chapter under the cursor had no room for: its whole description when the
// row cut it, and its bookmarks and forge state when their columns collapsed or cut them.
func (m Model) Overflow() []string {
	rev := m.Selected()
	if rev == nil {
		return nil
	}

	c := m.chapters[m.cursor]
	layout := m.layout()
	whole := map[string]string{
		colDesc: rev.Description, colBookmarks: strings.Join(rev.Bookmarks, " "), colForge: c.Forge,
	}

	var out []string

	for _, key := range []string{colDesc, colBookmarks, colForge} {
		text := whole[key]
		if text != "" && lipgloss.Width(text) > layout.Width(key) {
			out = append(out, text)
		}
	}

	return out
}
