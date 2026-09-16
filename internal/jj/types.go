// Package jj runs the jj CLI and parses its output into structured revisions,
// operations, and repository state.
package jj

import "errors"

// Revision is one row of the log: everything a row renders plus the flags that
// decide whether an action is allowed on it.
type Revision struct {
	ChangeID    string
	CommitID    string
	Description string
	AuthorEmail string
	Age         string
	Graph       string
	Bookmarks   []string
	Added       int
	Removed     int
	Files       int
	Immutable   bool
	Conflict    bool
	Empty       bool
	WorkingCopy bool
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
	Tags        string
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
)
