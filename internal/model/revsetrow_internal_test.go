package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kyleking/jojo/internal/tuitest"
)

// The revset the log opened on is already in the footer, so the row above the log only appears
// while the box is being edited or once the log shows another revset, and the log gets the row back
// otherwise. Every frame is exactly the terminal's height either way.
func TestRevsetRow_ShowsOnlyWhileEditedOrChanged(t *testing.T) {
	t.Parallel()

	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 16}, {60, 12}} {
		m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: sz.w, Height: sz.h}))

		first := func(m Model) string {
			t.Helper()

			lines := strings.Split(ansi.Strip(m.render()), "\n")
			if len(lines) != sz.h {
				t.Fatalf("%dx%d: the frame is %d rows", sz.w, sz.h, len(lines))
			}

			return strings.TrimSpace(lines[0])
		}

		if got := first(m); strings.HasPrefix(got, "r:") {
			t.Errorf("%dx%d: the opening revset took the first row: %q", sz.w, sz.h, got)
		}

		m = asModel(t, update(m, tuitest.Key("L")))
		if got := first(m); !strings.HasPrefix(got, "r:") {
			t.Errorf("%dx%d: editing the revset did not show its row: %q", sz.w, sz.h, got)
		}

		m = asModel(t, update(m, tuitest.Key(keyEsc)))
		if got := first(m); strings.HasPrefix(got, "r:") {
			t.Errorf("%dx%d: leaving the box unchanged kept its row: %q", sz.w, sz.h, got)
		}

		m = asModel(t, update(m, logLoadedMsg{revset: "all()", rows: twoRows()}))
		if got := first(m); !strings.HasPrefix(got, "r:") {
			t.Errorf("%dx%d: a log under another revset hid the row naming it: %q", sz.w, sz.h, got)
		}
	}
}
