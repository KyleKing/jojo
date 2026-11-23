package model

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/jj"
)

// dockDebounce is how long the cursor has to rest on a revision before the docked diff reads it. A
// diff is one jj call in the shared queue, so holding j would otherwise queue one per row passed.
const dockDebounce = 120 * time.Millisecond

// dockMinSideBySide is the narrowest terminal that docks the diff beside the log rather than under
// it, since below it neither half has room for a readable row.
const dockMinSideBySide = 100

// dockTickMsg fires once the cursor has rested. Seq names the request, so a tick a later move has
// overtaken does nothing.
type dockTickMsg struct {
	revision string
	seq      int
}

// dockLoadedMsg carries a docked diff read in the background.
type dockLoadedMsg struct {
	err      error
	revision string
	text     string
	seq      int
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
	cmd := m.loadDock(m.log.Selected())

	return m, cmd
}

// loadDock reads rev's diff now, superseding any read still pending.
func (m *Model) loadDock(rev *jj.Revision) tea.Cmd {
	if !m.docked || rev == nil {
		return nil
	}

	m.dockSeq++
	client, ctx, seq, id := m.client, m.ctx, m.dockSeq, rev.ChangeID

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: id})

		return dockLoadedMsg{revision: id, text: text, err: err, seq: seq}
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
		return m.showDock(msg), nil, true
	}

	return m, nil, false
}

// showDock keeps the file the reader was in when the new revision touches it, and drops a read the
// cursor has already moved past, which would otherwise flash an old revision over the new one.
func (m Model) showDock(msg dockLoadedMsg) Model {
	if msg.seq != m.dockSeq || !m.docked {
		return m
	}

	if msg.err != nil {
		if !errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			m.message = msg.err.Error()
		}

		return m
	}

	width, _ := m.dockSize()
	m.dock.Show("diff "+msg.revision, msg.text, diffpane.DockWidth(width), m.dock.CurrentPath())

	return m
}

// dockSize is the docked pane's width and height for the current terminal.
func (m Model) dockSize() (int, int) {
	logHeight := m.logHeight()
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
	}

	cmd := m.dock.Update(msg)

	return m, cmd
}

// dockKey is the binding that docks the diff beside the log.
const dockKey = "alt+p"
