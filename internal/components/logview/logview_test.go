package logview_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// fixture mirrors the shapes scripts/fixture-repo.sh builds: a merge with its
// connectors, a conflict, an immutable ancestor, a bookmark off the head, and
// an elided-revisions line carrying prose in the graph column.
func fixture() []jj.LogRow {
	return []jj.LogRow{
		{
			Revision: &jj.Revision{
				Graph:       "@  ",
				ChangeID:    "wkwyyxvq",
				Empty:       true,
				WorkingCopy: true,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{
			Revision: &jj.Revision{
				Graph:       "○  ",
				ChangeID:    "rnzxmskv",
				Description: "large refactor across many files",
				Files:       6,
				Added:       240,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{
			Revision: &jj.Revision{
				Graph:       "│ ○  ",
				ChangeID:    "koopqtrk",
				Description: "merge branch one and branch two",
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{Graph: &jj.GraphLine{Graph: "│ ├─╮"}},
		{
			Revision: &jj.Revision{
				Graph:       "│ │ ○  ",
				ChangeID:    "ryvkrytz",
				Description: "branch two: add branch-two.txt",
				Files:       1,
				Added:       1,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{Graph: &jj.GraphLine{Graph: "├───╯"}},
		{
			Revision: &jj.Revision{
				Graph:       "○  ",
				ChangeID:    "twnsvwmr",
				Bookmarks:   []string{"main"},
				Description: "add c",
				Files:       1,
				Added:       1,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{
			Revision: &jj.Revision{
				Graph:       "◆  ",
				ChangeID:    "nptwozlo",
				Bookmarks:   []string{"wip"},
				Description: "add b",
				Files:       1,
				Added:       1,
				Immutable:   true,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{
			Revision: &jj.Revision{
				Graph:       "│ ×  ",
				ChangeID:    "kyvykpum",
				Description: "edit shared file another way",
				Files:       1,
				Added:       8,
				Removed:     1,
				Conflict:    true,
				AuthorEmail: "fixture@example.com",
				Age:         "2 years ago",
			},
		},
		{Graph: &jj.GraphLine{Graph: "│ ~  (elided revisions)"}},
	}
}

func newModel(rows []jj.LogRow, width, height int) logview.Model {
	m := logview.New(styles())
	m.SetRows(rows)
	m.SetSize(width, height)

	return m
}

func styles() logview.Styles {
	return logview.StylesFrom(ui.New(theme.Macchiato()))
}

func TestCheckedRowIsMarked(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 4)
	m.Update(tuitest.Key("space"))

	frame := tuitest.StripANSI(m.View())
	if !strings.Contains(strings.SplitN(frame, "\n", 2)[0], logview.CheckMark) {
		t.Errorf("checked row is unmarked:\n%s", frame)
	}
}

func TestViewAtWidths(t *testing.T) {
	t.Parallel()

	for _, width := range []int{100, 72, 48} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Parallel()

			m := newModel(fixture(), width, len(fixture()))
			tuitest.AssertGolden(t, tuitest.StripANSI(m.View()))
		})
	}
}

// The cursor steps by revisions, so a connector between two rows is never
// selectable and never a dead keypress.
func TestCursorSkipsGraphOnlyRows(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)

	want := []string{"rnzxmskv", "koopqtrk", "ryvkrytz", "twnsvwmr"}
	for i, id := range want {
		m.Update(tuitest.Key("j"))

		got := m.Selected()
		if got == nil || got.ChangeID != id {
			t.Fatalf("after %d presses selected %v, want %s", i+1, got, id)
		}
	}
}

func TestJumpToWorkingCopyAndCheck(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)

	m.Update(tuitest.Key("G"))
	m.Update(tuitest.Key("space"))
	m.Update(tuitest.Key("@"))

	if got := m.Selected(); got == nil || !got.WorkingCopy {
		t.Fatalf("@ selected %v, want the working copy", got)
	}

	if got := m.Checked(); len(got) != 1 || got[0] != "kyvykpum" {
		t.Errorf("Checked() = %v, want [kyvykpum]", got)
	}
}

func advance(m *logview.Model, presses int) {
	for range presses {
		m.Update(tuitest.Key("j"))
	}
}

func TestSetSearchMatchesEachField(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pattern string
		want    string
		matches int
	}{
		"change id":   {pattern: "kyvykpum", want: "kyvykpum", matches: 1},
		"description": {pattern: "branch", want: "koopqtrk", matches: 2},
		"bookmark":    {pattern: "wip", want: "nptwozlo", matches: 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := newModel(fixture(), 100, 12)

			matches, index := m.SetSearch(tc.pattern)
			if matches != tc.matches || index != 1 {
				t.Fatalf("SetSearch(%q) = %d, %d, want %d, 1", tc.pattern, matches, index, tc.matches)
			}

			if got := m.Selected(); got == nil || got.ChangeID != tc.want {
				t.Errorf("SetSearch(%q) landed on %v, want %s", tc.pattern, got, tc.want)
			}
		})
	}
}

// A pattern that matches the prose jj writes on a graph-only line must never
// land the cursor on it, since that line carries no revision to select.
func TestSetSearchSkipsGraphOnlyRows(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)

	matches, index := m.SetSearch("elided")
	if matches != 0 || index != 0 {
		t.Fatalf("SetSearch(elided) = %d, %d, want 0, 0", matches, index)
	}
}

func TestSetSearchLandsAtOrAfterCursorAndWraps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want    string
		presses int
	}{
		"match after cursor":   {presses: 1, want: "koopqtrk"},
		"wraps to first match": {presses: 4, want: "koopqtrk"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := newModel(fixture(), 100, 12)
			advance(&m, tc.presses)

			matches, index := m.SetSearch("branch")
			if matches != 2 || index != 1 {
				t.Fatalf("SetSearch(branch) = %d, %d, want 2, 1", matches, index)
			}

			if got := m.Selected(); got == nil || got.ChangeID != tc.want {
				t.Errorf("SetSearch(branch) landed on %v, want %s", got, tc.want)
			}
		})
	}
}

