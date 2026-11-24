package jj_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// renamedRepo is old.txt edited, renamed to new.txt, edited again, beside an unrelated file, with
// sub/a.txt to read from a subdirectory.
func renamedRepo(t *testing.T) *jj.Client {
	t.Helper()

	client, dir := testRepo(t)
	write := func(name, body string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), fixtureDirMode); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), fixtureFileMode); err != nil {
			t.Fatal(err)
		}
	}

	lines := "a\nb\nc\nd\ne\nf\n"
	write("old.txt", lines)
	runJJ(t, dir, "commit", "-m", "add old")
	write("other.txt", "x\n")
	runJJ(t, dir, "commit", "-m", "unrelated")
	write("old.txt", lines+"g\n")
	runJJ(t, dir, "commit", "-m", "edit old")

	if err := os.Rename(filepath.Join(dir, "old.txt"), filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}

	runJJ(t, dir, "commit", "-m", "rename")
	write("new.txt", lines+"g\nh\n")
	write("sub/a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "edit new")

	return client
}

func descriptions(t *testing.T, client *jj.Client, revset string) []string {
	t.Helper()

	rows, err := client.Log(t.Context(), revset, 0)
	if err != nil {
		t.Fatal(err)
	}

	var out []string

	for _, r := range rows {
		if r.Revision != nil {
			out = append(out, r.Revision.Description)
		}
	}

	return out
}

// A file's history reaches back through its rename: the changes to it under the old name are in it,
// and the unrelated change is not, whether it is asked from the new name, from the rename itself, or
// by the path the file list prints from a subdirectory.
func TestFollowFile_FollowsARename(t *testing.T) {
	t.Parallel()

	client := renamedRepo(t)
	want := []string{"edit new", "rename", "edit old", "add old"}

	for _, tc := range []struct {
		name   string
		origin jj.FileOrigin
		names  []string
	}{
		{"a diff heading", jj.FileOrigin{Revision: "@-", Path: "new.txt"}, []string{"new.txt", "old.txt"}},
		{"the rename", jj.FileOrigin{Revision: "@--", Path: "new.txt"}, []string{"new.txt", "old.txt"}},
		{
			"the file list",
			jj.FileOrigin{Revision: "@--", Path: jj.RenamedTo("{old.txt => new.txt}"), CwdRelative: true},
			[]string{"new.txt", "old.txt"},
		},
		{"before the rename", jj.FileOrigin{Revision: "@---", Path: "old.txt"}, []string{"old.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, err := client.FollowFile(t.Context(), tc.origin, "::@")
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(h.Names, tc.names) {
				t.Errorf("names %v, want %v", h.Names, tc.names)
			}

			got := descriptions(t, client, h.Revset)
			if tc.name == "before the rename" {
				want := []string{"rename", "edit old", "add old"}
				if !slices.Equal(got, want) {
					t.Errorf("the old name's history is %v, want %v", got, want)
				}

				return
			}

			if !slices.Equal(got, want) {
				t.Errorf("history %v, want %v (revset %s)", got, want, h.Revset)
			}
		})
	}
}

// A path the revision does not change is refused by name rather than answered with an empty log.
func TestFollowFile_RefusesAFileTheRevisionDoesNotChange(t *testing.T) {
	t.Parallel()

	client := renamedRepo(t)

	_, err := client.FollowFile(t.Context(), jj.FileOrigin{Revision: "@-", Path: "other.txt"}, "::@")
	if err == nil || !strings.Contains(err.Error(), "does not change that file") {
		t.Errorf("got %v", err)
	}
}

func TestQuoteAndRenamedTo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, quoted, renamed string }{
		{"a.txt", `"a.txt"`, "a.txt"},
		{`we "ird"\x`, `"we \"ird\"\\x"`, `we "ird"\x`},
		{"src/{a.go => b.go}", `"src/{a.go => b.go}"`, "src/b.go"},
		{"{a => b}/c.go", `"{a => b}/c.go"`, "b/c.go"},
		{"src/{ => sub}/c.go", `"src/{ => sub}/c.go"`, "src/sub/c.go"},
	} {
		if got := jj.Quote(tc.in); got != tc.quoted {
			t.Errorf("Quote(%q) = %s, want %s", tc.in, got, tc.quoted)
		}

		if got := jj.RenamedTo(tc.in); got != tc.renamed {
			t.Errorf("RenamedTo(%q) = %s, want %s", tc.in, got, tc.renamed)
		}
	}
}
