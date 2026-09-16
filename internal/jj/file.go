package jj

import (
	"context"
	"regexp"
	"strings"
)

// FileChange is one path a revision touches, as `jj diff --summary` reports it.
type FileChange struct {
	// Status is jj's own letter: A added, M modified, D deleted, R renamed, C copied. A file that
	// only appears in the conflict list carries no status, since the revision's own diff is empty.
	Status   string
	Path     string
	Conflict bool
}

// Files lists what the revision changes, with the conflicted paths folded in. A merge's own diff can
// be empty while it is still conflicted, so the conflict list is the only source for those paths.
func (c *Client) Files(ctx context.Context, revision string) ([]FileChange, error) {
	out, err := c.executeJJ(ctx, "diff", "-r", revision, "--summary", "--no-pager", "--color=never")
	if err != nil {
		return nil, err
	}

	files := parseFileSummary(out)

	conflicted, err := c.conflicts(ctx, revision)
	if err != nil {
		return nil, err
	}

	return markConflicts(files, conflicted), nil
}

// noConflicts is what jj says instead of printing an empty list, and it exits non-zero saying it.
const noConflicts = "No conflicts found"

func (c *Client) conflicts(ctx context.Context, revision string) ([]string, error) {
	out, err := c.executeJJ(ctx, "resolve", "--list", "-r", revision)
	if err != nil {
		if strings.Contains(err.Error(), noConflicts) {
			return nil, nil
		}

		return nil, err
	}

	return parseConflictList(out), nil
}

func parseFileSummary(out string) []FileChange {
	files := []FileChange{}

	for _, line := range strings.Split(out, "\n") {
		status, path, ok := strings.Cut(line, " ")
		if !ok || path == "" {
			continue
		}

		files = append(files, FileChange{Status: status, Path: path})
	}

	return files
}

// conflictSuffix is the column `jj resolve --list` pads after the path. Trimming it from the right
// is what keeps a path containing spaces intact.
var conflictSuffix = regexp.MustCompile(`\s+\d+-sided conflict.*$`)

func parseConflictList(out string) []string {
	var paths []string

	for _, line := range strings.Split(out, "\n") {
		if path := conflictSuffix.ReplaceAllString(line, ""); path != "" && path != line {
			paths = append(paths, path)
		}
	}

	return paths
}

func markConflicts(files []FileChange, conflicted []string) []FileChange {
	for _, path := range conflicted {
		if i := indexOfPath(files, path); i >= 0 {
			files[i].Conflict = true

			continue
		}

		files = append(files, FileChange{Path: path, Conflict: true})
	}

	return files
}

func indexOfPath(files []FileChange, path string) int {
	for i, f := range files {
		if f.Path == path {
			return i
		}
	}

	return -1
}

// RestoreFile puts one path back to its state in the revision's parents, which undoes that file's
// change without touching anything else the revision does.
func (c *Client) RestoreFile(ctx context.Context, revision, path string) (string, error) {
	return c.mutate(ctx, "restore", "-c", revision, path)
}
