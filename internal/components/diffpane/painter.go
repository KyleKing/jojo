package diffpane

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/jj-diff/render"

	"github.com/kyleking/jojo/internal/ui"
)

// Syntax colors cost a lexer pass over every file a diff touches, which is tens of milliseconds for
// a small revision and over a second for a large one. The pane never pays that on the update loop:
// it draws a diff plain, asks for the colored rows in the background, and swaps them in when they
// land. A loader that reads a diff off the loop already can draw it there too (Painter.Paint), so a
// small diff arrives colored and the pane never shows it plain at all.

// colorBudget is how long Paint waits for the colored rows before handing over plain ones. It is
// short enough that a diff waiting on it still lands within a frame or two of its read finishing,
// and long enough that an ordinary revision never flashes plain before its colors.
const colorBudget = 60 * time.Millisecond

// coloredKeep is how many colored diffs a pane remembers, so stepping back to a revision the cursor
// just left draws it in color at once rather than plain and then again.
const coloredKeep = 8

// ColoredMsg carries a diff drawn with syntax colors, lexed off the update loop. Docked names which
// pane asked, since the full-screen pane and the dock can each be waiting.
type ColoredMsg struct {
	drawn  drawn
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

// colorCache is the colored diffs a pane has, most recent last, and the ones still being drawn. It is
// shared by every copy of the pane and only touched on the update loop.
type colorCache struct {
	order    []drawKey
	rows     map[drawKey]drawn
	inflight map[drawKey]bool
	// plain is the one diff drawn plain off the loop while its colors are drawn, waiting for Show.
	plain    drawn
	plainKey drawKey
}

// take returns the plain rows drawn off the loop for key, once, since only the next Show wants them.
func (c *colorCache) take(key drawKey) (drawn, bool) {
	if c.plainKey != key || c.plain.lines == nil {
		return drawn{}, false
	}

	d := c.plain
	c.plain, c.plainKey = drawn{}, drawKey{}

	return d, true
}

func newColorCache() *colorCache {
	return &colorCache{rows: map[drawKey]drawn{}, inflight: map[drawKey]bool{}}
}

func (c *colorCache) get(key drawKey) (drawn, bool) {
	d, ok := c.rows[key]

	return d, ok
}

func (c *colorCache) put(key drawKey, d drawn) {
	delete(c.inflight, key)

	if _, ok := c.rows[key]; !ok {
		c.order = append(c.order, key)
	}

	c.rows[key] = d

	for len(c.order) > coloredKeep {
		delete(c.rows, c.order[0])
		c.order = c.order[1:]
	}
}

// Painter is what drawing a diff needs from a pane, copied out of it so a command can draw off the
// update loop without reading the model.
type Painter struct {
	skin   ui.Skin
	key    drawKey
	docked bool
}

// Painter returns how this pane would draw a diff at width, which is what Show and Open take.
func (m Model) Painter(width int) Painter {
	lineWidth := m.list.InnerWidth(width)
	if m.docked {
		lineWidth = width
	}

	return Painter{skin: m.skin, key: drawKey{width: lineWidth, sideBySide: m.sideBySide}, docked: m.docked}
}

// Paint is a diff drawn by Painter.Paint: its colored rows if they were ready within the budget, else
// its plain rows and a command that delivers the colored ones as a ColoredMsg once they are.
type Paint struct {
	plain drawn
	later tea.Cmd
	msg   ColoredMsg
	ready bool
}

// Paint draws gitDiff in color, waiting up to the color budget for it. It is meant for a command
// that has just read the diff in the background, so the wait never holds the update loop.
func (p Painter) Paint(gitDiff string) Paint {
	done := make(chan ColoredMsg, 1)

	go func() { done <- p.colored(gitDiff) }()

	timer := time.NewTimer(colorBudget)
	defer timer.Stop()

	select {
	case msg := <-done:
		return Paint{msg: msg, ready: true}
	case <-timer.C:
		key := p.withText(gitDiff)

		return Paint{
			msg:   ColoredMsg{key: key, docked: p.docked},
			plain: p.draw(key, false),
			later: func() tea.Msg { return <-done },
		}
	}
}

// Accept takes a Paint for a diff about to be shown, so Show draws nothing itself: it finds the
// colored rows, or the plain ones with the colors on their way. The command it returns delivers
// colors that were not ready yet.
func (m *Model) Accept(p Paint) tea.Cmd {
	if p.ready {
		m.colors.put(p.msg.key, p.msg.drawn)

		return nil
	}

	if p.later == nil {
		return nil
	}

	m.colors.inflight[p.msg.key] = true
	m.colors.plain, m.colors.plainKey = p.plain, p.msg.key

	return p.later
}

// Colorize asks for the colored rows of the diff on show, when it is drawn plain and nothing is
// drawing them yet. The parent calls it after every update, so whatever re-drew the pane (a new
// diff, a resize, the side-by-side toggle) is colored without each path asking.
func (m *Model) Colorize() tea.Cmd {
	if !m.plain || !m.list.IsOpen() || m.colors.inflight[m.key] {
		return nil
	}

	m.colors.inflight[m.key] = true
	p := Painter{skin: m.skin, key: m.key, docked: m.docked}

	return func() tea.Msg { return p.colored(p.key.text) }
}

// Colored swaps in colored rows when they are for the diff on show, keeping the cursor, and keeps
// them either way for a return to that diff.
func (m *Model) Colored(msg ColoredMsg) {
	if msg.docked != m.docked {
		return
	}

	m.colors.put(msg.key, msg.drawn)

	if !m.plain || msg.key != m.key || !m.list.IsOpen() {
		return
	}

	m.plain = false
	m.files, m.paths, m.hunks = msg.drawn.files, msg.drawn.paths, msg.drawn.hunks
	m.list.Open(m.title, msg.drawn.lines, m.list.Cursor())
}

func (p Painter) withText(text string) drawKey {
	key := p.key
	key.text = text

	return key
}

func (p Painter) colored(text string) ColoredMsg {
	key := p.withText(text)

	return ColoredMsg{drawn: p.draw(key, true), key: key, docked: p.docked}
}

// draw renders key's diff, with syntax colors or without. Both draw the same rows in the same
// places, since highlighting only colors what a row already holds, so a cursor kept across the swap
// stays on the same line.
func (p Painter) draw(key drawKey, syntax bool) drawn {
	opts := render.Options{
		Width:      key.width,
		Palette:    paletteFor(p.skin),
		Syntax:     syntax,
		WordDiff:   true,
		SideBySide: key.sideBySide,
		SignBar:    p.skin.SignBar(),
		HunkLabels: true,
	}

	var d drawn

	for _, seg := range segments(key.text) {
		if !seg.diff {
			for _, line := range seg.lines {
				d.lines = append(d.lines, prose(p.skin, line, key.width))
			}

			continue
		}

		text := strings.Join(seg.lines, "\n") + "\n"
		rendered := render.Lines(text, opts)
		bounds, paths := fileHeaders(text, rendered)

		for _, bound := range bounds {
			d.files = append(d.files, bound+len(d.lines))
		}

		d.paths = append(d.paths, paths...)

		for _, hunk := range hunkLabels(rendered, bounds) {
			d.hunks = append(d.hunks, hunk+len(d.lines))
		}

		d.lines = append(d.lines, rendered...)
	}

	return d
}
