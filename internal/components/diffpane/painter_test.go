package diffpane_test

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/ui"
)

const goDiff = `diff --git a/main.go b/main.go
index 0000000..1111111 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@ func main()
 package main
-func main() { println("old") }
+func main() { println("new") }
`

const otherDiff = `diff --git a/other.go b/other.go
index 0000000..1111111 100644
--- a/other.go
+++ b/other.go
@@ -1 +1 @@
-var x = 1
+var x = 2
`

func docked() diffpane.Model {
	return diffpane.NewDocked(ui.New(theme.Macchiato()))
}

// colorize runs the command Colorize returns and hands its message back to the pane, which is the
// round trip the program makes.
func colorize(t *testing.T, m *diffpane.Model) {
	t.Helper()

	cmd := m.Colorize()
	if cmd == nil {
		t.Fatal("Colorize() = nil for a diff drawn plain")
	}

	m.Colored(coloredMsg(t, cmd))
}

func coloredMsg(t *testing.T, cmd tea.Cmd) diffpane.ColoredMsg {
	t.Helper()

	got := cmd()

	msg, ok := got.(diffpane.ColoredMsg)
	if !ok {
		t.Fatalf("the command answered %T, want a ColoredMsg", got)
	}

	return msg
}

func stripped(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ansi.Strip(line)
	}

	return out
}

func TestShow_DrawsPlainThenSwapsInColorsOnTheSameRows(t *testing.T) {
	t.Parallel()

	m := docked()
	m.Show("diff a", goDiff, 80, "")
	plain := slices.Clone(m.Lines())
	m.StepHunk(1)
	cursorBefore := m.CurrentPath()

	colorize(t, &m)

	if slices.Equal(plain, m.Lines()) {
		t.Error("colored rows match the plain ones; the swap drew nothing new")
	}

	if !slices.Equal(stripped(plain), stripped(m.Lines())) {
		t.Errorf("colors changed the text of the rows:\nplain   %q\ncolored %q", stripped(plain), stripped(m.Lines()))
	}

	if m.CurrentPath() != cursorBefore {
		t.Errorf("cursor moved files across the swap: %q, was %q", m.CurrentPath(), cursorBefore)
	}

	if m.Colorize() != nil {
		t.Error("Colorize() asked again for a diff already in color")
	}
}

func TestColored_IgnoresADiffNoLongerOnShow(t *testing.T) {
	t.Parallel()

	m := docked()
	m.Show("diff a", goDiff, 80, "")
	stale := m.Colorize()

	m.Show("diff b", otherDiff, 80, "")
	plainB := slices.Clone(m.Lines())

	m.Colored(coloredMsg(t, stale))

	if !slices.Equal(plainB, m.Lines()) {
		t.Error("colors for the diff the cursor left were drawn over the one on show")
	}

	m.Show("diff a", goDiff, 80, "")

	if m.Colorize() != nil {
		t.Error("returning to a diff whose colors landed late drew it plain again")
	}
}

func TestShow_ReturningToADiffDrawsItInColorAtOnce(t *testing.T) {
	t.Parallel()

	m := docked()
	m.Show("diff a", goDiff, 80, "")
	colorize(t, &m)
	colored := slices.Clone(m.Lines())

	m.Show("diff b", otherDiff, 80, "")
	m.Show("diff a", goDiff, 80, "")

	if m.Colorize() != nil {
		t.Error("Colorize() asked for colors a return visit already had")
	}

	if !slices.Equal(colored, m.Lines()) {
		t.Error("a return visit drew different rows than the colored ones kept")
	}
}

func TestSetWidth_RedrawsPlainAndColorsAgain(t *testing.T) {
	t.Parallel()

	m := docked()
	m.Show("diff a", goDiff, 80, "")
	colorize(t, &m)

	m.SetWidth(60)
	colorize(t, &m)

	for _, line := range m.Lines() {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("row is %d cells after narrowing to 60: %q", w, ansi.Strip(line))
		}
	}
}

func TestColored_OnlyReachesThePaneThatAsked(t *testing.T) {
	t.Parallel()

	dock := docked()
	dock.Show("diff a", goDiff, 80, "")
	msg := coloredMsg(t, dock.Colorize())

	full := diffpane.New(ui.New(theme.Macchiato()))
	full.Open("diff a", goDiff, 80)
	full.Colored(msg)

	if full.Colorize() == nil {
		t.Error("the full-screen pane took colors the dock asked for")
	}
}
