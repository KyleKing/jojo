// Package jj runs the jj CLI and parses its output into structured revisions,
// operations, and repository state.
package jj

import "errors"

// Revision is one row of the log: everything a row renders plus the flags that
// decide whether an action is allowed on it.
type Revision struct {
	ChangeID    string
	Description string
	AuthorEmail string
	Age         string
	Graph       string
	// Bookmarks is what the row renders, so it keeps jj's markers: a trailing
	// "*" for a local bookmark ahead of its remote, and "name@remote" forms.
	Bookmarks []string
	// LocalBookmarks is the same set with jj's markers stripped and remotes
	// dropped, which is what a forge can be asked about.
	LocalBookmarks []string
	// RemoteBookmarks names the branches this revision is on at a remote, with
	// the "@remote" suffix dropped, so a revision that was pushed but never
	// tracked locally can still be matched to a pull request.
	RemoteBookmarks []string
	Added           int
	Removed         int
	Files           int
	Immutable       bool
	Conflict        bool
	Empty           bool
	WorkingCopy     bool
}

// GraphLine is a log line carrying no revision, such as a connector between two
// rows or the `~` marking elided ancestors. It renders in place so the graph
// stays continuous.
type GraphLine struct {
	Graph string
}

// LogRow is either a Revision or a GraphLine. Exactly one field is non-nil.
type LogRow struct {
	Revision *Revision
	Graph    *GraphLine
}

// Operation is one entry of `jj op log`, the undo history.
type Operation struct {
	ID          string
	Description string
	Age         string
	Current     bool
}

// Errors callers inspect with errors.Is.
var (
	// ErrCommandFailed wraps any non-zero jj exit.
	ErrCommandFailed = errors.New("jj command failed")
	// ErrRevsetSyntax marks a revset the user is still typing, which the log
	// view reports in the revset box rather than as a failure.
	ErrRevsetSyntax = errors.New("invalid revset")
	// ErrImmutable marks an edit refused because the target is immutable.
	ErrImmutable = errors.New("revision is immutable")
	// ErrPushRefused marks a push jj declined for a reason the user can fix, such as a revision
	// with no description, rather than a failure of the command itself.
	ErrPushRefused = errors.New("push refused")
	// ErrNothingChanged marks a jj command that succeeded and moved nothing, so
	// a caller must not report the edit as done.
	ErrNothingChanged = errors.New("nothing changed")
)

// Evolution is one entry of `jj evolog`: one version a change passed through,
// and the operation that produced it.
type Evolution struct {
	CommitID    string
	Description string
	Age         string
	Operation   string
	Empty       bool
}

// Workspace is one entry of `jj workspace list`, a working copy attached to
// this repository.
type Workspace struct {
	Name        string
	ChangeID    string
	Description string
	Root        string
	Current     bool
}

// Bookmark is one entry of `jj bookmark list`: the local bookmark when Remote is empty, and one of
// its remote counterparts otherwise.
type Bookmark struct {
	Name        string
	Remote      string
	ChangeID    string
	Description string
	Ahead       int
	Behind      int
	Tracked     bool
	Conflict    bool
}
