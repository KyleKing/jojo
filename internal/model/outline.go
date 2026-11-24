package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/outlineview"
	"github.com/kyleking/jojo/internal/jj"
)

// DefaultOutlineRevset is the stack the outline reads when config.toml names none: what the revision
// under the cursor sits on above trunk, and what has been stacked on it since. {rev} is that
// revision.
const DefaultOutlineRevset = "(trunk()..{rev}) | ({rev}:: ~ ::trunk())"

// outlineLimit caps a stack, which is a review's worth of revisions rather than a history.
const outlineLimit = 200

// outlineState is the stack outline while it takes the log's place.
type outlineState struct {
	revset string
	view   outlineview.Model
	seq    int
	active bool
	// openedDock says the outline docked the diff itself, so closing it gives the room back.
	openedDock bool
}

// outlineLoadedMsg carries a stack read in the background, its sizes and forge state with it.
type outlineLoadedMsg struct {
	err    error
	stats  map[string]jj.Stats
	forge  map[string]string
	revset string
	revs   []jj.Revision
	seq    int
}

// outlineRevsetFor is the revset template with the revision under the cursor put in, @ when there
// is none.
func outlineRevsetFor(template, changeID string) string {
	if template == "" {
		template = DefaultOutlineRevset
	}

	if changeID == "" {
		changeID = "@"
	}

	return strings.ReplaceAll(template, "{rev}", changeID)
}

// openOutline reads the stack at the cursor. The outline appears when the read lands, so a slow
// read leaves the log in place rather than an empty frame.
func (m *Model) openOutline() tea.Cmd {
	id := ""
	if rev := m.log.Selected(); rev != nil {
		id = rev.ChangeID
	}

	return m.readOutline(outlineRevsetFor(m.outlineTemplate, id))
}

// readOutline reads revset as chapters: the graph's revisions parent first, the stats of each and
// what the forge pane's caches already hold for its bookmarks, in one background command.
func (m *Model) readOutline(revset string) tea.Cmd {
	m.outline.seq++
	client, ctx, seq, lookup := m.client, m.ctx, m.outline.seq, m.forge.Lookup()

	return func() tea.Msg {
		rows, err := client.Log(ctx, revset, outlineLimit)
		if err != nil {
			return outlineLoadedMsg{err: err, seq: seq, revset: revset}
		}

		revs := chaptersOf(rows)
		ids := make([]string, 0, len(revs))
		forge := map[string]string{}

		for i := range revs {
			rev := &revs[i]
			ids = append(ids, rev.CommitID)

			if lookup == nil {
				continue
			}

			for _, name := range rev.LocalBookmarks {
				if state := lookup(name); state != "" {
					forge[rev.ChangeID] = state

					break
				}
			}
		}

		stats, err := client.Stats(ctx, ids)

		return outlineLoadedMsg{revs: revs, stats: stats, forge: forge, revset: revset, seq: seq, err: err}
	}
}

// chaptersOf is the revisions of a log, parent first: jj lists children above their parents.
func chaptersOf(rows []jj.LogRow) []jj.Revision {
	var revs []jj.Revision

	for _, row := range rows {
		if row.Revision != nil {
			revs = append(revs, *row.Revision)
		}
	}

	slices.Reverse(revs)

	return revs
}

