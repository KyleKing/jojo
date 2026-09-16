package help_test

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/ui"
)

func testModel(t *testing.T) help.Model {
	t.Helper()

	keys, err := help.NewKeymap(nil)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	return help.New(ui.New(theme.Macchiato()), keys)
}

// A legend too tall for its frame has to say where the reader is in it,
// because silently clipping the rest is the bug this component exists to
// avoid.
func TestView_ReportsScrollPositionWhenClipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model help.Model
	}{
		{"top", testModel(t)},
		{"scrolled", testModel(t).ScrollBy(3)},
		{"end", testModel(t).ScrollToEnd()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			golden.RequireEqual(t, []byte(ansi.Strip(tt.model.View(50, 10))))
		})
	}
}

// A legend that fits the frame needs no scroll position: there is nothing to
// scroll to.
func TestView_FitsWithoutAScrollFooterWhenTall(t *testing.T) {
	t.Parallel()

	got := ansi.Strip(testModel(t).View(60, 40))
	if got == "" {
		t.Fatal("View rendered nothing")
	}

	golden.RequireEqual(t, []byte(got))
}

// ScrollBy clamps at the top, and ScrollToEnd resolves against the frame's
// height rather than needing it passed in up front.
func TestScroll_ClampsAtBothEnds(t *testing.T) {
	t.Parallel()

	top := ansi.Strip(testModel(t).ScrollBy(-5).View(50, 10))
	if top != ansi.Strip(testModel(t).View(50, 10)) {
		t.Errorf("scrolling above the top changed the view:\n%s", top)
	}

	end := testModel(t).ScrollToEnd()
	if got, want := ansi.Strip(end.View(50, 10)), ansi.Strip(end.ScrollBy(100).View(50, 10)); got != want {
		t.Errorf("scrolling past the end changed the view:\n%s\nvs\n%s", got, want)
	}
}
