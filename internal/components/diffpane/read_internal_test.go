package diffpane

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/ui"
)

// threeHunks has two hunks in one file and one in another, which is what HunkAt numbers per file.
const threeHunks = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
@@ -20,2 +20,2 @@
 twenty
-twenty-one
+TWENTY-ONE
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-bee
+BEE
`

// HunkAt names the file and the hunk the cursor is in, JumpHunk lands on one by that name, and a
// read hunk's label, and the heading of a file read whole, draw behind the read glyph.
func TestRead_HunkAtJumpHunkAndTheReadGlyph(t *testing.T) {
	t.Parallel()

	m := NewDocked(ui.New(theme.Macchiato()))
	m.Show("diff x", threeHunks, 60, "")

	if path, k, ok := m.HunkAt(); !ok || path != "a.txt" || k != -1 {
		t.Fatalf("on the first heading HunkAt = %q %d %v, want a.txt's heading", path, k, ok)
	}

	if n := m.FileHunks("a.txt"); n != 2 {
		t.Fatalf("a.txt draws %d hunks, want 2", n)
	}

	for _, want := range []struct {
		path string
		k    int
	}{{"a.txt", 1}, {"b.txt", 0}, {"a.txt", 0}, {"b.txt", -1}} {
		if !m.JumpHunk(want.path, want.k) {
			t.Fatalf("JumpHunk(%s, %d) found nothing", want.path, want.k)
		}

		if path, k, _ := m.HunkAt(); path != want.path || k != want.k {
			t.Errorf("after JumpHunk(%s, %d) HunkAt = %s %d", want.path, want.k, path, k)
		}
	}

	if m.JumpHunk("a.txt", 2) || m.JumpHunk("c.txt", 0) {
		t.Error("JumpHunk landed on a hunk the diff does not have")
	}

	m.SetRead(func(path string, k int) bool { return path == "b.txt" || k == 1 })

	if read := readRows(m); len(read) != 3 || !strings.Contains(read[0], "L20") || !strings.Contains(read[1], "b.txt") {
		t.Errorf("rows drawn read: %q, want a.txt's second hunk, b.txt's heading, and its hunk", read)
	}
}

// readRows is every row the docked pane draws behind the read glyph.
func readRows(m Model) []string {
	var read []string

	for _, line := range strings.Split(m.DockView(62, 40, false), "\n") {
		if text := ansi.Strip(line); strings.Contains(text, ReadGlyph+" ") {
			read = append(read, strings.TrimSpace(strings.TrimPrefix(text, "│ ")))
		}
	}

	return read
}