// landOutline fills the outline and docks the diff beside it on the chapter under the cursor.
func (m Model) landOutline(msg outlineLoadedMsg) (Model, tea.Cmd) {
	if msg.seq != m.outline.seq {
		return m, nil
	}

	if msg.err != nil {
		m.submit.afterOutline = false
		m.fail(msg.err)

		return m, nil
	}

	chapters := make([]outlineview.Chapter, len(msg.revs))

	for i := range msg.revs {
		rev := &msg.revs[i]
		if stats, ok := msg.stats[rev.CommitID]; ok {
			m.stats[rev.CommitID] = stats
			rev.Added, rev.Removed, rev.Files = stats.Added, stats.Removed, stats.Files
		}

		chapters[i] = outlineview.Chapter{Rev: rev, Forge: msg.forge[rev.ChangeID], Read: m.strips[rev.ChangeID]}
	}

	opening := !m.outline.active
	m.outline.view.SetChapters(chapters)

	// The outline opens on the revision the log cursor was on, which is the chapter being asked about.
	if rev := m.log.Selected(); opening && rev != nil {
		m.outline.view.Focus(rev.ChangeID)
	}
	m.outline.revset, m.outline.active = msg.revset, true

	if opening && !m.docked {
		m.docked, m.outline.openedDock = true, true
	}

	if opening && m.focus != focusDock {
		m.focus = focusLog
	}

	m.showPRs()
	m.resize()
	cmd := m.loadDock(m.dockTarget())

	// The stack's strips are counted behind the first frame, so ]u knows where to go before the dock
	// has read each chapter.
	if opening {
		cmd = tea.Batch(cmd, m.countStack())
	}

	if m.submit.afterOutline {
		m.submit.afterOutline = false
		if a := m.submitGate(); !a.OK {
			return m, tea.Batch(cmd, m.say(m.keys.Label(help.ActionSubmit)+"  "+a.Why))
		}

		return m, tea.Batch(cmd, m.startSubmit())
	}

	return m, cmd
}

// closeOutline gives the log back, on the chapter the outline was on.
func (m *Model) closeOutline() tea.Cmd {
	if rev := m.outline.view.Selected(); rev != nil {
		m.log.Focus(rev.ChangeID)
	}

	m.outline.active = false
	m.outline.seq++

	if m.outline.openedDock {
		m.docked, m.outline.openedDock = false, false
		m.dock.Close()
	}

	if m.focus == focusDock && !m.docked {
		m.focus = focusLog
	}

	m.resize()

	return m.loadDock(m.dockTarget())
}

// dockTarget is the revision the docked diff follows: the outline's chapter while it is open, the
// log's cursor otherwise.
func (m Model) dockTarget() *jj.Revision {
	if m.outline.active {
		return m.outline.view.Selected()
	}

	return m.log.Selected()
}

// dispatchOutline owns the keys while the outline has the focus. It reads, so a write verb pressed
// by habit says where the writes are rather than acting on a chapter it cannot redraw.
func (m Model) dispatchOutline(action string) (tea.Model, tea.Cmd) {
	if key, ok := navKey(action); ok {
		moved, changed := m.outline.view.MoveKey(key.String())
		if !moved || !changed {
			return m, nil
		}

		cmd := m.followDock(m.dockTarget())

		return m, cmd
	}

	switch action {
	case keys.NextAction(keys.ObjChapter), keys.PrevAction(keys.ObjChapter):
		cmd := m.stepChapter(actionDir(action))

		return m, cmd
	case keys.NextAction(keys.ObjHunk), keys.PrevAction(keys.ObjHunk):
		m.dock.StepHunk(actionDir(action))

		return m, nil
	case keys.NextAction(keys.ObjRound), keys.PrevAction(keys.ObjRound):
		cmd := m.stepRound(actionDir(action))

		return m, cmd
	case keys.ActionRounds:
		cmd := m.openRoundsBox()

		return m, cmd
	case help.ActionChapterRead:
		m.focus = focusDock

		return m, nil
	case keys.ActionRead:
		cmd := m.markChapter()

		return m, cmd
	case keys.NextAction(keys.ObjUnread), keys.PrevAction(keys.ObjUnread):
		return m.stepUnreadRev(help.Outline, actionDir(action))
	case keys.ActionOnlyUnread:
		cmd := m.toggleOnlyUnread()

		return m, cmd
	case help.ActionSubmit:
		cmd := m.startSubmit()

		return m, cmd
	case keys.ActionBack, keys.ActionOutline:
		cmd := m.closeOutline()

		return m, cmd
	}

	if next, cmd, handled := m.globalAction(action); handled {
		return next, cmd
	}

	return m, m.say("the outline reads; esc returns to the log, where the verbs are")
}

// stepChapter moves the outline's cursor one chapter, the docked diff following once it rests.
func (m *Model) stepChapter(dir int) tea.Cmd {
	if !m.outline.active {
		return m.say("]] steps chapters in the outline: I opens it")
	}

	if !m.outline.view.Move(dir) {
		if dir > 0 {
			return m.say("the last chapter")
		}

		return m.say("the first chapter")
	}

	return m.followDock(m.dockTarget())
}
