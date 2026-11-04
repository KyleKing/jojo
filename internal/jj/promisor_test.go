package jj_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// gitdir builds a fake git store under dir: a config line and a promisor marker when each is
// named, since PromisorProblem reads the filesystem and nothing else.
func gitdir(t *testing.T, dir, config string, promisor bool) string {
	t.Helper()

	git := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(git, "objects", "pack"), fixtureDirMode); err != nil {
		t.Fatal(err)
	}

	if config != "" {
		writeFile(t, dir, ".git/config", config)
	}
	if promisor {
		writeFile(t, dir, ".git/objects/pack/pack-abc.promisor", "")
	}

	return git
}

func TestPromisorProblem(t *testing.T) {
	t.Parallel()

	t.Run("a plain store answers nothing", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		gitdir(t, dir, "[core]\nrepositoryformatversion = 1\n", false)

		if got := jj.PromisorProblem(dir); got != "" {
			t.Errorf("PromisorProblem = %q, want empty", got)
		}
	})

	t.Run("a filtered clone is named", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		gitdir(t, dir, "[remote \"origin\"]\n\tpartialclonefilter = blob:none\n", false)

		if got := jj.PromisorProblem(dir); !strings.Contains(got, "partial-clone") {
			t.Errorf("PromisorProblem = %q, want the filter named", got)
		}
	})

	t.Run("promisor packs are named with the filter already gone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		gitdir(t, dir, "[core]\nrepositoryformatversion = 1\n", true)

		if got := jj.PromisorProblem(dir); !strings.Contains(got, "promisor") {
			t.Errorf("PromisorProblem = %q, want the packs named", got)
		}
	})

	t.Run("a gitfile resolves to the real store", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		git := gitdir(t, filepath.Join(dir, "linked"), "", true)
		writeFile(t, dir, ".git", "gitdir: "+git+"\n")

		if got := jj.PromisorProblem(dir); !strings.Contains(got, "promisor") {
			t.Errorf("PromisorProblem = %q, want the packs named", got)
		}
	})

	t.Run("no git store answers nothing", func(t *testing.T) {
		t.Parallel()

		if got := jj.PromisorProblem(t.TempDir()); got != "" {
			t.Errorf("PromisorProblem = %q, want empty", got)
		}
	})
}
