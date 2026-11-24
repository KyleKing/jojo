package jj

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// A file's history is `jj log` over the changes that touched it. jj 0.47 has no `--follow`: `files()`
// matches a path, not a file, so the changes from before a rename are under another name. jj does
// detect a rename inside one change's diff, though, so FollowFile asks each change touching the
// names known so far whether it renamed one of them from something else, adds that, and asks again
// until no change does. The rounds are capped, since every round is a log over the whole scope.

// followRounds is how many renames back FollowFile follows a file.
const followRounds = 8

// ErrNoSuchFile is a file the revision's diff does not name, which has no history to show from there.
var ErrNoSuchFile = errors.New("the revision does not change that file")

// FileOrigin names a file where the reader pressed for its history: the revision whose diff shows it
// and its path, relative to the repository root (a diff's heading) or to the working directory (the
// file list, which prints jj's summary).
type FileOrigin struct {
	Revision string
	Path     string
	// CwdRelative says Path is relative to where jj runs rather than to the repository root.
	CwdRelative bool
}

// FileHistory is one file's names, the newest first and each relative to the repository root, and
// the revset of the changes in scope that touched it under any of them.
type FileHistory struct {
	Revset string
	Names  []string
}

// Fileset is a fileset matching exactly the files named, each relative to the repository root.
func Fileset(names ...string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, "root-file:"+Quote(n))
	}

	return strings.Join(parts, " | ")
}

// Quote is s as a jj string literal, which revsets, filesets, and templates all read the same way.
func Quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`, "\x00", `\0`)

	return `"` + r.Replace(s) + `"`
}

// RenamedTo is the path a summary line names a file by after a rename: jj prints `src/{a => b}.go`
// for one, which is no path a fileset matches. Any other path comes back as it is.
func RenamedTo(path string) string {
	open, closing := strings.Index(path, "{"), strings.LastIndex(path, "}")
	if open < 0 || closing < open {
		return path
	}

	_, after, ok := strings.Cut(path[open+1:closing], " => ")
	if !ok {
		return path
	}

	return strings.ReplaceAll(path[:open]+after+path[closing+1:], "//", "/")
}

// renameTemplate prints each rename in a change's diff of fileset as `source<TAB>target`, a line
// each; status prints every entry the same way with its status first, for the origin's own diff.
func renameTemplate(fileset string) string {
	return "self.diff(" + Quote(fileset) + `).files().filter(|e| e.status() == "renamed")` +
		`.map(|e| e.source().path() ++ "\t" ++ e.target().path() ++ "\n").join("")`
}

func statusTemplate(fileset string) string {
	return "self.diff(" + Quote(fileset) + `).files()` +
		`.map(|e| e.status() ++ "\t" ++ e.source().path() ++ "\t" ++ e.target().path() ++ "\n").join("")`
}

// FollowFile is the history of the file at origin within scope, a revset the caller chooses: every
// name the file had, back through each rename jj detects, and the revset of the changes in scope
// that touched any of them. The scope bounds the search as well as the result, so a rename outside
// it is not followed.
func (c *Client) FollowFile(ctx context.Context, origin FileOrigin, scope string) (FileHistory, error) {
	pattern := "root-file:" + Quote(origin.Path)
	if origin.CwdRelative {
		pattern = "file:" + Quote(origin.Path)
	}

	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", origin.Revision,
		"-T", statusTemplate(pattern))
	if err != nil {
		return FileHistory{}, fmt.Errorf("reading %s in %s: %w", origin.Path, origin.Revision, err)
	}

	var names []string

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 { //nolint:mnd // status, source, target
			continue
		}

		names = appendNew(names, fields[2])
		if fields[0] == "renamed" {
			names = appendNew(names, fields[1])
		}
	}

	if len(names) == 0 {
		return FileHistory{}, fmt.Errorf("%w: %s in %s", ErrNoSuchFile, origin.Path, origin.Revision)
	}

	return c.followRenames(ctx, names, scope)
}

// followRenames grows names by every rename in scope that ended at one of them, until none does.
func (c *Client) followRenames(ctx context.Context, names []string, scope string) (FileHistory, error) {
	for range followRounds {
		revset := historyRevset(names, scope)

		out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", revset,
			"-T", renameTemplate(Fileset(names...)))
		if err != nil {
			return FileHistory{}, fmt.Errorf("following %s back through its renames: %w", names[0], err)
		}

		grown := names

		for _, line := range strings.Split(out, "\n") {
			source, target, ok := strings.Cut(line, "\t")
			if ok && slices.Contains(names, target) {
				grown = appendNew(grown, source)
			}
		}

		if len(grown) == len(names) {
			return FileHistory{Revset: revset, Names: names}, nil
		}

		names = grown
	}

	return FileHistory{Revset: historyRevset(names, scope), Names: names}, nil
}

// historyRevset is the changes in scope touching any of names.
func historyRevset(names []string, scope string) string {
	return "files(" + Fileset(names...) + ") & (" + scope + ")"
}

func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}

	return append(list, s)
}
