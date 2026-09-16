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
