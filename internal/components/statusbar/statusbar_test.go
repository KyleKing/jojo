package statusbar_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/statusbar"
	"github.com/kyleking/jojo/internal/ui"
)

func testModel() statusbar.Model {
	return statusbar.New(ui.New(theme.Macchiato()))
}

func testHints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "n", What: "new"},
		{Key: "e", What: "edit"},
		{Key: "d", What: "describe"},
		{Key: "?", What: "help"},
	}
}

// The help hint is the only route to the rest of the keymap, so it has to
// survive every width the footer is asked to render at, and everything else
// drops from the right behind an ellipsis before it does.
func TestView_DropsHintsFromTheRightKeepingHelp(t *testing.T) {
	t.Parallel()

	ctx := statusbar.Context{Revset: "trunk()..@", Hints: testHints()}

	tests := []struct {
		width        int
		wantEllipsis bool
		wantNew      bool
	}{
		{width: 20, wantEllipsis: false, wantNew: false},
		{width: 25, wantEllipsis: true, wantNew: false},
		{width: 40, wantEllipsis: true, wantNew: true},
		{width: 60, wantEllipsis: false, wantNew: true},
	}

	for _, tt := range tests {
		got := ansi.Strip(testModel().View(tt.width, ctx))

		if !strings.Contains(got, "? help") {
			t.Errorf("width %d dropped the help hint: %q", tt.width, got)
		}

		if strings.Contains(got, "…") != tt.wantEllipsis {
			t.Errorf("width %d ellipsis: want %v, got %q", tt.width, tt.wantEllipsis, got)
		}

		if strings.Contains(got, "n new") != tt.wantNew {
			t.Errorf("width %d new hint: want %v, got %q", tt.width, tt.wantNew, got)
		}

		if got := lipgloss.Width(testModel().View(tt.width, ctx)); got != tt.width {
			t.Errorf("width %d rendered %d cells", tt.width, got)
		}
	}
}

// jj has no single current branch, so every bookmark near @ has to survive
// in the footer, and a wide glyph in one of their names must not shift the
// row's measured width.
func TestView_MeasuresWideBookmarkNamesInCells(t *testing.T) {
	t.Parallel()

	ctx := statusbar.Context{
		Bookmarks: []statusbar.Bookmark{{Name: "リリース", AtWorkingCopy: true}, {Name: "main"}},
		Hints:     []keyhint.Hint{{Key: "?", What: "help"}},
	}

	for _, width := range []int{24, 40, 80} {
		got := testModel().View(width, ctx)
		if w := lipgloss.Width(got); w != width {
			t.Errorf("width %d rendered %d cells: %q", width, w, ansi.Strip(got))
		}
	}
}

// A transient message carries its own undo hint rather than gating the edit
// behind a confirmation, so it has to actually reach the rendered line.
func TestView_ShowsTheTransientMessageInPlaceOfBookmarksAndRevset(t *testing.T) {
	t.Parallel()

	ctx := statusbar.Context{
		Bookmarks: []statusbar.Bookmark{{Name: "main", AtWorkingCopy: true}},
		Revset:    "trunk()..@",
		Message:   "abandoned mkyzvz, press u to undo",
		Hints:     []keyhint.Hint{{Key: "?", What: "help"}},
	}

	got := ansi.Strip(testModel().View(60, ctx))

	if !strings.Contains(got, "press u to undo") {
		t.Errorf("undo hint missing from %q", got)
	}

	if strings.Contains(got, "main") || strings.Contains(got, "trunk()") {
		t.Errorf("bookmarks or revset leaked through the transient message: %q", got)
	}
}

