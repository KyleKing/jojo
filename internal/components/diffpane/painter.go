package diffpane

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/jj-diff/render"
)

// Syntax colors cost a lexer pass over every file a diff touches, which is tens of milliseconds for
// a small revision and over a second for a large one, and drawing every row of a large diff costs
// tens more. The pane pays neither on the update loop. A diff is a render.Document per file, which
// parses and counts its rows without drawing them, and the pane draws only the rows on screen and a
// margin around them, with Peek, which never lexes and draws a file plain until Start has lexed it.
// Colorize starts the lexers of the files around the cursor and answers a command that only waits
// for them; Colored then marks those documents lexed, and the next frame Peeks the same window again,
// now in color. Peek draws the same rows in the same places either way, so a cursor kept across the
// swap stays on its line.
//
// Only the files near the cursor are lexed, never the whole diff: a log walked with the diff docked
// shows each revision for a moment, and lexing all 92 files of a large one cost 20 ms of CPU per
// keypress for colors nobody scrolled to. A move that brings an unlexed file near the cursor asks
// for it then.

// coloredKeep is how many colored diffs a pane remembers, so stepping back to a revision the cursor
// just left draws it in color at once rather than plain and then again.
const coloredKeep = 8

// windowMargin is how many rows past the ones asked for a window draws on each side, so scrolling a
// few rows draws from the window rather than asking the documents again.
const windowMargin = 32

// lexMargin is how far from the cursor, each way, a file is lexed: past any row a frame around the
// cursor draws, a dock's or a full screen's, and no further.
const lexMargin = 2 * windowMargin

// ColoredMsg says the lexers of a diff a pane drew plain have finished, so its rows can be drawn
// again in color. Docked names which pane asked, since the full-screen pane and the dock can each be
// waiting.
type ColoredMsg struct {
	sheet  *sheet
	parts  []int
	docked bool
}

// drawKey is everything a diff's rows depend on beyond the skin, which a pane never changes.
type drawKey struct {
	text       string
	width      int
	sideBySide bool
}

// sheet is a diff parsed for drawing: each file of its git segments a document, and jj's prose
// between them as lines, in order, with where each file's header row lands. It draws rows on demand
// and keeps the last window it drew. It is shared by every copy of the pane and only touched on the
// update loop.
type sheet struct {
	draw  func(line string) string
	opts  render.Options
	parts []part
	files []int
	paths []string
	// hunks is each file's hunk label rows, read the first time a motion needs them.
	hunks map[int][]int
	// asked is the parts whose lexers have been started, and lexed those whose colors have landed.
	asked, lexed []bool
	// win is the rows last drawn, from row lo, and winGen the lexed generation they were drawn at, so
	// the first frame after more colors land draws the window again.
	win    []string
	lo     int
	rows   int
	gen    int
	winGen int
}

// part is one file's document, or one line of prose, starting at row at.
type part struct {
	doc   *render.Document
	text  string
	prose string
	// path is the file the document draws, "" for prose or a file whose header names none.
	path string
	at   int
	n    int
}

// parse splits key's diff into a document per file and its prose lines, drawing nothing yet. The
// documents' rows are the rows one document of the whole segment would draw, since jj-diff lays a
// diff out file by file.
func (m *Model) parse(key drawKey) *sheet {
	opts := render.Options{
		Width:      key.width,
		Palette:    paletteFor(m.skin),
		Syntax:     true,
		WordDiff:   true,
		SideBySide: key.sideBySide,
		SignBar:    m.skin.SignBar(),
		HunkLabels: true,
	}

	skin, width := m.skin, key.width
	sh := &sheet{opts: opts, hunks: map[int][]int{}, draw: func(line string) string { return prose(skin, line, width) }}

	for _, seg := range segments(key.text) {
		if !seg.diff {
			for _, line := range seg.lines {
				sh.parts = append(sh.parts, part{prose: line, at: sh.rows, n: 1})
				sh.rows++
			}

			continue
		}

		for _, file := range splitFiles(seg.lines) {
			sh.addFile(file)
		}
	}

	sh.asked = make([]bool, len(sh.parts))
	sh.lexed = make([]bool, len(sh.parts))

	return sh
}

// addFile parses one file's diff into a document after the rows the sheet already has, and records
// its header row when the document draws one.
func (sh *sheet) addFile(text string) {
	doc := render.New(text, sh.opts)

	n := doc.Len()
	if n == 0 {
		return
	}

	p := part{doc: doc, text: text, at: sh.rows, n: n}

	// jj-diff heads every file it draws with one `<M|A|D|R> path` row, so the file's first row is its
	// header and needs no drawing to find.
	if match := gitFileRE.FindStringSubmatch(text); match != nil {
		sh.files = append(sh.files, sh.rows)
		sh.paths = append(sh.paths, match[1])
		p.path = match[1]
	}

	sh.parts = append(sh.parts, p)

	sh.rows += n
}

