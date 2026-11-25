package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/seen"

	"github.com/kyleking/jojo/internal/jj"
)

// statsAhead is how many rows below the screen get their stats with it, so a short scroll lands on
// rows already filled in.
const statsAhead = 40

// statsMsg carries the diff stats of the commits a read named, keyed by commit id.
type statsMsg struct {
	err   error
	stats map[string]jj.Stats
	asked []string
}

// statsCache keeps every commit's stats for the session. A commit id names its content, so an entry
// never goes stale, and a reload after a write reads only the commits the write created.
type statsCache map[string]jj.Stats

// fillStats puts what the cache knows into the rows, which share their revisions with the log view,
// and returns the commits on or near the screen it does not know yet.
func (m Model) fillStats() []string {
	var missing []string

	for _, rev := range m.log.Window(statsAhead) {
		stats, known := m.stats[rev.CommitID]
		if !known {
			if rev.CommitID != "" && !m.statsAsked[rev.CommitID] {
				missing = append(missing, rev.CommitID)
			}

			continue
		}

		rev.Added, rev.Removed, rev.Files = stats.Added, stats.Removed, stats.Files
	}

	return missing
}

// loadStats reads the stats of the rows on screen that lack them. The log arrives without them,
// because computing them for the whole window was ten times the cost of the graph itself.
func (m Model) loadStats() tea.Cmd {
	return tea.Batch(m.readStats(), m.countUnread())
}

// readStats asks for the stats fillStats found missing.
func (m Model) readStats() tea.Cmd {
	missing := m.fillStats()
	if len(missing) == 0 {
		return nil
	}

	for _, id := range missing {
		m.statsAsked[id] = true
	}

	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		stats, err := client.Stats(ctx, missing)

		return statsMsg{stats: stats, err: err, asked: missing}
	}
}

// applyStats files the answer and asks for whatever scrolled into view while it was out.
func (m Model) applyStats(msg statsMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		// A row without a size still reads, so the failure costs a column rather than the footer, and
		// the next scroll asks again.
		for _, id := range msg.asked {
			delete(m.statsAsked, id)
		}

		return m, nil
	}

	for id, stats := range msg.stats {
		m.stats[id] = stats
	}

	return m, m.loadStats()
}

// unreadCountedMsg carries the hunk lists of the commits a count named, by change id.
type unreadCountedMsg struct {
	err     error
	changes map[string]counted
	asked   []string
}

// counted is one change's hunk list, read at commit.
type counted struct {
	commit string
	refs   []seen.Ref
}

// countUnread reads the hunk lists of the mutable rows on or near the screen that no read has counted
// at their commit, in one jj call off the Update loop, so their unread counts are right from launch
// rather than only for the revisions the dock has opened. Immutable rows draw no count, and an empty
// one has nothing to read.
func (m Model) countUnread() tea.Cmd {
	if m.marksOff() {
		return nil
	}

	var (
		commits []string
		change  = map[string]string{}
	)

	for _, rev := range m.log.Window(statsAhead) {
		if rev.Immutable || rev.Empty || rev.CommitID == "" || m.countAsked[rev.CommitID] ||
			m.marks.live[rev.ChangeID].commit == rev.CommitID {
			continue
		}

		m.countAsked[rev.CommitID] = true
		commits = append(commits, rev.CommitID)
		change[rev.CommitID] = rev.ChangeID
	}

	if len(commits) == 0 {
		return nil
	}

	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		diffs, err := client.GitDiffs(ctx, commits)
		if err != nil {
			return unreadCountedMsg{err: err, asked: commits}
		}

		changes := make(map[string]counted, len(diffs))
		for commit, text := range diffs {
			changes[change[commit]] = counted{commit: commit, refs: seen.Parse([]byte(text))}
		}

		return unreadCountedMsg{changes: changes, asked: commits}
	}
}

// applyUnreadCount files each change's hunk list, which draws its row's count, unless a read the
// dock or the outline made since has filed a newer one. A failed count costs the counts, not the
// footer, and the next scroll asks again.
func (m Model) applyUnreadCount(msg unreadCountedMsg) Model {
	if msg.err != nil {
		for _, id := range msg.asked {
			delete(m.countAsked, id)
		}

		return m
	}

	for changeID, c := range msg.changes {
		if live, ok := m.marks.live[changeID]; ok && live.commit != "" && live.commit != c.commit {
			continue
		}

		m.noteRead(changeID, c.commit, c.refs)
	}

	return m
}
