package diffpane

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/jj-diff/render"
)

// Syntax colors cost a lexer pass over every file a diff touches, which is tens of milliseconds for
// a small revision and over a second for a large one. The pane never pays that on the update loop: a
// diff is a render.Document per git segment, drawn with Peek, which never lexes and draws a file
// plain until Start has lexed it. Colorize starts the lexers and answers a command that only waits
// for them; Colored then draws the same documents again, on the loop, now in color. Peek draws the
// same rows in the same places either way, so a cursor kept across the swap stays on its line.

// coloredKeep is how many colored diffs a pane remembers, so stepping back to a revision the cursor
// just left draws it in color at once rather than plain and then again.
const coloredKeep = 8

// ColoredMsg says the lexers of a diff a pane drew plain have finished, so its rows can be drawn
// again in color. Docked names which pane asked, since the full-screen pane and the dock can each be
// waiting.
type ColoredMsg struct {
	key    drawKey
	docked bool
}

// drawKey is everything a diff's rows depend on beyond the skin, which a pane never changes.
type drawKey struct {
	text       string
	width      int
	sideBySide bool
}

// drawn is a diff as rows, with where each file header and hunk label landed.
type drawn struct {
	lines []string
	files []int
	paths []string
	hunks []int
}

// sheet is a diff parsed for drawing: its git segments as documents, and jj's prose between them as
// lines, in order. A segment's doc is nil for prose.
type sheet struct {
	parts []part
}

type part struct {
	doc   *render.Document
	text  string
	lines []string
}

// colorCache is the colored diffs a pane has, most recent last, and the sheets still being lexed. It
// is shared by every copy of the pane and only touched on the update loop.
type colorCache struct {
	rows     map[drawKey]drawn
	inflight map[drawKey]*sheet
	order    []drawKey
}

func newColorCache() *colorCache {
	return &colorCache{rows: map[drawKey]drawn{}, inflight: map[drawKey]*sheet{}}
}

func (c *colorCache) get(key drawKey) (drawn, bool) {
	d, ok := c.rows[key]

	return d, ok
}

func (c *colorCache) put(key drawKey, d drawn) {
	if _, ok := c.rows[key]; !ok {
		c.order = append(c.order, key)
	}

	c.rows[key] = d

	for len(c.order) > coloredKeep {
		delete(c.rows, c.order[0])
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

// Colorize starts lexing the diff on show when it is drawn plain and nothing is lexing it yet, and
// answers a command that waits for the lexers and nothing else, since a Document is not safe to
// draw from off the update loop. The parent calls it after every update, so whatever re-drew the pane
// (a new diff, a resize, the side-by-side toggle) is colored without each path asking.
func (m *Model) Colorize() tea.Cmd {
	if !m.plain || !m.list.IsOpen() || m.sheet == nil {
		return nil
	}

	if _, waiting := m.colors.inflight[m.key]; waiting {
		return nil
	}

	m.colors.inflight[m.key] = m.sheet

	var done []<-chan struct{}

	for _, p := range m.sheet.parts {
		if p.doc != nil {
			done = append(done, p.doc.Start())
		}
	}

	msg := ColoredMsg{key: m.key, docked: m.docked}

	return func() tea.Msg {
		for _, ch := range done {
			<-ch
		}

		return msg
	}
}

// Colored draws a diff whose lexers finished in color, swapping the rows in when it is the one on
// show, keeping the cursor, and keeps them either way for a return to that diff.
func (m *Model) Colored(msg ColoredMsg) {
	if msg.docked != m.docked {
		return
	}

	sh, ok := m.colors.inflight[msg.key]
	if !ok {
		return
	}

	delete(m.colors.inflight, msg.key)

	d, _ := m.drawSheet(sh)
	m.colors.put(msg.key, d)

	if !m.plain || msg.key != m.key || !m.list.IsOpen() {
		return
	}

	m.plain, m.sheet = false, nil
	m.files, m.paths, m.hunks = d.files, d.paths, d.hunks
	m.list.Open(m.title, d.lines, m.list.Cursor())
}

// parse splits key's diff into documents and prose, rendering nothing yet.
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

	sh := &sheet{}

	for _, seg := range segments(key.text) {
		if !seg.diff {
			sh.parts = append(sh.parts, part{lines: seg.lines})

			continue
		}

		text := strings.Join(seg.lines, "\n") + "\n"
		sh.parts = append(sh.parts, part{doc: render.New(text, opts), text: text})
	}

	return sh
}

// drawSheet draws every row of sh without lexing, and reports whether every file came out colored.
func (m *Model) drawSheet(sh *sheet) (drawn, bool) {
	var d drawn

	colored := true

	for _, p := range sh.parts {
		if p.doc == nil {
			for _, line := range p.lines {
				d.lines = append(d.lines, prose(m.skin, line, m.keyWidth()))
			}

			continue
		}

		rendered, ok := p.doc.Peek(0, p.doc.Len())
		colored = colored && ok
		bounds, paths := fileHeaders(p.text, rendered)

		for _, bound := range bounds {
			d.files = append(d.files, bound+len(d.lines))
		}

		d.paths = append(d.paths, paths...)

		for _, hunk := range hunkLabels(rendered, bounds) {
			d.hunks = append(d.hunks, hunk+len(d.lines))
		}

		d.lines = append(d.lines, rendered...)
	}

	return d, colored
}

// keyWidth is the width prose lines are cut to, the same the documents draw at.
func (m *Model) keyWidth() int { return m.keyFor("").width }
