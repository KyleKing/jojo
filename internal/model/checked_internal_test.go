package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

func threeRows() []jj.LogRow {
	return []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa", Description: "one"}},
		{Revision: &jj.Revision{ChangeID: "bbbb", Description: "two"}},
		{Revision: &jj.Revision{ChangeID: "cccc", Description: "three"}},
	}
}

// checkFirstTwo checks aaaa and bbbb and leaves the cursor on cccc, the shape that sent an abandon at
// the checked rows while the cursor sat somewhere else.
func checkFirstTwo(t *testing.T) Model {
	t.Helper()

	m := asModel(t, update(testModel(t, threeRows()), tea.WindowSizeMsg{Width: 100, Height: 20}))
	for _, key := range []string{"space", "j", "space", "j"} {
		m = asModel(t, update(m, tuitest.Key(key)))
	}

	if got := m.log.Checked(); len(got) != 2 {
		t.Fatalf("checked %v, want two", got)
	}

	return m
}

// Every verb that takes several revisions follows one rule: the checked set when there is one. The
// picker names the whole set in the command it builds, and never offers a checked revision as its
// own destination.
func TestChecked_RebaseAndSquashTakeTheWholeSet(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"r", "R", "s"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(checkFirstTwo(t), tuitest.Key(key)))
			if !m.pick.active {
				t.Fatal("no picker opened")
			}

			command := m.pick.command()
			if !strings.Contains(command, "aaaa") || !strings.Contains(command, "bbbb") {
				t.Errorf("the command does not name the checked set: %s", command)
			}

			if m.pick.dest != "cccc" {
				t.Errorf("the destination is %q, want cccc past the checked rows", m.pick.dest)
			}

			m = asModel(t, update(m, tuitest.Key("k")))
			if m.pick.dest != "cccc" {
				t.Errorf("k moved the destination onto checked revision %q", m.pick.dest)
			}
		})
	}
}

// A check that outlives its action redirects the next one. Esc clears them, and so does a write that
// went through; a failed one keeps them, so the batch can be retried.
func TestChecked_ClearedByEscAndBySuccessfulWrite(t *testing.T) {
	t.Parallel()

	cleared := asModel(t, update(checkFirstTwo(t), tuitest.Key("esc")))
	if got := cleared.log.Checked(); len(got) != 0 {
		t.Errorf("esc left %v checked", got)
	}

	wrote := asModel(t, update(checkFirstTwo(t), messageMsg{text: "jj abandon aaaa bbbb", wrote: true}))
	if got := wrote.log.Checked(); len(got) != 0 {
		t.Errorf("a successful write left %v checked", got)
	}

	failedWrite := asModel(t, update(checkFirstTwo(t), messageMsg{text: "jj abandon: no", output: "jj abandon"}))
	if got := failedWrite.log.Checked(); len(got) != 2 {
		t.Errorf("a failed write cleared the checks: %v", got)
	}
}

// Checked rows scroll out of sight, so the footer keeps the count while any stand.
func TestChecked_FooterCountsTheBatch(t *testing.T) {
	t.Parallel()

	for _, width := range []int{120, 80, 50} {
		m := asModel(t, update(checkFirstTwo(t), tea.WindowSizeMsg{Width: width, Height: 20}))

		if footer := tuitest.StripANSI(m.footer()); !strings.Contains(footer, "2 checked") {
			t.Errorf("width %d footer lacks the count: %q", width, footer)
		}
	}
}
