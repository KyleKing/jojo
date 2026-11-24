package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/components/roundsview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/rounds"
)

// roundsState is the docked diff stepping through the versions of one change.
type roundsState struct {
	change string
	commit string
	list   []rounds.Round
	total  int
	// at is the round the dock shows, -1 for the change as it stands.
	at int
	// pending is what to do once a read of the versions lands: a step in a direction, or the box.
	pending roundsPending
}

type roundsPending struct {
	dir int
	box bool
}

// versionsMsg carries a change's evolution read in the background.
type versionsMsg struct {
	err      error
	change   string
	commit   string
	versions []jj.Version
	seq      int
}

// interdiffKey names one round's diff in the session cache: two commit ids name it for good.
func interdiffKey(r rounds.Round) string { return r.From + ".." + r.To }

// versionsKey is what a change's evolution is cached under: the commit it was read at, since a later
// rewrite adds a version and so names a new commit.
func versionsKey(rev *jj.Revision) string {
	if rev.CommitID != "" {
		return rev.CommitID
	}

	return rev.ChangeID
}

// stepRound moves the docked diff one version older (dir -1) or newer (dir 1) through the change it
// follows, reading the evolution first when this change has none on hand.
func (m *Model) stepRound(dir int) tea.Cmd {
	rev := m.dockTarget()
	if rev == nil {
		return nil
	}

	m.ensureDocked()

	if !m.roundsFor(rev) {
		return m.readVersions(rev, roundsPending{dir: dir})
	}

	return m.moveRound(dir)
}

// ensureDocked docks the diff when it is not, since rounds are drawn there.
func (m *Model) ensureDocked() {
	if m.docked {
		return
	}

	m.docked = true
	if m.outline.active {
		m.outline.openedDock = true
	}

	m.resize()
}

// roundsFor makes rev's rounds the ones on hand, from the session cache when it holds them, and
// reports whether they are.
func (m *Model) roundsFor(rev *jj.Revision) bool {
	key := versionsKey(rev)
	if m.rounds.commit == key && m.rounds.list != nil {
		return true
	}

	versions, ok := m.versions[key]
	if !ok {
		return false
	}

	m.setRounds(rev.ChangeID, key, versions)

	return true
}

func (m *Model) setRounds(change, commit string, versions []jj.Version) {
	at := -1
	if m.rounds.commit == commit {
		at = m.rounds.at
	}

	m.rounds = roundsState{change: change, commit: commit, list: rounds.Group(versions), total: len(versions), at: at}
}

func (m *Model) readVersions(rev *jj.Revision, then roundsPending) tea.Cmd {
	m.dockSeq++
	m.rounds.pending = then
	client, ctx, seq := m.client, m.ctx, m.dockSeq
	change, commit := rev.ChangeID, versionsKey(rev)

	return func() tea.Msg {
		versions, err := client.Versions(ctx, commit)

		return versionsMsg{change: change, commit: commit, versions: versions, err: err, seq: seq}
	}
}

// landVersions caches the evolution and does what was asked while it was read. A read the cursor
// has moved past is cached all the same, since the next ]r on that change would ask for it again.
func (m Model) landVersions(msg versionsMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if msg.seq == m.dockSeq {
			m.fail(msg.err)
		}

		return m, nil
	}

	m.versions[msg.commit] = msg.versions

	if msg.seq != m.dockSeq {
		return m, nil
	}

	pending := m.rounds.pending
	m.setRounds(msg.change, msg.commit, msg.versions)

	if pending.box {
		m.roundsBox.Open(msg.change, m.rounds.list, m.rounds.total, m.rounds.at)

		return m, nil
	}

	cmd := m.moveRound(pending.dir)

	return m, cmd
}

// moveRound steps from the round the dock shows. Older than the first version stays put, and newer
// than the newest is the change as it stands again.
func (m *Model) moveRound(dir int) tea.Cmd {
	r := &m.rounds
	if len(r.list) == 0 {
		return m.say("no versions to step through")
	}

	switch {
	case r.at < 0 && dir > 0:
		return m.say("showing the change as it stands; [r steps back through its versions")
	case r.at < 0:
		return m.showRound(len(r.list) - 1)
	case r.at+dir < 0:
		return m.say("the first version: nothing before it")
	case r.at+dir >= len(r.list):
		return m.showWhole()
	}

	return m.showRound(r.at + dir)
}

// showWhole puts the change as it stands back in the dock.
func (m *Model) showWhole() tea.Cmd {
	m.rounds.at = -1

	return m.loadDock(m.dockTarget())
}

// showRound puts round i in the dock: its interdiff from the version before it, or the first
// version's own diff, under a banner naming the version and what made it. A round read once this
// session is drawn at once.
func (m *Model) showRound(i int) tea.Cmd {
	r := m.rounds.list[i]
	m.rounds.at = i
	m.dockSeq++

	title := r.Banner(m.rounds.total)
	key := interdiffKey(r)

	if text, ok := m.interdiffs[key]; ok {
		leaveChange(m.dockReads())
		m.showDockText(title, r.Prose()+text, "")

		return nil
	}

	client, ctx, seq := m.client, m.ctx, m.dockSeq

	return func() tea.Msg {
		var (
			text string
			err  error
		)

		if r.From == "" {
			text, err = client.CommitDiff(ctx, r.To)
		} else {
			text, err = client.Interdiff(ctx, r.From, r.To)
		}

		return dockLoadedMsg{
			revision: r.To, title: title, prose: r.Prose(), text: text, err: err, seq: seq, cacheKey: key,
		}
	}
}

// openRoundsBox lists the rounds of the change the dock follows, reading them first when needed.
func (m *Model) openRoundsBox() tea.Cmd {
	rev := m.dockTarget()
	if rev == nil {
		return nil
	}

	if !m.roundsFor(rev) {
		return m.readVersions(rev, roundsPending{box: true})
	}

	m.roundsBox.Open(rev.ChangeID, m.rounds.list, m.rounds.total, m.rounds.at)

	return nil
}

// handleRoundsMsg covers what the rounds box and the dock's motions report.
func (m Model) handleRoundsMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case versionsMsg:
		next, cmd := m.landVersions(msg)

		return next, cmd, true
	case outlineLoadedMsg:
		next, cmd := m.landOutline(msg)

		return next, cmd, true
	case roundsview.PickedMsg:
		if msg.Index < 0 || msg.Index >= len(m.rounds.list) {
			return m, nil, true
		}

		m.ensureDocked()
		cmd := m.showRound(msg.Index)

		return m, cmd, true
	case roundsview.WholeMsg:
		cmd := m.showWhole()

		return m, cmd, true
	case roundsview.RestoreMsg:
		next, cmd := m.restoreVersion(msg)

		return next, cmd, true
	case roundsview.CancelledMsg:
		return m, nil, true
	case diffpane.MotionMsg:
		cmd := m.dockMotion(msg)

		return m, cmd, true
	}

	return m, nil, false
}

// dockMotion is a motion the docked diff passed up, over the versions or the chapters it shows.
func (m *Model) dockMotion(msg diffpane.MotionMsg) tea.Cmd {
	if msg.Object == diffpane.ObjectUnread {
		return m.stepUnread(m.activeReads(), msg.Dir)
	}

	if m.diff.IsOpen() {
		// The diff drawn over the log has no versions or chapters of its own.
		return nil
	}

	switch msg.Object {
	case diffpane.ObjectRound:
		return m.stepRound(msg.Dir)
	case diffpane.ObjectChapter:
		return m.stepChapter(msg.Dir)
	}

	return nil
}
