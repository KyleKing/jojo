package model

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/readmark"
)

// dockDebounce is how long the cursor has to rest on a revision before the docked diff reads it. A
// diff is one jj call in the shared queue, so holding j would otherwise queue one per row passed.
const dockDebounce = 120 * time.Millisecond

// outlineDockShare is the percent of a wide terminal the dock takes beside the outline.
const (
	outlineDockShare = 45
	percent          = 100
)

// dockMinSideBySide is the narrowest terminal that docks the diff beside the log rather than under
// it, since below it neither half has room for a readable row.
const dockMinSideBySide = 100

// dockTickMsg fires once the cursor has rested. Seq names the request, so a tick a later move has
// overtaken does nothing.
type dockTickMsg struct {
	revision string
	seq      int
}

// dockLoadedMsg carries a docked diff read in the background: a revision's whole diff, with its hunks
// named for the read strip, or one round of its evolution, under a title of its own.
type dockLoadedMsg struct {
	err      error
	revision string
	text     string
	// title and prose are a round's banner and the sentences drawn above its interdiff, and cacheKey
	// the name its diff is kept under for the session. All three are empty for a whole diff.
	title    string
	prose    string
	cacheKey string
	hunks    []readmark.Hunk
	// paint is the diff already drawn in color off the loop, or on its way.
	paint diffpane.Paint
	seq   int
}

// toggleDock opens the diff beside the log, reading the revision under the cursor at once, or closes
// it and gives the log back the room.
func (m Model) toggleDock() (tea.Model, tea.Cmd) {
	m.docked = !m.docked

	if !m.docked {
		m.dock.Close()

		if m.focus == focusDock {
			m.focus = focusLog
		}

		m.resize()

		return m, nil
	}

	m.resize()
	cmd := m.loadDock(m.dockTarget())

	return m, cmd
}

// loadDock reads rev's diff now, superseding any read still pending.
func (m *Model) loadDock(rev *jj.Revision) tea.Cmd {
	if !m.docked || rev == nil {
		return nil
	}

	m.dockSeq++
	m.rounds.at = -1
	client, ctx, seq, id, painter := m.client, m.ctx, m.dockSeq, rev.ChangeID, m.dockPainter()

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: id})
		if err != nil {
			return dockLoadedMsg{revision: id, err: err, seq: seq}
		}

		return dockLoadedMsg{
			revision: id, text: text, seq: seq, hunks: readmark.Hunks(text), paint: painter.Paint(text),
		}
	}
}

// followDock is what a cursor move asks of the docked diff: a read once the cursor rests there.
func (m *Model) followDock(rev *jj.Revision) tea.Cmd {
	if !m.docked || rev == nil {
		return nil
	}

	m.dockSeq++
	tick := dockTickMsg{revision: rev.ChangeID, seq: m.dockSeq}

	return tea.Tick(dockDebounce, func(time.Time) tea.Msg { return tick })
}

func (m Model) handleDockMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case dockTickMsg:
		if msg.seq != m.dockSeq {
			return m, nil, true
		}

		cmd := m.loadDock(&jj.Revision{ChangeID: msg.revision})

		return m, cmd, true
	case dockLoadedMsg:
		next, cmd := m.showDock(msg)

		return next, cmd, true
	}

	return m, nil, false
}

// showDock keeps the file the reader was in when the new revision touches it, and drops a read the
// cursor has already moved past, which would otherwise flash an old revision over the new one. The
// command delivers the diff's colors when they were not ready by the time it was read.
func (m Model) showDock(msg dockLoadedMsg) (Model, tea.Cmd) {
	if msg.seq != m.dockSeq || !m.docked {
		return m, nil
	}

	if msg.err != nil {
		if !errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			m.fail(msg.err)
		}

		return m, nil
	}

	if msg.cacheKey != "" {
		m.interdiffs[msg.cacheKey] = msg.text
		m.showDockText(msg.title, msg.prose+msg.text, "")

		return m, nil
	}

	cmd := m.dock.Accept(msg.paint)

	m.noteRead(msg.revision, msg.hunks)
	m.showDockText("diff "+msg.revision, msg.text, m.dock.CurrentPath())

	return m, cmd
}

// dockPainter is how the dock would draw a diff at its current size, for a read to draw off the loop.
func (m Model) dockPainter() diffpane.Painter {
	width, _ := m.dockSize()

	return m.dock.Painter(diffpane.DockWidth(width))
}

// showDockText draws text in the dock under title, landing on keep's file when the text has it.
func (m *Model) showDockText(title, text, keep string) {
	width, _ := m.dockSize()
	m.dock.Show(title, text, diffpane.DockWidth(width), keep)
}

// dockSize is the docked pane's width and height for the current terminal.
func (m Model) dockSize() (int, int) {
	logHeight := m.logHeight()
	if m.width >= dockMinSideBySide && m.outline.active {
		// The outline's rows carry more columns than the log's, so it keeps the larger share.
		return m.width * outlineDockShare / percent, logHeight
	}

	if m.width >= dockMinSideBySide {
		return m.width - m.width/2, logHeight
	}

	return m.width, logHeight / 2 //nolint:mnd // the log and the diff share the rows evenly.
}

// dockSideBySide reports whether the docked pane sits beside the log rather than under it.
func (m Model) dockSideBySide() bool { return m.width >= dockMinSideBySide }

func (m Model) handleDockKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key := m.keys.Resolve(msg.String()); key {
	case keyEsc, "q":
		m.focus = focusLog

		return m, nil
	case keyTab:
		return m.cycleFocus()
	case dockKey:
		return m.toggleDock()
	case "H":
		cmd := m.openRoundsBox()

		return m, cmd
	}

	cmd := m.dock.Update(msg)

	return m, cmd
}

// dockKey is the binding that docks the diff beside the log.
const dockKey = "alt+p"