// A footer wider than the terminal wraps onto a second row and costs a line of the log, so the
// prefix goes before the last hint does, and the hint itself is trimmed before anything overflows.
func TestViewNeverOverflowsANarrowTerminal(t *testing.T) {
	t.Parallel()

	ctx := statusbar.Context{
		Bookmarks: []statusbar.Bookmark{{Name: "main"}, {Name: "wip"}},
		Revset:    "builtin_log()",
		Undo:      "describe commit abc12345",
		Hints: []keyhint.Hint{
			{Key: "j", What: "down"},
			{Key: "k", What: "up"},
			{Key: "?", What: "help"},
		},
	}

	bar := statusbar.New(ui.New(theme.Macchiato()))

	for _, width := range []int{120, 80, 60, 40, 30, 20, 10, 5, 1} {
		if got := lipgloss.Width(bar.View(width, ctx)); got != width {
			t.Errorf("View(%d) rendered %d cells, want exactly %d", width, got, width)
		}
	}
}

// A message too wide for the hints beside it used to drop the whole prefix, message included, so an
// invalid bookmark name failed with nothing on screen but key hints.
func TestView_TruncatesAWideMessageRatherThanDroppingIt(t *testing.T) {
	t.Parallel()

	message := "jj bookmark create: jj command failed: invalid value 'my topic' for '<NAMES>...': " +
		"Failed to parse bookmark name: Syntax error (expected <EOI>)"

	for _, width := range []int{30, 60, 100} {
		got := ansi.Strip(testModel().View(width, statusbar.Context{Message: message, Hints: testHints()}))

		if !strings.Contains(got, "jj bookmark") {
			t.Errorf("width %d dropped the message: %q", width, got)
		}

		if !strings.Contains(got, "…") {
			t.Errorf("width %d did not mark the cut: %q", width, got)
		}

		if !strings.Contains(got, "? help") {
			t.Errorf("width %d dropped the help hint: %q", width, got)
		}

		if w := lipgloss.Width(got); w != width {
			t.Errorf("width %d rendered %d cells", width, w)
		}
	}
}

// The footer named every bookmark in the log, which crowded the hints out at 120 and 200 columns. It
// names the one at @ and counts the rest, which the log draws on their rows anyway.
func TestView_NamesOneBookmarkAndCountsTheRest(t *testing.T) {
	t.Parallel()

	rest := []string{"stack-a-2", "stack-a-1", "side", "main*", "main@origin", "stack-b"}
	marks := make([]statusbar.Bookmark, 0, 2+len(rest))
	marks = append(marks, statusbar.Bookmark{Name: "stack-a-3"},
		statusbar.Bookmark{Name: "stack-a-4", AtWorkingCopy: true})

	for _, name := range rest {
		marks = append(marks, statusbar.Bookmark{Name: name})
	}

	for _, width := range []int{80, 120} {
		got := ansi.Strip(testModel().View(width, statusbar.Context{Bookmarks: marks, Hints: testHints()}))

		if !strings.HasPrefix(got, "stack-a-4 +7  ") || strings.Contains(got, "stack-a-3") {
			t.Errorf("at %d the footer reads %q, want stack-a-4 +7 alone", width, got)
		}

		if !strings.Contains(got, "d describe") {
			t.Errorf("at %d the bookmarks crowded out the hints: %q", width, got)
		}
	}

	one := statusbar.Context{Bookmarks: marks[:1], Hints: testHints()}
	got := ansi.Strip(testModel().View(80, one))
	if !strings.HasPrefix(got, "stack-a-3 ") || strings.Contains(got, "+") {
		t.Errorf("one bookmark reads %q, want it alone", got)
	}
}

// A write's message says u undoes it, and the undo segment said it again beside it, cutting both:
// "jj abandon luzunqzu · press u to undo  u undoes abandon com…". While a message is up it is the
// only one.
func TestView_MessageStandsInForTheUndoSegment(t *testing.T) {
	t.Parallel()

	ctx := statusbar.Context{
		Message: "jj abandon luzunqzu, press u to undo", Undo: "abandon commit 0123", Hints: testHints(),
	}

	got := ansi.Strip(testModel().View(120, ctx))
	if strings.Contains(got, "u undoes") || !strings.Contains(got, "press u to undo") {
		t.Errorf("the footer reads %q, want the message alone", got)
	}

	ctx.Message = ""
	if got := ansi.Strip(testModel().View(120, ctx)); !strings.Contains(got, "u undoes abandon commit 0123") {
		t.Errorf("with no message the footer reads %q, want the undo segment", got)
	}
}
