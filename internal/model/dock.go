package model

import (
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/seen"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// dockDebounce is how long the cursor has to rest on a revision before the docked diff reads it. A
// diff is one jj call in the shared queue, so holding j would otherwise queue one per row passed; a
// held key repeats about every 33 ms, so this is just long enough to let a held key pass.
const dockDebounce = 40 * time.Millisecond

// dockCacheSize bounds the diffs the dock keeps by commit id. A commit id names its content, so an
// entry never goes stale, only old; past the bound the cache starts over.
const dockCacheSize = 256

// outlineMaxShare is the most of a wide terminal the outline takes beside the dock, which gets the
// rest: the outline's rows are short, so past its own width a share would only be blank. It may
// stretch to half the terminal rather than drop a column, since a dropped size or bookmark is
// nowhere else on screen.
const (
	outlineMaxShare     = 45
	outlineStretchShare = 50
	percent             = 100
)

// dockMinSideBySide is the narrowest terminal that docks the diff beside the log rather than under
// it, since below it neither half has room for a readable row.
const dockMinSideBySide = 100

// dockTickMsg fires once the cursor has rested. Seq names the request, so a tick a later move has
// overtaken does nothing.
type dockTickMsg struct {
	revision string
	commit   string
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
	// commit is the commit a whole diff was read at, "" when the cursor's revision did not say.
	commit string
	// path names the file a history narrowed the diff to, "" for the revision's whole diff.
	path  string
	hunks []seen.Ref
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
	client, ctx, seq, id, commit := m.client, m.ctx, m.dockSeq, rev.ChangeID, rev.CommitID
	paths := m.historyPaths()

	if cached, ok := m.cachedDock(rev); ok {
		cached.seq = seq

		return func() tea.Msg { return cached }
	}

	path := ""
	if len(paths) > 0 {
		path = m.history.file.Names[0]
	}

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: id, Paths: paths})
		if err != nil {
			return dockLoadedMsg{revision: id, err: err, seq: seq}
		}

		if path != "" {
			return dockLoadedMsg{revision: id, text: text, seq: seq, path: path}
		}

		return dockLoadedMsg{revision: id, commit: commit, text: text, seq: seq, hunks: seen.Parse([]byte(text))}
	}
}

// followDock is what a cursor move asks of the docked diff: a read once the cursor rests there.
func (m *Model) followDock(rev *jj.Revision) tea.Cmd {
	if !m.docked || rev == nil {
		return nil
	}

	// A diff already read draws at once: the debounce only spares jj a read.
	if _, ok := m.cachedDock(rev); ok {
		return m.loadDock(rev)
	}

	m.dockSeq++
	tick := dockTickMsg{revision: rev.ChangeID, commit: rev.CommitID, seq: m.dockSeq}

	return m.after(dockDebounce, func(time.Time) tea.Msg { return tick })
}

// cachedDock is rev's whole diff as the dock last read it at this commit, when it has.
func (m *Model) cachedDock(rev *jj.Revision) (dockLoadedMsg, bool) {
	if rev.CommitID == "" || len(m.historyPaths()) > 0 {
		return dockLoadedMsg{}, false
	}

	cached, ok := m.dockCache[rev.CommitID]
	if ok {
		// The change id is the row's, which can grow a character once another change shares it.
		cached.revision = rev.ChangeID
	}

	return cached, ok
}

// keepDock files a whole diff read at a known commit, which the update loop alone writes.
func (m *Model) keepDock(msg dockLoadedMsg) {
	if msg.commit == "" || msg.path != "" || msg.cacheKey != "" || msg.err != nil {
		return
	}

	if len(m.dockCache) >= dockCacheSize {
		clear(m.dockCache)
	}

	m.dockCache[msg.commit] = msg
}

func (m Model) handleDockMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case dockTickMsg:
		if msg.seq != m.dockSeq {
			return m, nil, true
		}

		cmd := m.loadDock(&jj.Revision{ChangeID: msg.revision, CommitID: msg.commit})

		return m, cmd, true
	case dockLoadedMsg:
		return m.showDock(msg), nil, true
	}

	return m, nil, false
}

