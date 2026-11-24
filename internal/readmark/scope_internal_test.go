package readmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const full = "qwounpzzkmrtsvxylnopqrstuvwxyzkk"

func writeScope(t *testing.T, dir, name, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name+scopeExt), []byte(body), filePerm); err != nil {
		t.Fatal(err)
	}
}

func hunk(hash string) string { return "[[hunk]]\nhash = '" + hash + "'\npath = 'a.txt'\n" }

func TestMigrate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		files map[string]string
		name  string
		moved bool
	}{
		{name: "one prefix moves", files: map[string]string{"qwounpzz": hunk("h1")}, moved: true},
		{name: "a nine-character prefix moves", files: map[string]string{"qwounpzzk": hunk("h1")}, moved: true},
		{name: "nothing to move", files: map[string]string{"other123": hunk("h1")}},
		{
			name:  "two prefixes are ambiguous",
			files: map[string]string{"qwounpzz": hunk("h1"), "qwounpzzk": hunk("h2")},
		},
		{name: "the full file wins", files: map[string]string{"qwounpzz": hunk("h1"), full: hunk("h2")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for name, body := range tc.files {
				writeScope(t, dir, name, body)
			}

			moved, err := Migrate(dir, full)
			if err != nil || moved != tc.moved {
				t.Fatalf("Migrate = %v, %v; want %v", moved, err, tc.moved)
			}

			if !tc.moved {
				return
			}

			got, err := os.ReadFile(filepath.Join(dir, full+scopeExt)) //nolint:gosec // the test's own file
			if err != nil || !strings.Contains(string(got), "h1") {
				t.Errorf("the full file holds %q (%v)", got, err)
			}

			if _, err := os.Stat(filepath.Join(dir, full+scopeExt+".lock")); !os.IsNotExist(err) {
				t.Errorf("the lock was left behind: %v", err)
			}
		})
	}
}

// A full file another process wrote between Migrate's look and its lock is merged into, not lost.
func TestMove_MergesIntoAFullFileWrittenMeanwhile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeScope(t, dir, "qwounpzz", hunk("h1")+hunk("h2"))
	writeScope(t, dir, full, hunk("h2")+hunk("h3"))

	moved, err := move(filepath.Join(dir, "qwounpzz"+scopeExt), filepath.Join(dir, full+scopeExt))
	if err != nil || !moved {
		t.Fatalf("move = %v, %v", moved, err)
	}

	got, err := os.ReadFile(filepath.Join(dir, full+scopeExt)) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"h1", "h2", "h3"} {
		if strings.Count(string(got), "'"+h+"'")+strings.Count(string(got), `"`+h+`"`) != 1 {
			t.Errorf("the merged file holds %s other than once:\n%s", h, got)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "qwounpzz"+scopeExt)); !os.IsNotExist(err) {
		t.Errorf("the short file is still there: %v", err)
	}
}
