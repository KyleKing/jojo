package model

import (
	"fmt"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
)

func manyRows(n int, commitPrefix string) []jj.LogRow {
	rows := make([]jj.LogRow, n)
	for i := range rows {
		rows[i] = jj.LogRow{Revision: &jj.Revision{
			ChangeID: fmt.Sprintf("k%07d", i),
			CommitID: fmt.Sprintf("%s%06d", commitPrefix, i),
		}}
	}

	return rows
}

// Per-row stats made the startup log about ten times slower than the graph alone, and every write
// paid for them again across the whole window. The first paint asks only for the rows near the
// screen, and a reload reads only commits it has not seen.
func TestStats_ReadOnlyForRowsOnScreenAndNeverTwice(t *testing.T) {
	t.Parallel()

	for _, height := range []int{24, 40} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(testModel(t, nil), tea.WindowSizeMsg{Width: 100, Height: height}))
			m, cmd := m.applyLog(logLoadedMsg{revset: "@", rows: manyRows(500, "c")})

			if cmd == nil || len(m.statsAsked) == 0 {
				t.Fatal("the first paint asked for no stats")
			}

			if limit := height + statsAhead; len(m.statsAsked) > limit {
				t.Errorf("asked for %d rows' stats, want at most the %d near the screen", len(m.statsAsked), limit)
			}

			m, _ = m.applyStats(statsMsg{stats: answerAll(m.statsAsked)})
			if top := m.log.Selected(); top.Files != 1 || top.Added != 2 {
				t.Errorf("the row under the cursor was not filled in: %+v", top)
			}

			asked := len(m.statsAsked)

			// The same commits after a reload: the cache answers, nothing is read.
			m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: manyRows(500, "c")})
			if len(m.statsAsked) != asked || m.log.Selected().Files != 1 {
				t.Errorf("a reload of known commits read again (%d asked, was %d)", len(m.statsAsked), asked)
			}

			// A write rewrote them: only the new commit ids are read.
			m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: manyRows(500, "d")})
			if len(m.statsAsked) != 2*asked {
				t.Errorf("after a rewrite asked for %d, want the %d new commits", len(m.statsAsked)-asked, asked)
			}
		})
	}
}

// A failed read costs the size column, not the next attempt: the rows are asked for again.
func TestStats_AFailedReadIsAskedAgain(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, nil), tea.WindowSizeMsg{Width: 100, Height: 24}))
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: manyRows(50, "c")})

	asked := make([]string, 0, len(m.statsAsked))
	for id := range m.statsAsked {
		asked = append(asked, id)
	}

	m, _ = m.applyStats(statsMsg{err: jj.ErrCommandFailed, asked: asked})
	if len(m.statsAsked) != 0 {
		t.Fatalf("%d rows stay marked as asked after the read failed", len(m.statsAsked))
	}

	if m.loadStats() == nil {
		t.Error("the next chance did not ask again")
	}
}

// answerAll is a stats read that answered every commit asked for.
func answerAll(asked map[string]bool) map[string]jj.Stats {
	answer := map[string]jj.Stats{}
	for id := range asked {
		answer[id] = jj.Stats{Files: 1, Added: 2, Removed: 3}
	}

	return answer
}
