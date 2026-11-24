package jj_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyleking/jojo/internal/hermetic"
	"github.com/kyleking/jojo/internal/jj"
)

const (
	fixtureFileMode = 0o600
	fixtureDirMode  = 0o750
)

// testRepo builds a temporary jj repository, skipping the test when no jj binary is installed.
func testRepo(tb testing.TB) (*jj.Client, string) {
	tb.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		tb.Skip("jj is not installed; skipping")
	}

	dir := tb.TempDir()

	// hermetic.Main names the committer and keeps jj's per-repository config in the run's home.
	runJJ(tb, dir, "git", "init", ".")

	return jj.NewClient(dir), dir
}

func runJJ(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", args...)
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("jj %v failed: %v\n%s", args, err, out)
	}
}

// descriptionOf reads @'s description via stdout alone, since jj can print one-time warnings (such as
// a config file migration notice) to stderr that would otherwise pollute a before/after comparison.
func descriptionOf(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "jj", "log", "--no-graph", "-r", "@", "-T", "description")
	cmd.Dir = dir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("jj log: %v", err)
	}

	return stdout.String()
}

func writeFile(tb testing.TB, dir, name, content string) {
	tb.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), fixtureFileMode); err != nil {
		tb.Fatalf("write %s: %v", name, err)
	}
}

// seedRepo builds a repository shaped like the requirements doc calls for: a linear chain, a
// bookmark, an empty working-copy commit, and a merge with a real two-sided conflict.
func seedRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")

	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "commit", "-m", "second")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "main")

	writeFile(t, dir, "c.txt", "c\n")
	runJJ(t, dir, "describe", "-m", "third")

	runJJ(t, dir, "new", "-m", "empty change")

	runJJ(t, dir, "new", "main", "-m", "conflict-a")
	writeFile(t, dir, "b.txt", "conflict-a-content\n")
	runJJ(t, dir, "bookmark", "create", "-r", "@", "side-a")

	runJJ(t, dir, "new", "main", "-m", "conflict-b")
	writeFile(t, dir, "b.txt", "conflict-b-content\n")
	runJJ(t, dir, "bookmark", "create", "-r", "@", "side-b")

	runJJ(t, dir, "new", "side-a", "side-b", "-m", "merge-conflict")

	return client, dir
}

// allowLargeFiles raises the repo's snapshot size limit, since jj otherwise refuses to snapshot any
// file over 1MiB and a diff test needs one well past that.
func allowLargeFiles(t *testing.T, dir string) {
	t.Helper()

	hermetic.RepoConfig(t, dir, "[snapshot]\nmax-new-file-size = \"10MiB\"")
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()

	//nolint:gosec // G304: fixture helper reads a file this test just wrote into t.TempDir().
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return string(content)
}
