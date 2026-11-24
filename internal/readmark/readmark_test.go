package readmark_test

import (
	"testing"

	"github.com/kyleking/jojo/internal/readmark"
)

const before = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
 one
-two
+TWO
@@ -10,2 +10,2 @@
 ten
-eleven
+ELEVEN
`

// The same hunks further down the file: line numbers moved, text did not.
const shifted = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -5,2 +5,2 @@
 one
-two
+TWO
@@ -14,2 +14,2 @@
 ten
-eleven
+eleven!
`

// A hunk keeps its id when only its line numbers move and takes a new one when its text changes,
// which is what lets a read mark survive a rebase and fall away after an edit.
func TestHunks_IDsFollowTextNotPosition(t *testing.T) {
	t.Parallel()

	a, b := readmark.Hunks(before), readmark.Hunks(shifted)
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("hunks: %d and %d, want 2 each", len(a), len(b))
	}

	if a[0].ID != b[0].ID {
		t.Error("a hunk that only moved down took a new id")
	}

	if a[1].ID == b[1].ID {
		t.Error("a hunk whose text changed kept its id")
	}

	if a[0].Path != "a.go" || a[0].Hunk != 1 || a[1].Hunk != 2 {
		t.Errorf("hunks = %+v", a)
	}
}

func TestStrip_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want  string
		strip readmark.Strip
	}{
		{"····", readmark.Strip{}},
		{"▱▱▱▱", readmark.Strip{Total: 5}},
		{"▰▱▱▱", readmark.Strip{Read: 1, Total: 50}},
		{"▰▰▰▱", readmark.Strip{Read: 49, Total: 50}},
		{"▰▰▰▰", readmark.Strip{Read: 3, Total: 3}},
	}

	for _, tc := range cases {
		if got := tc.strip.String(); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc.strip, got, tc.want)
		}
	}
}
