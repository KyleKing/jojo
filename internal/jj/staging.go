package jj

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// The staging vocabulary maps git's index onto jj by treating the working copy's parent as the
// index: staging a file squashes it from @ into @-, unstaging squashes it back, and committing
// describes @- and inserts a fresh empty change between it and @. Every step moves content between
// two revisions or moves a parent pointer, so none of them can lose what the working copy holds.

// ErrNoIndex marks a staging verb refused because @- cannot stand in for git's index: it is
// immutable, it is shared with another child, or @ has more than one parent. Rewriting it in any of
// those cases would reach past the working copy, so the verb refuses rather than asks.
var ErrNoIndex = errors.New("@- cannot act as the index")

// ErrNothingStaged marks a commit with nothing in the index to commit.
var ErrNothingStaged = errors.New("nothing staged to commit")

// keepEmptied stops jj abandoning a squash's source once it is empty, which for the staging verbs
// would take the working-copy change or the index, and its description, with it.
const keepEmptied = "--keep-emptied"

// recordSep ends one revision in a multi-revision template, since a description can hold newlines.
const recordSep = "\x1e"

// indexTemplate renders what a staging verb checks about @-, with the description last so its
// newlines cannot shift the fields before it.
const indexTemplate = `change_id.shortest(8) ++ "` + fieldSep + `" ++ ` +
	`if(immutable,"I","") ++ if(empty,"E","") ++ "` + fieldSep + `" ++ ` +
	`description ++ "` + recordSep + `"`

const indexFieldCount = 3

// Index is the revision standing in for git's index: the working copy's only parent.
type Index struct {
	ChangeID    string
	Description string
	// Empty reports that nothing is staged.
	Empty bool
}

// Index reads @- and refuses with ErrNoIndex when rewriting it would touch more than @ and @-.
func (c *Client) Index(ctx context.Context) (Index, error) {
	out, err := c.executeJJ(ctx, "log", "--no-graph",
		"-r", "parents(@)", "-T", indexTemplate)
	if err != nil {
		return Index{}, err
	}

	index, immutable, err := parseIndex(out)
	if err != nil {
		return Index{}, err
	}

	if immutable {
		return Index{}, fmt.Errorf("%w: %s is immutable", ErrNoIndex, index.ChangeID)
	}

	siblings, err := c.executeJJ(ctx, "log", "--no-graph",
		"-r", "children(@-) ~ @", "-T", `change_id.shortest(8) ++ "\n"`)
	if err != nil {
		return Index{}, err
	}

	if others := strings.Fields(siblings); len(others) > 0 {
		return Index{}, fmt.Errorf("%w: %s has other children (%s)",
			ErrNoIndex, index.ChangeID, strings.Join(others, " "))
	}

	return index, nil
}

func parseIndex(out string) (Index, bool, error) {
	records := strings.Split(strings.TrimSuffix(out, recordSep), recordSep)
	if len(records) != 1 || records[0] == "" {
		return Index{}, false, fmt.Errorf("%w: the working copy has %d parents", ErrNoIndex, len(records))
	}

	fields := strings.SplitN(records[0], fieldSep, indexFieldCount)
	if len(fields) != indexFieldCount {
		return Index{}, false, fmt.Errorf("%w: unreadable parent %q", ErrCommandFailed, records[0])
	}

	index := Index{
		ChangeID:    fields[0],
		Empty:       strings.Contains(fields[1], "E"),
		Description: strings.TrimRight(fields[2], "\n"),
	}

	return index, strings.Contains(fields[1], "I"), nil
}

// Stage moves paths from @ into @-, git's `add`. The working copy's files do not change: only which
// revision records the change does.
func (c *Client) Stage(ctx context.Context, paths ...string) (string, error) {
	if _, err := c.Index(ctx); err != nil {
		return CommandLine(StageArgs(paths)), err
	}

	return c.mutate(ctx, StageArgs(paths)...)
}

// StageArgs is the invocation Stage runs. --keep-emptied keeps @ and its description in place when
// the last change leaves it, so staging everything never abandons the working-copy change.
func StageArgs(paths []string) []string {
	return SquashArgs("@", "@-", append([]string{keepEmptied}, filePatterns(paths)...))
}

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

// Unstage moves paths from @- back into @, git's `restore --staged`. Squashing into a descendant
// carries the change forward, so the working copy's files still do not change, and --keep-emptied
// keeps the index in place when it empties.
func (c *Client) Unstage(ctx context.Context, paths ...string) (string, error) {
	if _, err := c.Index(ctx); err != nil {
		return CommandLine(UnstageArgs(paths)), err
	}

	return c.mutate(ctx, UnstageArgs(paths)...)
}

// UnstageArgs is the invocation Unstage runs.
func UnstageArgs(paths []string) []string {
	return SquashArgs("@-", "@", append([]string{keepEmptied}, filePatterns(paths)...))
}

// CommitIndex describes @- with message and inserts an empty change between it and @ to be the next
// index, git's `commit`. The insert moves @'s parent pointer and leaves its content alone, so what
// was never staged stays in the working copy.
func (c *Client) CommitIndex(ctx context.Context, message string) (string, error) {
	index, err := c.Index(ctx)
	if err != nil {
		return "", err
	}

	if index.Empty {
		return "", ErrNothingStaged
	}

	described, err := c.mutate(ctx, "describe", "-r", "@-", "-m", message)
	if err != nil && !errors.Is(err, ErrNothingChanged) {
		return described, err
	}

	inserted, err := c.mutate(ctx, "new", "--insert-after", "@-", "--insert-before", "@", "--no-edit")

	return described + " && " + inserted, err
}
