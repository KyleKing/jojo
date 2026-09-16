package jj

import (
	"context"
	"errors"
)

// RebaseScope picks which revisions a rebase moves, mirroring jj's -r/-s/-b flags. There is no
// default: the caller always states the scope explicitly.
type RebaseScope int

// Scopes a rebase can target.
const (
	RebaseScopeRevisions RebaseScope = iota // -r
	RebaseScopeSource                       // -s
	RebaseScopeBranch                       // -b
)

// RebaseDestinationMode picks how a rebase's destination revisions are used, mirroring jj's
// --onto/--insert-after/--insert-before flags.
type RebaseDestinationMode int

// Destination modes a rebase can target.
const (
	RebaseDestOnto         RebaseDestinationMode = iota // --onto
	RebaseDestInsertAfter                               // --insert-after
	RebaseDestInsertBefore                              // --insert-before
)

// RebaseOptions makes a rebase's scope and destination explicit rather than inferred from the
// revisions passed.
type RebaseOptions struct {
	Revisions   []string
	Destination []string
	Mode        RebaseDestinationMode
	Scope       RebaseScope
}

var (
	errRebaseNoRevisions   = errors.New("rebase requires at least one revision")
	errRebaseNoDestination = errors.New("rebase requires at least one destination revision")
)

// ontoFlag lands the revisions on the destination rather than beside it, which is what a rebase
// means unless the caller says otherwise.
const ontoFlag = "--onto"

var (
	scopeFlags = map[RebaseScope]string{RebaseScopeSource: "-s", RebaseScopeBranch: "-b"}
	modeFlags  = map[RebaseDestinationMode]string{
		RebaseDestInsertAfter:  "--insert-after",
		RebaseDestInsertBefore: "--insert-before",
	}
)

// flag falls back to -r, which moves the revisions named and nothing else.
func (s RebaseScope) flag() string {
	if f, ok := scopeFlags[s]; ok {
		return f
	}

	return "-r"
}

func (m RebaseDestinationMode) flag() string {
	if f, ok := modeFlags[m]; ok {
		return f
	}

	return ontoFlag
}

// New creates a new change on top of revs, or on top of @ when revs is empty.
func (c *Client) New(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"new"}, revs...)

	return c.mutate(ctx, args...)
}

// Edit makes rev the working-copy commit.
func (c *Client) Edit(ctx context.Context, rev string) (string, error) {
	return c.mutate(ctx, "edit", rev)
}

// Describe sets rev's change description.
func (c *Client) Describe(ctx context.Context, rev, message string) (string, error) {
	return c.mutate(ctx, "describe", "-r", rev, "-m", message)
}

// Squash moves from's changes into into.
func (c *Client) Squash(ctx context.Context, from, into string) (string, error) {
	// Without --use-destination-message jj opens $EDITOR to combine the two
	// descriptions, which blocks a TUI that has its own describe flow.
	return c.mutate(ctx, "squash", "--from", from, "--into", into, "--use-destination-message")
}

// Abandon abandons revs, rebasing their descendants onto their parents.
func (c *Client) Abandon(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"abandon"}, revs...)

	return c.mutate(ctx, args...)
}

// Absorb moves rev's changes into the closest mutable ancestor where each line last changed.
func (c *Client) Absorb(ctx context.Context, rev string) (string, error) {
	return c.mutate(ctx, "absorb", "-r", rev)
}

// Duplicate creates new changes with the same content as revs.
func (c *Client) Duplicate(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"duplicate"}, revs...)

	return c.mutate(ctx, args...)
}

// Rebase moves opts.Revisions, scoped by opts.Scope, to opts.Destination.
func (c *Client) Rebase(ctx context.Context, opts RebaseOptions) (string, error) {
	if len(opts.Revisions) == 0 {
		return "", errRebaseNoRevisions
	}

	if len(opts.Destination) == 0 {
		return "", errRebaseNoDestination
	}

	args := []string{"rebase"}

	scopeFlag := opts.Scope.flag()
	for _, rev := range opts.Revisions {
		args = append(args, scopeFlag, rev)
	}

	destFlag := opts.Mode.flag()
	for _, rev := range opts.Destination {
		args = append(args, destFlag, rev)
	}

	return c.mutate(ctx, args...)
}

// Parallelize turns a chain of revisions into siblings of one another.
func (c *Client) Parallelize(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"parallelize"}, revs...)

	return c.mutate(ctx, args...)
}

// Undo reverts the last jj operation in the repository.
func (c *Client) Undo(ctx context.Context) (string, error) {
	return c.mutate(ctx, "undo")
}

// OpRestore restores the repository to the state at opID.
func (c *Client) OpRestore(ctx context.Context, opID string) (string, error) {
	return c.mutate(ctx, "op", "restore", opID)
}
