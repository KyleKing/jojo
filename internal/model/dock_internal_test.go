package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kyleking/jojo/internal/components/diffpane"
	"github.com/kyleking/jojo/internal/tuitest"
)

const dockFirstDiff = `diff --git a/first.txt b/first.txt
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

const dockSecondDiff = `diff --git a/third.txt b/third.txt
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

// dockedModel opens the dock at width by height with the first revision's diff already read, which is
// what the read toggleDock starts would deliver.
func dockedModel(t *testing.T, width, height int) Model {
	t.Helper()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: width, Height: height}))

	next, cmd := m.Update(tuitest.Key("p"))
	if cmd == nil {
		t.Fatal("docking the diff started no read")
	}

	m = asModel(t, next)

	return asModel(t, update(m, dockLoadedMsg{revision: "aaaa", text: dockFirstDiff, seq: m.dockSeq}))
}

func dockTitle(m Model) string { return m.dock.Title() }

// moveLog presses key in the log and returns what the cursor move asks of the root, which is the
// SelectedMsg the log reports routed back through Update.
func moveLog(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(tuitest.Key(key))
	if cmd == nil {
		t.Fatalf("%s did not move the log cursor", key)
	}

	next, cmd = asModel(t, next).Update(cmd())

	return asModel(t, next), cmd
}

// Moving the log cursor waits for it to rest, then reads that revision, keeping second.txt in view
// because the new revision touches it too.
func TestDock_FollowsTheLogCursorKeepingTheFile(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	m.dock.Motion(diffpane.ObjectFile, 1)

	if got := m.dock.CurrentPath(); got != "second.txt" {
		t.Fatalf("the dock is in %q after ]f, want second.txt", got)
	}

	m, cmd := moveLog(t, m, "j")
	if cmd == nil {
		t.Fatal("moving the cursor did not ask the dock to follow")
	}

	tick, ok := cmd().(dockTickMsg)
	if !ok || tick.revision != "bbbb" {
		t.Fatalf("moving the cursor produced %#v, want a debounced read of bbbb", tick)
	}

	next, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("the rested cursor read nothing")
	}

	m = asModel(t, next)
	m = asModel(t, update(m, dockLoadedMsg{revision: "bbbb", text: dockSecondDiff, seq: m.dockSeq}))

	if got := dockTitle(m); got != "diff bbbb" {
		t.Errorf("the dock shows %q, want the revision under the cursor", got)
	}

	if got := m.dock.CurrentPath(); got != "second.txt" {
		t.Errorf("the dock is in %q after following, want second.txt kept", got)
	}
}

func TestDock_LandsOnTheFirstFileWhenTheRevisionLacksTheCurrentOne(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	m.dockSeq++
	m = asModel(t, update(m, dockLoadedMsg{revision: "bbbb", text: dockSecondDiff, seq: m.dockSeq}))

	if got := m.dock.CurrentPath(); got != "third.txt" {
		t.Errorf("the dock is in %q, want the new revision's first file", got)
	}
}

// Holding j sends a burst of moves, and only the last should cost a jj call.
func TestDock_DebouncesRapidCursorMoves(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)

	m, first := moveLog(t, m, "j")
	m, _ = moveLog(t, m, "k")

	stale, ok := first().(dockTickMsg)
	if !ok {
		t.Fatalf("the first move produced %T, want a tick", first())
	}

	if _, cmd := m.Update(stale); cmd != nil {
		t.Error("a tick a later move overtook still read a diff")
	}
}

// A read finishing after the cursor moved on would draw the old revision over the new one.
func TestDock_DropsAReadTheCursorHasMovedPast(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	stale := m.dockSeq
	m.dockSeq++

	m = asModel(t, update(m, dockLoadedMsg{revision: "bbbb", text: dockSecondDiff, seq: stale}))

	if got := dockTitle(m); got != "diff aaaa" {
		t.Errorf("a stale read replaced the dock with %q", got)
	}
}

