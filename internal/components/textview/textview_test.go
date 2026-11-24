package textview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/textview"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// The footer keeps one line of a failure. This is where the rest of it goes, so every line jj wrote
// has to be reachable, including ones below the frame.
func TestView_ShowsEveryLineOfTheOutput(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{60, 12}, {40, 8}} {
		m := textview.New(ui.New(theme.Macchiato()))
		m.Open("jj git fetch failed", "jj git fetch\n\nFetching from Git remotes: origin\n"+
			"Error: Could not find repository at '/nonexistent/remote.git'\nline five\nline six\nline seven")

		var seen strings.Builder
		for range 10 {
			seen.WriteString(tuitest.StripANSI(m.View(size.width, size.height)))
			m.Update(tuitest.Key("j"))
		}

		for _, want := range []string{"Error: Could not find", "line seven"} {
			if !strings.Contains(seen.String(), want) {
				t.Errorf("%dx%d never showed %q", size.width, size.height, want)
			}
		}
	}
}

func TestUpdate_EscCloses(t *testing.T) {
	t.Parallel()

	m := textview.New(ui.New(theme.Macchiato()))
	m.Open("t", "x")

	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil || m.IsOpen() {
		t.Fatal("esc left the overlay open")
	}

	if _, ok := cmd().(textview.CancelledMsg); !ok {
		t.Error("esc did not report the dismissal")
	}
}