func TestStepMatchWrapsBothWays(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)

	matches, index := m.SetSearch("branch")
	if matches != 2 || index != 1 {
		t.Fatalf("SetSearch(branch) = %d, %d, want 2, 1", matches, index)
	}

	matches, index = m.StepMatch(1)
	if matches != 2 || index != 2 {
		t.Fatalf("StepMatch(1) = %d, %d, want 2, 2", matches, index)
	}

	if got := m.Selected(); got == nil || got.ChangeID != "ryvkrytz" {
		t.Errorf("StepMatch(1) landed on %v, want ryvkrytz", got)
	}

	matches, index = m.StepMatch(1)
	if matches != 2 || index != 1 {
		t.Fatalf("StepMatch(1) wrapping forward = %d, %d, want 2, 1", matches, index)
	}

	matches, index = m.StepMatch(-1)
	if matches != 2 || index != 2 {
		t.Fatalf("StepMatch(-1) wrapping backward = %d, %d, want 2, 2", matches, index)
	}
}

func TestSetSearchEmptyPatternClearsSearch(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)
	m.SetSearch("branch")

	matches, index := m.SetSearch("")
	if matches != 0 || index != 0 {
		t.Fatalf("SetSearch(\"\") = %d, %d, want 0, 0", matches, index)
	}

	if matches, index := m.StepMatch(1); matches != 0 || index != 0 {
		t.Errorf("StepMatch after clearing search = %d, %d, want 0, 0", matches, index)
	}
}

// A pattern with no matches must not move the cursor: it has nowhere to go.
func TestSetSearchNoMatchLeavesCursor(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)
	before := m.Selected()

	matches, index := m.SetSearch("nonesuch")
	if matches != 0 || index != 0 {
		t.Fatalf("SetSearch(nonesuch) = %d, %d, want 0, 0", matches, index)
	}

	if got := m.Selected(); got == nil || got.ChangeID != before.ChangeID {
		t.Errorf("SetSearch(nonesuch) moved the cursor to %v, want %v", got, before)
	}
}

// SetRows holds the cursor on the surviving change id across a reload, and a
// recorded search pattern must survive it too, recomputed against the new rows.
func TestSearchSurvivesSetRows(t *testing.T) {
	t.Parallel()

	m := newModel(fixture(), 100, 12)

	matches, index := m.SetSearch("branch")
	if matches != 2 || index != 1 {
		t.Fatalf("SetSearch(branch) = %d, %d, want 2, 1", matches, index)
	}

	rows := fixture()
	rows = append(rows, jj.LogRow{Revision: &jj.Revision{
		Graph:       "○  ",
		ChangeID:    "zzzbranch",
		Description: "another branch commit",
	}})
	m.SetRows(rows)

	matches, index = m.StepMatch(1)
	if matches != 3 || index != 2 {
		t.Fatalf("StepMatch(1) after SetRows = %d, %d, want 3, 2", matches, index)
	}

	if got := m.Selected(); got == nil || got.ChangeID != "ryvkrytz" {
		t.Errorf("StepMatch(1) after SetRows landed on %v, want ryvkrytz", got)
	}
}