// Tab reaches the dock, where j scrolls the diff rather than the log, and esc hands the keys back.
func TestDock_TabFocusesTheDiffAndEscReturnsToTheLog(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)

	m = asModel(t, update(m, tuitest.Key("tab")))
	if m.focus != focusDock {
		t.Fatalf("tab left focus at %d, want the dock", m.focus)
	}

	before := m.log.Selected().ChangeID
	m = asModel(t, update(m, tuitest.Key("j")))

	if m.log.Selected().ChangeID != before {
		t.Error("j in the focused dock moved the log")
	}

	if m.dock.CurrentPath() == "" {
		t.Error("j in the focused dock did not move inside the diff")
	}

	m = asModel(t, update(m, tuitest.Key("esc")))
	if m.focus != focusLog {
		t.Errorf("esc left focus at %d, want the log", m.focus)
	}
}

func TestDock_ClosingGivesTheLogItsRoomBack(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	m = asModel(t, update(m, tuitest.Key("p")))

	if m.docked || m.dock.IsOpen() {
		t.Fatal("the second alt+p left the dock open")
	}

	for _, line := range strings.Split(m.logPane(), "\n") {
		if strings.Contains(tuitest.StripANSI(line), "│ ") {
			t.Fatalf("the closed dock still draws beside the log: %q", tuitest.StripANSI(line))
		}
	}
}

// A wide terminal docks the diff beside the log and a narrow one under it, and either way the frame
// keeps exactly the terminal's size.
func TestDock_FitsTheTerminalAtEverySize(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{120, 16}, {80, 16}, {100, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			frame := dockedModel(t, size.width, size.height).render()
			lines := strings.Split(frame, "\n")

			if len(lines) != size.height {
				t.Errorf("the frame is %d lines, want %d", len(lines), size.height)
			}

			// The revset box on the first row is measured by its own tests; these are the rows the log
			// and the dock share.
			for i, line := range lines[1:] {
				if w := ansi.StringWidth(line); w > size.width {
					t.Errorf("line %d is %d cells wide, want at most %d", i+1, w, size.width)
				}
			}

			if !strings.Contains(tuitest.StripANSI(frame), "▎new first") {
				t.Errorf("the frame does not show the docked diff:\n%s", tuitest.StripANSI(frame))
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

const dockGoDiff = `diff --git a/main.go b/main.go
index 0000000..1111111 100644
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
-func main() { println("old") }
+func main() { println("new") }
`

// A revision shows plain while the update that drew it asks for its colors, which only wait for the
// lexers, so the update loop never lexes; a return to it draws in color at once.
func TestDock_ColorsArriveWithTheReadOrRightAfterIt(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	m.dockSeq++

	next, lex := m.Update(dockLoadedMsg{revision: "bbbb", text: dockGoDiff, seq: m.dockSeq})
	if lex == nil {
		t.Fatal("a revision drawn plain did not ask for its colors")
	}

	m = asModel(t, update(asModel(t, next), lex()))
	colored := strings.Join(m.dock.Lines(), "\n")

	m.dockSeq++
	m = asModel(t, update(m, dockLoadedMsg{revision: "aaaa", text: dockFirstDiff, seq: m.dockSeq}))
	m.dockSeq++

	next, cmd := m.Update(dockLoadedMsg{revision: "bbbb", text: dockGoDiff, seq: m.dockSeq})
	m = asModel(t, next)

	if cmd != nil {
		t.Error("a return to a revision already drawn in color asked for its colors again")
	}

	if got := strings.Join(m.dock.Lines(), "\n"); got != colored {
		t.Error("a return to a revision drew other rows than its colored ones")
	}

	m.dockSeq++
	plainText := strings.ReplaceAll(dockGoDiff, "new", "newer")

	next, cmd = m.Update(dockLoadedMsg{revision: "cccc", text: plainText, seq: m.dockSeq})
	if cmd == nil {
		t.Fatal("a revision drawn plain did not ask for its colors")
	}

	m = asModel(t, next)
	plain := strings.Join(m.dock.Lines(), "\n")
	m = asModel(t, update(m, cmd()))

	if got := strings.Join(m.dock.Lines(), "\n"); got == plain || ansi.Strip(got) != ansi.Strip(plain) {
		t.Error("the colors that landed did not replace the plain rows line for line")
	}
}
