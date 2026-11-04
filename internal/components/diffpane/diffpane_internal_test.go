package diffpane

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

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
