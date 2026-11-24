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

	press(&m, "]", "f")

	if m.list.Cursor() != bounds[1] {
		t.Errorf("cursor = %d after ]f, want the second header at %d", m.list.Cursor(), bounds[1])
	}

	press(&m, "]", "f")

	if m.list.Cursor() != bounds[1] {
		t.Error("]f at the last file should clamp, not wrap")
	}

	press(&m, "[", "f")

	if m.list.Cursor() != bounds[0] {
		t.Errorf("cursor = %d after [, want the first header at %d", m.list.Cursor(), bounds[0])
	}
}

// press plays keys the way jojo's keymap dispatches them to the pane: ] or [ and an object is a
// Motion, n and N a Repeat, and anything else goes to Update.
func press(m *Model, keys ...string) []tea.Msg {
	var out []tea.Msg

	collect := func(cmd tea.Cmd) {
		if cmd != nil {
			out = append(out, cmd())
		}
	}

	for i := 0; i < len(keys); i++ {
		switch k := keys[i]; {
		case (k == "]" || k == "[") && i+1 < len(keys):
			dir, object := 1, keys[i+1]
			if k == "[" {
				dir = -1
			}

			if object == "[" {
				object = ObjectChapter
			}

			i++
			collect(m.Motion(object, dir))
		case k == "n":
			collect(m.Repeat(1))
		case k == "N":
			collect(m.Repeat(-1))
		default:
			collect(m.Update(key(k)))
		}
	}

	return out
}

const twoHunkDiff = `diff --git a/a.txt b/a.txt
index 0000000..1111111 100644
--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
@@ -20,3 +20,3 @@
 twenty
-twenty-one
+TWENTY-ONE
 twenty-two
diff --git a/b.txt b/b.txt
index 0000000..2222222 100644
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-old b
+new b
`

// ]h and [h step hunk labels across files, n repeats the last motion and N reverses it, and before
// any motion n steps hunks, so reading a diff top to bottom is one key held down.
func TestHunkMotionsAndRepeat(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff", twoHunkDiff, 80, "")

	if len(m.hunks) != 3 {
		t.Fatalf("hunks = %v, want three labels", m.hunks)
	}

	press(&m, "n")

	if m.list.Cursor() != m.hunks[0] {
		t.Errorf("n before any motion: cursor %d, want the first hunk %d", m.list.Cursor(), m.hunks[0])
	}

	press(&m, "]", "h")

	if m.list.Cursor() != m.hunks[1] {
		t.Errorf("]h: cursor %d, want %d", m.list.Cursor(), m.hunks[1])
	}

	press(&m, "n")

	if m.list.Cursor() != m.hunks[2] || m.CurrentPath() != "b.txt" {
		t.Errorf("n after ]h: cursor %d in %s, want the hunk in b.txt", m.list.Cursor(), m.CurrentPath())
	}

	press(&m, "N")

	if m.list.Cursor() != m.hunks[1] {
		t.Errorf("N: cursor %d, want back at %d", m.list.Cursor(), m.hunks[1])
	}
}

// The motions over what the parent holds go up as messages, and n repeats one of those too.
func TestMotionsTheParentHoldsGoUp(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff", twoHunkDiff, 80, "")

	cases := []struct {
		keys []string
		want MotionMsg
	}{
		{[]string{"]", "r"}, MotionMsg{Object: ObjectRound, Dir: 1}},
		{[]string{"[", "r"}, MotionMsg{Object: ObjectRound, Dir: -1}},
		{[]string{"N"}, MotionMsg{Object: ObjectRound, Dir: 1}},
		{[]string{"]", "]"}, MotionMsg{Object: ObjectChapter, Dir: 1}},
		{[]string{"[", "["}, MotionMsg{Object: ObjectChapter, Dir: -1}},
	}

	for _, tc := range cases {
		got := press(&m, tc.keys...)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%v sent %v, want %v", tc.keys, got, tc.want)
		}
	}
}

func TestFileKeysNoopWithoutHeaders(t *testing.T) {
	t.Parallel()

	m := New(ui.New(theme.Macchiato()))
	m.Open("diff", "not a diff", 80)

	if !m.IsOpen() {
		return // render produced no lines; nothing to step over
	}

	press(&m, "]", "f")

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
	press(&m, "]", "f")

	if got := m.CurrentPath(); got != "second.txt" {
		t.Fatalf("CurrentPath() = %q after ]f, want second.txt", got)
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

const operationShow = `8a5b04bdf956 root@vm default@ now, lasted 2 milliseconds
describe commit 0b68fc5a49c0578c6f0b76b3581fc772f51b23ee
args: jj describe -m wip2

Changed commits:
+ puwuvkol d63b3a3c wip2
- puwuvkol/1 0b68fc5a (hidden) wip
diff --git a/JJ-COMMIT-DESCRIPTION b/JJ-COMMIT-DESCRIPTION
--- JJ-COMMIT-DESCRIPTION
+++ JJ-COMMIT-DESCRIPTION
@@ -1,1 +1,1 @@
-wip
+wip2

Changed working copy default@:
+ puwuvkol d63b3a3c wip2
- puwuvkol/1 0b68fc5a (hidden) wip
`

// An operation's diff went to jj's pager, and a pager that quits on a short page flashed it and
// returned. The pane draws it instead: jj's header and changed commits as prose, the patch as a diff.
func TestOpen_OperationDrawsItsHeaderAroundThePatch(t *testing.T) {
	t.Parallel()

	for _, width := range []int{100, 60} {
		m := New(ui.New(theme.Macchiato()))
		m.Open("operation 8a5b04bd", operationShow, width)

		plain := make([]string, 0, len(m.list.Items()))
		for _, line := range m.list.Items() {
			plain = append(plain, ansi.Strip(line))
		}

		text := strings.Join(plain, "\n")
		wants := []string{
			"args: jj describe -m wip2", "Changed commits:", "+ puwuvkol d63b3a3c wip2",
			"Changed working copy default@:",
		}
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("width %d lacks %q:\n%s", width, want, text)
			}
		}

		if len(m.files) != 1 || m.paths[0] != "JJ-COMMIT-DESCRIPTION" {
			t.Errorf("width %d found files %v at %v, want the one patch", width, m.paths, m.files)
		}

		if header := plain[m.files[0]]; !strings.Contains(header, "JJ-COMMIT-DESCRIPTION") {
			t.Errorf("width %d file boundary lands on %q", width, header)
		}
	}
}

// A revision's diff is one segment, so it renders exactly as it did before operations shared the path.
func TestSegments_APlainDiffIsOneSegment(t *testing.T) {
	t.Parallel()

	if got := segments(twoFileDiff); len(got) != 1 || !got[0].diff {
		t.Errorf("segments = %+v, want one diff", got)
	}

	if got := segments(""); len(got) != 0 {
		t.Errorf("segments of nothing = %+v", got)
	}
}
