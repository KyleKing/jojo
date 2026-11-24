package model

import (
	tea "charm.land/bubbletea/v2"

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
