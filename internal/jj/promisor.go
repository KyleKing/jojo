package jj

import (
	"os"
	"path/filepath"
	"strings"
)

// PromisorProblem reports why jj's backend will fail to read the repository's objects, or ""
// when it will read them: a partial-clone filter keeps blobs on the remote until asked, and a
// promisor pack holds objects jj's gix store skips — either way jj reports the failure only as
// "object <id> of type file not found" at checkout.
func PromisorProblem(root string) string {
	gitdir := gitDir(root)
	if gitdir == "" {
		return ""
	}

	objdir := gitdir
	if common := commonDir(gitdir); common != "" {
		objdir = common
	}

	config, err := os.ReadFile(filepath.Join(objdir, "config")) //nolint:gosec // the store's own config
	if err == nil &&
		strings.Contains(string(config), "partialclone") {
		return "a partial-clone filter"
	}

	packs, err := filepath.Glob(filepath.Join(objdir, "objects", "pack", "*.promisor"))
	if err == nil && len(packs) > 0 {
		return "promisor packs"
	}

	return ""
}

// gitDir resolves the repository's git dir: .git is a directory in a plain or colocated checkout
// and a "gitdir:" file in a linked worktree.
func gitDir(root string) string {
	dotgit := filepath.Join(root, ".git")

	if info, err := os.Stat(dotgit); err == nil && info.IsDir() {
		return dotgit
	}

	return readPointer(dotgit, "gitdir:")
}

// commonDir resolves where a linked worktree's shared store lives, since its objects and config
// sit there rather than in the worktree's own gitdir.
func commonDir(gitdir string) string {
	return readPointer(filepath.Join(gitdir, "commondir"), "")
}

// readPointer reads a one-line git pointer file ("gitdir: ../path" or a bare "../path") and
// resolves a relative target against the file's own directory.
func readPointer(path, prefix string) string {
	data, err := os.ReadFile(path) //nolint:gosec // the path is the repository's own metadata
	if err != nil {
		return ""
	}

	target := strings.TrimSpace(strings.TrimPrefix(string(data), prefix))
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}

	return target
}
