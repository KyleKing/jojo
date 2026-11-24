package jj

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RepoStore is the repository a workspace at root belongs to: <root>/.jj/repo in the main
// workspace, where it is a directory, and the directory that file names in a secondary workspace,
// where `.jj/repo` is a file holding a path relative to `.jj`. Every workspace of a repository
// answers the same path, which is what state shared across them (read marks) is keyed by.
func RepoStore(root string) (string, error) {
	dotJJ := filepath.Join(root, ".jj")
	repo := filepath.Join(dotJJ, "repo")

	info, err := os.Stat(repo)
	if err != nil {
		return "", fmt.Errorf("finding the repository of %s: %w", root, err)
	}

	if info.IsDir() {
		return absClean(repo)
	}

	data, err := os.ReadFile(repo) //nolint:gosec // G304: the workspace's own pointer file.
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", repo, err)
	}

	target := strings.TrimSpace(string(data))
	if target == "" {
		return "", fmt.Errorf("%s: %w", repo, errEmptyPointer)
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(dotJJ, target)
	}

	return absClean(target)
}

// errEmptyPointer marks a secondary workspace whose `.jj/repo` names nothing.
var errEmptyPointer = errors.New("names no repository")

func absClean(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", path, err)
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}

	return filepath.Clean(abs), nil
}