// splitFiles cuts a git segment's lines at each file's `diff --git` header, each file's text ending
// in a newline as a document wants it.
func splitFiles(lines []string) []string {
	var starts []int

	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") || i == 0 {
			starts = append(starts, i)
		}
	}

	texts := make([]string, 0, len(starts))

	for k, from := range starts {
		to := len(lines)
		if k+1 < len(starts) {
			to = starts[k+1]
		}

		texts = append(texts, strings.Join(lines[from:to], "\n")+"\n")
	}

	return texts
}

// rowsIn draws rows [from, to) of the sheet, each in color once its document is lexed.
func (sh *sheet) rowsIn(from, to int) []string {
	from, to = max(0, from), min(to, sh.rows)
	out := make([]string, 0, max(0, to-from))

	for _, p := range sh.parts {
		if p.at+p.n <= from || p.at >= to {
			continue
		}

		if p.doc == nil {
			out = append(out, sh.draw(p.prose))

			continue
		}

		rows, _ := p.doc.Peek(max(from, p.at)-p.at, min(to, p.at+p.n)-p.at)
		out = append(out, rows...)
	}

	return out
}

// row is row i as drawn, drawing a window around it when the last window does not hold it, or was
// drawn before the colors landed.
func (sh *sheet) row(i int) string {
	if i < 0 || i >= sh.rows {
		return ""
	}

	if i < sh.lo || i >= sh.lo+len(sh.win) || sh.gen != sh.winGen {
		sh.window(i-windowMargin, i+2*windowMargin)
	}

	return sh.win[i-sh.lo]
}

// window draws rows [from, to) and keeps them as the sheet's window.
func (sh *sheet) window(from, to int) {
	from = max(0, from)
	sh.win = sh.rowsIn(from, to)
	sh.lo, sh.winGen = from, sh.gen
}

// all draws every row, which only a caller that wants the whole text pays for.
func (sh *sheet) all() []string {
	return sh.rowsIn(0, sh.rows)
}

// start lexes, in the background, every document drawing a row in [from, to) whose lexer has not
// been started, answering those parts and a channel per document.
func (sh *sheet) start(from, to int) ([]int, []<-chan struct{}) {
	var (
		parts []int
		done  []<-chan struct{}
	)

	for i, p := range sh.parts {
		if p.doc == nil || sh.asked[i] || p.at+p.n <= from || p.at >= to {
			continue
		}

		sh.asked[i] = true
		parts = append(parts, i)
		done = append(done, p.doc.Start())
	}

	return parts, done
}

// landed marks parts lexed, so the next frame draws them in color.
func (sh *sheet) landed(parts []int) {
	for _, i := range parts {
		sh.lexed[i] = true
	}

	sh.gen++
}

// hunkPast is the first hunk label past cursor in dir, walking files from the cursor's own outward
// and reading each one's hunks only when the walk reaches it.
func (sh *sheet) hunkPast(cursor, dir int) (int, bool) {
	n := len(sh.parts)

	for k := range n {
		f := k
		if dir < 0 {
			f = n - 1 - k
		}

		p := sh.parts[f]
		if (dir > 0 && p.at+p.n <= cursor) || (dir < 0 && p.at > cursor) {
			continue
		}

		rows := sh.fileHunks(f)
		for j := range rows {
			r := rows[j]
			if dir < 0 {
				r = rows[len(rows)-1-j]
			}

			if (dir > 0 && r > cursor) || (dir < 0 && r < cursor) {
				return r, true
			}
		}
	}

	return 0, false
}

// furthestHunk is the last hunk label in dir, from the file the walk starts in at the far end,
// fallback when no file that way has one.
func (sh *sheet) furthestHunk(dir, fallback int) int {
	n := len(sh.parts)

	for k := range n {
		f := n - 1 - k
		if dir < 0 {
			f = k
		}

		if rows := sh.fileHunks(f); len(rows) > 0 {
			if dir < 0 {
				return rows[0]
			}

			return rows[len(rows)-1]
		}
	}

	return fallback
}

// partAt is the index of the part drawing row, -1 past the last.
func (sh *sheet) partAt(row int) int {
	i := sort.Search(len(sh.parts), func(i int) bool { return sh.parts[i].at+sh.parts[i].n > row })
	if i == len(sh.parts) || sh.parts[i].at > row {
		return -1
	}

	return i
}

// pathPart is the index of the part drawing path's file, -1 when the sheet has none.
func (sh *sheet) pathPart(path string) int {
	for i := range sh.parts {
		if sh.parts[i].doc != nil && sh.parts[i].path == path {
			return i
		}
	}

	return -1
}

