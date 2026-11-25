package jj

import (
	"context"
	"errors"
	"fmt"
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
	// Flags are jj's own toggles, spelled as jj spells them (--skip-emptied, --keep-divergent,
	// --simplify-parents), appended as given.
	Flags []string
	Mode  RebaseDestinationMode
	Scope RebaseScope
	// SkipEmptied abandons a revision the rebase leaves empty, which is what landing a stack on a
	// trunk that already took some of it usually wants.
	SkipEmptied bool
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

// Commit describes @ with message and starts a new change on top of it, as one operation.
func (c *Client) Commit(ctx context.Context, message string) (string, error) {
	return c.mutate(ctx, "commit", "-m", message)
}

// RestoreArgs puts from's content into into, which with from as into's parent discards into's
// changes. Flags (--restore-descendants) follow.
func RestoreArgs(from, into string, flags ...string) []string {
	return append([]string{"restore", "--from", from, "--into", into}, flags...)
}

// Edit makes rev the working-copy commit.
func (c *Client) Edit(ctx context.Context, rev string) (string, error) {
	return c.mutate(ctx, "edit", rev)
}

// Describe sets rev's change description.
func (c *Client) Describe(ctx context.Context, rev, message string) (string, error) {
	return c.mutate(ctx, "describe", "-r", rev, "-m", message)
}

// Squash moves from's changes into into. Each path names one file exactly, so a space or a
// parenthesis in it reaches jj as part of the name rather than as fileset syntax.
func (c *Client) Squash(ctx context.Context, from, into string, paths ...string) (string, error) {
	return c.mutate(ctx, SquashArgs(from, into, filePatterns(paths))...)
}

// SquashArgs is the invocation Squash runs, exported because moving part of a revision needs the
// same command handed to a diff editor holding the terminal.
func SquashArgs(from, into string, paths []string) []string {
	// Without --use-destination-message jj opens $EDITOR to combine the two descriptions, which
	// blocks a TUI that has its own describe flow.
	const fixed = 6

	args := make([]string, 0, fixed+len(paths))
	args = append(args, "squash", "--from", from, "--into", into, "--use-destination-message")

	return append(args, paths...)
}

// SquashAll moves every one of sources into into, as one operation.
func (c *Client) SquashAll(ctx context.Context, sources []string, into string) (string, error) {
	// A flag and a value per source, then the destination and the message flag.
	args := make([]string, 0, 2*len(sources)+4)
	args = append(args, "squash")

	for _, from := range sources {
		args = append(args, "--from", from)
	}

	return c.mutate(ctx, append(args, "--into", into, "--use-destination-message")...)
}

// Abandon abandons revs, rebasing their descendants onto their parents.
func (c *Client) Abandon(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"abandon"}, revs...)

	return c.mutate(ctx, args...)
}

// DivergentRevset is every stale side a rewrite left behind, minus whatever the trunk already
// keeps, so abandoning it never removes the live version of a pushed change.
const DivergentRevset = "divergent() & ~::trunk()"

// ConvergeRevset is every divergent revision jj converge would consider by default, so the dialog
// lists the sides converge replaces.
const ConvergeRevset = "mutable() & divergent()"

// AbandonDivergent removes the divergent sides listing them left: rewriting a change after its
// commit was pushed strands the pushed version's twin, and they accumulate silently.
func (c *Client) AbandonDivergent(ctx context.Context) (string, error) {
	return c.mutate(ctx, "abandon", DivergentRevset)
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

	return c.mutate(ctx, RebaseArgs(opts)...)
}

// RebaseArgs is the invocation Rebase runs, exported so a preview can run the same command
// somewhere else and the footer can show it before it runs.
func RebaseArgs(opts RebaseOptions) []string {
	args := []string{"rebase"}

	scopeFlag := opts.Scope.flag()
	for _, rev := range opts.Revisions {
		args = append(args, scopeFlag, rev)
	}

	destFlag := opts.Mode.flag()
	for _, rev := range opts.Destination {
		args = append(args, destFlag, rev)
	}

	if opts.SkipEmptied {
		args = append(args, "--skip-emptied")
	}

	return append(args, opts.Flags...)
}

