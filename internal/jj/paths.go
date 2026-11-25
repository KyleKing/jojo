package jj

import (
	"context"
	"regexp"
	"strings"
)

// recordSep ends one revision in a multi-revision template, since a description can hold newlines.
const recordSep = "\x1e"

// plainPath is a path jj reads as itself. Anything else, a space or a parenthesis, would parse as
// fileset syntax, so it goes through file:"..." instead.
var plainPath = regexp.MustCompile(`^[A-Za-z0-9._/+-]+$`)

// filePatterns names each path exactly, keeping the plain ones as typed so the command shown still
// reads like one a person would write.
func filePatterns(paths []string) []string {
	out := make([]string, len(paths))

	for i, path := range paths {
		if plainPath.MatchString(path) {
			out[i] = path

			continue
		}

		out[i] = `file:"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path) + `"`
	}

	return out
}

// CommitPaths describes @ with message holding only paths and starts a new change on top holding
// the rest, as one operation (`jj commit <paths> -m`). The working copy's files do not change, only
// which revision records them.
func (c *Client) CommitPaths(ctx context.Context, message string, paths []string) (string, error) {
	return c.mutate(ctx, CommitPathsArgs(message, paths)...)
}

// CommitPathsArgs is the invocation CommitPaths runs.
func CommitPathsArgs(message string, paths []string) []string {
	return append([]string{"commit", "-m", message}, filePatterns(paths)...)
}

// SplitPaths splits paths out of revision into a commit of their own described by message, which
// jj places before what is left, keeping the revision's description (`jj split -r <rev> <paths>
// -m`). No editor opens: the paths are the selection.
func (c *Client) SplitPaths(ctx context.Context, revision, message string, paths []string) (string, error) {
	return c.mutate(ctx, SplitPathsArgs(revision, message, paths)...)
}

// SplitPathsArgs is the invocation SplitPaths runs.
func SplitPathsArgs(revision, message string, paths []string) []string {
	return append([]string{"split", "-r", revision, "-m", message}, filePatterns(paths)...)
}
