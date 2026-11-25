package model

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// checkedRepo is liveRepo with the working copy, described "work", changing three files.
func checkedRepo(t *testing.T) string {
	t.Helper()

	dir := liveRepo(t)

	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	jjRun(t, dir, "describe", "-m", "work")

	return dir
}

func filesOf(t *testing.T, dir, rev string) []string {
	t.Helper()

	out := jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r", rev,
		"-T", `diff.files().map(|f| f.path()).join("\n")`)

	return strings.Fields(out)
}

// Space only checks: the repository is byte-for-byte as it was until a verb runs, and s then squashes
// exactly the checked files into the change picked, in one operation.
func TestFilesLive_SpaceChecksAndSSquashesExactlyTheCheckedFiles(t *testing.T) {
	t.Parallel()

	dir := checkedRepo(t)
	m := liveModel(t, dir, "work")
	m = press(t, m, "l")

	if !m.files.IsOpen() {
		t.Fatalf("l did not open the file list: %q", m.message)
	}

	before, ops := repoPrint(t, dir), opCount(t, dir)

	m = press(t, m, "space", "j", "j", "space")

	if got := m.files.Checked(); !slices.Equal(got, []string{"a.txt", "c.txt"}) {
		t.Fatalf("checked %v, want a.txt and c.txt", got)
	}

	if repoPrint(t, dir) != before || opCount(t, dir) != ops {
		t.Fatal("checking files wrote to the repository")
	}

	m = press(t, m, "s")
	if !m.destPicker.IsOpen() {
		t.Fatal("s did not ask where the checked files go")
	}

	press(t, m, "m", "i", "d", "d", "l", "e", "enter")

	if got := opCount(t, dir); got != ops+1 {
		t.Errorf("the squash took %d operations, want one", got-ops)
	}

	middle := filesOf(t, dir, "description(exact:'middle\n')")
	if got := middle; !slices.Equal(got, []string{"a.txt", "c.txt", "middle.txt"}) {
		t.Errorf("middle holds %v, want the checked files added", got)
	}

	if got := filesOf(t, dir, "description(exact:'work\n')"); !slices.Equal(got, []string{"b.txt"}) {
		t.Errorf("work holds %v, want only the unchecked file", got)
	}
}

// c commits the checked files and leaves the rest in a fresh working copy.
func TestFilesLive_CCommitsTheCheckedFiles(t *testing.T) {
	t.Parallel()

	dir := checkedRepo(t)
	m := liveModel(t, dir, "work")
	m = press(t, m, "l", "j", "space", "c")

	if !m.prompt.IsOpen() {
		t.Fatal("c did not ask for the message")
	}

	m = press(t, m, "ctrl+u")
	for _, r := range "just b" {
		m = press(t, m, string(r))
	}

	press(t, m, "enter")

	if got := filesOf(t, dir, "@-"); !slices.Equal(got, []string{"b.txt"}) {
		t.Errorf("the commit holds %v, want b.txt", got)
	}

	if got := filesOf(t, dir, "@"); !slices.Equal(got, []string{"a.txt", "c.txt"}) {
		t.Errorf("@ holds %v, want the unchecked files", got)
	}
}

// S splits the checked files out of the revision into a commit of their own, before the rest.
func TestFilesLive_SSplitsTheCheckedFilesOut(t *testing.T) {
	t.Parallel()

	dir := checkedRepo(t)
	m := liveModel(t, dir, "work")
	m = press(t, m, "l", "space", "S")

	if !m.prompt.IsOpen() {
		t.Fatal("S did not ask for the message")
	}

	m = press(t, m, "ctrl+u")
	for _, r := range "just a" {
		m = press(t, m, string(r))
	}

	press(t, m, "enter")

	if got := filesOf(t, dir, "description(exact:'just a\n')"); !slices.Equal(got, []string{"a.txt"}) {
		t.Errorf("the split-out commit holds %v, want a.txt", got)
	}

	if got := filesOf(t, dir, "description(exact:'work\n')"); !slices.Equal(got, []string{"b.txt", "c.txt"}) {
		t.Errorf("work holds %v, want the rest", got)
	}
}
