package diffpane

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const twoFileDiff = `diff --git a/first.txt b/first.txt
index 0000000..1111111 100644
--- a/first.txt
+++ b/first.txt
@@ -1 +1 @@
-old first
+new first
diff --git a/second.txt b/second.txt
index 0000000..2222222 100644
--- a/second.txt
+++ b/second.txt
@@ -1 +1 @@
-old second
+new second
`

func key(s string) tea.KeyPressMsg {
	code, _ := utf8.DecodeRuneInString(s)

	return tea.KeyPressMsg(tea.Key{Text: s, Code: code})
}

func TestFileKeysStepBetweenHeaders(t *testing.T) {
	t.Parallel()

	m := New(ui.New(theme.Macchiato()))
	m.Open("diff", twoFileDiff, 80)

	bounds := fileBoundaries(twoFileDiff, m.list.Items())
	if len(bounds) != 2 {
		t.Fatalf("fileBoundaries = %v, want one per file", bounds)
	}

	m.Update(key("]"))

	if m.list.Cursor() != bounds[1] {
		t.Errorf("cursor = %d after ], want the second header at %d", m.list.Cursor(), bounds[1])
	}

	m.Update(key("]"))

	if m.list.Cursor() != bounds[1] {
		t.Error("] at the last file should clamp, not wrap")
	}

	m.Update(key("["))

	if m.list.Cursor() != bounds[0] {
		t.Errorf("cursor = %d after [, want the first header at %d", m.list.Cursor(), bounds[0])
	}
}

func TestFileKeysNoopWithoutHeaders(t *testing.T) {
	t.Parallel()

	m := New(ui.New(theme.Macchiato()))
	m.Open("diff", "not a diff", 80)

	if !m.IsOpen() {
		return // render produced no lines; nothing to step over
	}

	m.Update(key("]"))

	if m.list.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0 when the diff has no file headers", m.list.Cursor())
	}
}

const otherFileDiff = `diff --git a/third.txt b/third.txt
index 0000000..3333333 100644
--- a/third.txt
+++ b/third.txt
@@ -1 +1 @@
-old third
+new third
diff --git a/second.txt b/second.txt
index 0000000..2222222 100644
--- a/second.txt
+++ b/second.txt
@@ -1 +1 @@
-old second
+new second
`

// The docked pane follows the log, and a reader halfway down second.txt should still be reading
// second.txt when the next revision changes it too, wherever it now sits in the diff.
func TestShow_KeepsTheFileWhenTheNewRevisionTouchesIt(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff a", twoFileDiff, 60, "")
	m.Update(key("]"))

	if got := m.CurrentPath(); got != "second.txt" {
		t.Fatalf("CurrentPath() = %q after ], want second.txt", got)
	}

	m.Show("diff b", otherFileDiff, 60, m.CurrentPath())

	if got := m.CurrentPath(); got != "second.txt" {
		t.Errorf("CurrentPath() = %q after following, want second.txt kept", got)
	}

	if m.list.Cursor() != m.files[1] {
		t.Errorf("cursor = %d, want second.txt's header at %d", m.list.Cursor(), m.files[1])
	}
}

func TestShow_FallsBackToTheFirstFileWhenTheRevisionLacksIt(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff a", twoFileDiff, 60, "")

	m.Show("diff b", otherFileDiff, 60, "first.txt")

	if got := m.CurrentPath(); got != "third.txt" {
		t.Errorf("CurrentPath() = %q, want the first file, third.txt", got)
	}
}

// The docked pane shares rows with the log, so a line too many or too few shifts everything beside it.
func TestDockView_FillsExactlyItsRect(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{50, 8}, {30, 20}, {12, 3}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			m := NewDocked(ui.New(theme.Macchiato()))
			m.Show("diff a", twoFileDiff, DockWidth(size.width), "")

			frame := m.DockView(size.width, size.height, true)
			lines := strings.Split(frame, "\n")

			if len(lines) != size.height {
				t.Fatalf("DockView drew %d lines, want %d", len(lines), size.height)
			}

			for i, line := range lines {
				if w := ansi.StringWidth(line); w > size.width {
					t.Errorf("line %d is %d cells wide, want at most %d", i, w, size.width)
				}
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

func TestDockView_SaysSoWhenNothingChanged(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff empty", "", 40, "")

	if view := tuitest.StripANSI(m.DockView(42, 4, false)); !strings.Contains(view, "nothing changed") {
		t.Errorf("an empty docked diff does not say so:\n%s", view)
	}
}