// hunkRows is every hunk label row, file by file.
func (sh *sheet) hunkRows() []int {
	out := make([]int, 0, len(sh.parts))

	for f := range sh.parts {
		out = append(out, sh.fileHunks(f)...)
	}

	return out
}

// fileHunks is where part f's hunk labels land, read the first time it is asked. Each hunk is parsed
// alone under the file's header, which counts its rows without drawing them, and the header's rows
// are found by drawing the first few rows of the first hunk. When the counts do not add up to the
// file's own, the file's rows are drawn and searched instead.
func (sh *sheet) fileHunks(f int) []int {
	if rows, ok := sh.hunks[f]; ok {
		return rows
	}

	p := sh.parts[f]
	if p.doc == nil {
		return nil
	}

	rows := sh.countHunks(p)
	if rows == nil {
		drawn, _ := p.doc.Peek(0, p.n)
		for _, r := range hunkLabels(drawn, []int{0}) {
			rows = append(rows, p.at+r)
		}
	}

	sh.hunks[f] = rows

	return rows
}

// headPeek is how many rows of a lone hunk are drawn to find its label under the file's header.
const headPeek = 4

// countHunks is part p's hunk label rows from each hunk parsed alone, nil when they cannot be
// counted that way.
func (sh *sheet) countHunks(p part) []int {
	head, hunks := splitHunks(p.text)
	if len(hunks) == 0 {
		return nil
	}

	lens := make([]int, len(hunks))
	for i, h := range hunks {
		lens[i] = render.New(head+h, sh.opts).Len()
	}

	first, _ := render.New(head+hunks[0], sh.opts).Peek(0, min(headPeek, lens[0]))

	labels := hunkLabels(first, []int{0})
	if len(labels) == 0 {
		return nil
	}

	lead, total := labels[0], labels[0]
	rows := make([]int, 0, len(hunks))

	for _, n := range lens {
		rows = append(rows, p.at+total)
		total += n - lead
	}

	if total != p.n {
		return nil
	}

	return rows
}

// splitHunks is a file's diff cut into its header and each hunk from its @@ line on.
func splitHunks(text string) (string, []string) {
	var (
		head  strings.Builder
		hunks []string
	)

	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "@@") {
			hunks = append(hunks, "")
		}

		if len(hunks) == 0 {
			head.WriteString(line)

			continue
		}

		hunks[len(hunks)-1] += line
	}

	return head.String(), hunks
}

// colorCache is the sheets a pane has drawn, most recent last, each keeping whatever of it has been
// lexed. It is shared by every copy of the pane and only touched on the update loop.
type colorCache struct {
	sheets map[drawKey]*sheet
	order  []drawKey
}

func newColorCache() *colorCache {
	return &colorCache{sheets: map[drawKey]*sheet{}}
}

func (c *colorCache) get(key drawKey) (*sheet, bool) {
	sh, ok := c.sheets[key]

	return sh, ok
}

func (c *colorCache) put(key drawKey, sh *sheet) {
	if _, ok := c.sheets[key]; !ok {
		c.order = append(c.order, key)
	}

	c.sheets[key] = sh

	for len(c.order) > coloredKeep {
		delete(c.sheets, c.order[0])
		c.order = c.order[1:]
	}
}

// keyFor is how this pane draws text at its width.
func (m *Model) keyFor(text string) drawKey {
	width := m.list.InnerWidth(m.width)
	if m.docked {
		width = m.width
	}

	return drawKey{text: text, width: width, sideBySide: m.sideBySide}
}

// Colorize starts lexing the files near the cursor that nothing has started on, and answers a command
// that waits for those lexers and nothing else, since a Document is not safe to draw from off the
// update loop. The parent calls it after every update, so whatever re-drew the pane or moved its
// cursor (a new diff, a resize, the side-by-side toggle, a scroll into a file not yet lexed) is
// colored without each path asking.
func (m *Model) Colorize() tea.Cmd {
	if !m.list.IsOpen() || m.sheet == nil {
		return nil
	}

	cursor := m.list.Cursor()

	parts, done := m.sheet.start(cursor-lexMargin, cursor+lexMargin)
	if len(parts) == 0 {
		return nil
	}

	msg := ColoredMsg{sheet: m.sheet, parts: parts, docked: m.docked}

	return func() tea.Msg {
		for _, ch := range done {
			<-ch
		}

		return msg
	}
}

// Colored marks the files whose lexers finished as lexed, so the next frame that draws them draws
// them in color, on show now or on a return to that diff. The rows on show keep their cursor, since
// only their colors change.
func (m *Model) Colored(msg ColoredMsg) {
	if msg.docked != m.docked || msg.sheet == nil {
		return
	}

	msg.sheet.landed(msg.parts)
}