// showDock keeps the file the reader was in when the new revision touches it, and drops a read the
// cursor has already moved past, which would otherwise flash an old revision over the new one. Its
// colors follow through Colorize, which Update asks for after every update.
func (m Model) showDock(msg dockLoadedMsg) Model {
	if msg.seq != m.dockSeq || !m.docked {
		return m
	}

	if msg.err != nil {
		if !errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			m.fail(msg.err)
		}

		return m
	}

	m.dockShown = msg.revision

	// One file's diff is not the change's whole diff, so it files no read marks.
	if msg.path != "" {
		leaveChange(m.dockReads())
		m.showDockText("diff "+msg.revision+" · "+msg.path, msg.text, msg.path)

		return m
	}

	if msg.cacheKey != "" {
		m.interdiffs[msg.cacheKey] = msg.text
		leaveChange(m.dockReads())
		m.showDockText(msg.title, msg.prose+msg.text, "")

		return m
	}

	m.keepDock(msg)
	m.noteRead(msg.revision, msg.commit, msg.hunks)
	m.showChange(m.dockReads(), msg.revision, "diff "+msg.revision, msg.text, m.dock.CurrentPath())

	return m
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
		return m.width - m.outlineWidth() - besideMargin, logHeight
	}

	if m.width >= dockMinSideBySide {
		return m.width - m.width/2, logHeight
	}

	return m.width, logHeight / 2 //nolint:mnd // the log and the diff share the rows evenly.
}

// outlineWidth is the outline's share of a wide terminal beside the dock: its own width, up to the
// cap, past which its rows drop a size, then a bookmark, before a description goes under its least.
// The share stretches toward half the terminal to keep every column, or failing that the description
// and the read strip whole, since a dropped column is nowhere else on screen.
func (m Model) outlineWidth() int {
	share, most := m.width*outlineMaxShare/percent, m.width*outlineStretchShare/percent

	for _, least := range []int{m.outline.view.MinWidth(), m.outline.view.EssentialWidth()} {
		if least > share && least <= most {
			share = least

			break
		}
	}

	return min(m.outline.view.ContentWidth(), share)
}

// dockSideBySide reports whether the docked pane sits beside the log rather than under it.
func (m Model) dockSideBySide() bool { return m.width >= dockMinSideBySide }

// dispatchView runs an action in the docked diff while it has the focus, or in the full-screen diff
// over the log, which read the same keys, the read marks' included.
func (m Model) dispatchView(action string) (tea.Model, tea.Cmd) {
	pane, docked := &m.dock, m.openModal() != ctxDiff
	if !docked {
		pane = &m.diff
	}

	if key, ok := navKey(action); ok {
		cmd := pane.Update(key)
		m.settleHeld()

		return m, cmd
	}

	if object, dir, ok := paneMotion(action); ok {
		cmd := pane.Motion(object, dir)
		m.settleHeld()

		return m, cmd
	}

	switch action {
	case help.ActionHistory:
		return m.startHistory()
	case keys.ActionAgain, keys.ActionReverse:
		cmd := pane.Repeat(map[bool]int{true: 1, false: -1}[action == keys.ActionAgain])
		m.settleHeld()

		return m, cmd
	case keys.ActionRead:
		cmd := m.markPane(m.activeReads())

		return m, cmd
	case keys.ActionOnlyUnread:
		cmd := m.toggleOnlyUnread()

		return m, cmd
	case keys.ActionViewSideBySide:
		pane.ToggleSideBySide()

		return m, nil
	case keys.ActionBack, keys.ActionQuit:
		if !docked {
			return m, pane.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		}

		m.focus = focusLog

		return m, nil
	case keys.ActionRounds:
		cmd := m.openRoundsBox()

		return m, cmd
	}

	if next, cmd, handled := m.globalAction(action); handled {
		return next, cmd
	}

	return m, nil
}

// paneObjects are the diff pane's names for the shared ] and [ objects.
var paneObjects = map[string]string{
	keys.ObjHunk: diffpane.ObjectHunk, keys.ObjFile: diffpane.ObjectFile,
	keys.ObjRound: diffpane.ObjectRound, keys.ObjChapter: diffpane.ObjectChapter,
	keys.ObjUnread: diffpane.ObjectUnread,
}

// paneMotion is the diff pane's object and direction for a shared motion action.
func paneMotion(action string) (string, int, bool) {
	noun, ok := strings.CutSuffix(action, "-next")
	dir := 1

	if !ok {
		noun, ok = strings.CutSuffix(action, "-prev")
		dir = -1
	}

	object, known := paneObjects[noun]

	return object, dir, ok && known
}
