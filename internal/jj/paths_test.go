package jj_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// pathsRepo is a described parent and a working copy changing three files on top of it, the shape
// the path verbs pick files out of.
func pathsRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "one.txt", "one\n")
	writeFile(t, dir, "two.txt", "two\n")
	writeFile(t, dir, "three.txt", "three\n")
	runJJ(t, dir, "describe", "-m", "work in progress")

	return client, dir
}

func filesIn(t *testing.T, client *jj.Client, rev string) []string {
	t.Helper()

	// Reads see the working copy as the last log read left it, so take the snapshot one would.
	if err := client.Snapshot(context.Background()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	files, err := client.Files(context.Background(), rev)
	if err != nil {
		t.Fatalf("Files(%s): %v", rev, err)
	}

	paths := pathsOf(files)
	slices.Sort(paths)

	return paths
}

// logOf renders template for rev from stdout alone, as descriptionOf does for @.
func logOf(t *testing.T, dir, rev, template string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", "log", "--no-graph", "-r", rev, "-T", template)
	cmd.Dir = dir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("jj log -r %s: %v", rev, err)
	}

	return stdout.String()
}

// assertOnDisk is the invariant every path verb keeps: the files in the working copy read the same
// afterwards, whichever revision now records them.
func assertOnDisk(t *testing.T, dir string, want map[string]string) {
	t.Helper()

	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: a fixture path the test wrote.
		if err != nil {
			t.Errorf("%s is gone from the working copy: %v", name, err)

			continue
		}

		if string(got) != content {
			t.Errorf("%s reads %q, want %q", name, got, content)
		}
	}
}

var pathFiles = map[string]string{
	"base.txt": "base\n", "one.txt": "one\n", "two.txt": "two\n", "three.txt": "three\n",
}

func TestCommitPaths_CommitsOnlyTheNamedFilesAndLeavesTheRestInTheWorkingCopy(t *testing.T) {
	t.Parallel()

	client, dir := pathsRepo(t)

	if _, err := client.CommitPaths(context.Background(), "the first two", []string{"one.txt", "two.txt"}); err != nil {
		t.Fatalf("CommitPaths: %v", err)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"one.txt", "two.txt"}) {
		t.Errorf("the commit holds %v, want exactly the named files", got)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"three.txt"}) {
		t.Errorf("@ holds %v, want the file left out", got)
	}

	if got := logOf(t, dir, "@-", "description"); got != "the first two\n" {
		t.Errorf("the commit is described %q", got)
	}

	assertOnDisk(t, dir, pathFiles)
}

func TestSplitPaths_SplitsTheNamedFilesOutBeforeTheRest(t *testing.T) {
	t.Parallel()

	client, dir := pathsRepo(t)

	if _, err := client.SplitPaths(context.Background(), "@", "just one", []string{"one.txt"}); err != nil {
		t.Fatalf("SplitPaths: %v", err)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"one.txt"}) {
		t.Errorf("the split-out commit holds %v, want only one.txt", got)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"three.txt", "two.txt"}) {
		t.Errorf("the rest holds %v", got)
	}

	if got := logOf(t, dir, "@-", "description"); got != "just one\n" {
		t.Errorf("the split-out commit is described %q", got)
	}

	if got := logOf(t, dir, "@", "description"); got != "work in progress\n" {
		t.Errorf("the rest lost its description: %q", got)
	}

	assertOnDisk(t, dir, pathFiles)
}

func TestSquash_MovesExactlyTheNamedFiles(t *testing.T) {
	t.Parallel()

	client, dir := pathsRepo(t)

	if _, err := client.Squash(context.Background(), "@", "@-", "one.txt", "three.txt"); err != nil {
		t.Fatalf("Squash: %v", err)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"base.txt", "one.txt", "three.txt"}) {
		t.Errorf("the destination holds %v", got)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"two.txt"}) {
		t.Errorf("@ holds %v, want the file left out", got)
	}

	assertOnDisk(t, dir, pathFiles)
}

// A path jj would read as fileset syntax has to reach it as the file it names, or committing it
// fails with a parse error about a file the list showed.
func TestCommitPaths_CommitsAPathWithFilesetSyntaxInIt(t *testing.T) {
	t.Parallel()

	client, dir := pathsRepo(t)
	writeFile(t, dir, "sp ace(1).txt", "odd\n")

	if _, err := client.CommitPaths(context.Background(), "odd", []string{"sp ace(1).txt"}); err != nil {
		t.Fatalf("CommitPaths: %v", err)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"sp ace(1).txt"}) {
		t.Errorf("the commit holds %v, want the odd path alone", got)
	}
}
