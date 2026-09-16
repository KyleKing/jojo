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

		if !strings.Contains(got, "?] help") {
			t.Errorf("width %d dropped the help hint: %q", tt.width, got)
		}

		if strings.Contains(got, "…") != tt.wantEllipsis {
			t.Errorf("width %d ellipsis: want %v, got %q", tt.width, tt.wantEllipsis, got)
		}

		if strings.Contains(got, "[n]ew") != tt.wantNew {
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
