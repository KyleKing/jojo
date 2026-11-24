package jj_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// Every workspace of one repository answers the same store, the main one's .jj/repo directory, so
// state keyed by it is shared across them.
func TestRepoStore_SecondWorkspaceAnswersTheMainStore(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping")
	}

	base := t.TempDir()
	main := filepath.Join(base, "main")
	run(t, base, "git", "init", "main")
	run(t, main, "workspace", "add", filepath.Join(base, "second"))

	want, err := jj.RepoStore(main)
	if err != nil {
		t.Fatalf("RepoStore(main): %v", err)
	}

	if filepath.Base(want) != "repo" || filepath.Base(filepath.Dir(want)) != ".jj" {
		t.Errorf("the main workspace's store is %s, want its .jj/repo", want)
	}

	got, err := jj.RepoStore(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("RepoStore(second): %v", err)
	}

	if got != want {
		t.Errorf("the second workspace's store is %s, want the main one's %s", got, want)
	}

	if _, err := jj.RepoStore(t.TempDir()); err == nil {
		t.Error("a directory with no .jj answered a store")
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(t.Context(), "jj", args...)
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}
}