// SquashOptions describes a squash of several revisions into one.
type SquashOptions struct {
	Into    string
	Sources []string
	// KeepEmptied keeps a source the squash empties rather than abandoning it.
	KeepEmptied bool
	// Combine leaves the descriptions to jj, which opens an editor holding both whenever the two
	// differ, so a caller setting it hands jj the terminal.
	Combine bool
}

// SquashIntoArgs is the invocation squashing opts.Sources into opts.Into as one operation.
func SquashIntoArgs(opts SquashOptions) []string {
	args := []string{"squash"}
	for _, from := range opts.Sources {
		args = append(args, "--from", from)
	}

	args = append(args, "--into", opts.Into)

	if !opts.Combine {
		args = append(args, "--use-destination-message")
	}

	if opts.KeepEmptied {
		args = append(args, "--keep-emptied")
	}

	return args
}

// PlaceOptions says where new or duplicated revisions go: onto Destination as parents, or inserted
// after or before it.
type PlaceOptions struct {
	Destination []string
	Mode        RebaseDestinationMode
}

// Args is the placement as jj's flags: -o, -A, or -B before each destination.
func (p PlaceOptions) Args() []string {
	flag := p.Mode.flag()
	args := make([]string, 0, 2*len(p.Destination))

	for _, rev := range p.Destination {
		args = append(args, flag, rev)
	}

	return args
}

// NewArgs creates a revision placed by where. With noEdit set, @ stays where it is rather than moving.
func NewArgs(where PlaceOptions, noEdit bool) []string {
	args := []string{"new"}

	// Onto is jj new's positional form, which reads the way people type it.
	if where.Mode == RebaseDestOnto {
		args = append(args, where.Destination...)
	} else {
		args = append(args, where.Args()...)
	}

	if noEdit {
		args = append(args, "--no-edit")
	}

	return args
}

// DuplicateArgs copies revs, beside the originals when where names no destination.
func DuplicateArgs(revs []string, where PlaceOptions) []string {
	args := append([]string{"duplicate"}, revs...)

	return append(args, where.Args()...)
}

// Apply runs a write built by one of the *Args functions, so the command a preview drew and the
// footer showed is the one that runs.
func (c *Client) Apply(ctx context.Context, args []string) (string, error) {
	return c.mutate(ctx, args...)
}

// Parallelize turns a chain of revisions into siblings of one another.
func (c *Client) Parallelize(ctx context.Context, revs []string) (string, error) {
	args := append([]string{"parallelize"}, revs...)

	return c.mutate(ctx, args...)
}

// Undo reverts the last jj operation in the repository. It refuses with ErrUndoRefused where that
// would delete edits on disk, since the operation is a snapshot of them, or cannot reach what it
// changed, as with a push. Since jj undo snapshots before it reverses, the check snapshots first
// too: an edit made since the last log read would otherwise be the operation undone.
func (c *Client) Undo(ctx context.Context) (string, error) {
	target, ok, err := c.UndoTargetNow(ctx)
	if err != nil {
		return "", err
	}

	if why := target.Refusal(); ok && why != "" {
		return "", fmt.Errorf("%w: %s", ErrUndoRefused, why)
	}

	return c.mutate(ctx, "undo")
}

// Redo reapplies the operation the last undo reverted, and fails with jj's "Nothing to redo" when
// no undo precedes it.
func (c *Client) Redo(ctx context.Context) (string, error) {
	return c.mutate(ctx, "redo")
}

// OpRestore restores the repository to the state at opID.
func (c *Client) OpRestore(ctx context.Context, opID string) (string, error) {
	return c.mutate(ctx, "op", "restore", opID)
}
