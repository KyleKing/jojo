// Package rounds turns a change's evolution into review rounds: one round per version that changed
// what the change says, each compared with the version before it, and the versions that only moved
// the change folded into the round they sit in so stepping through them costs one keypress.
package rounds

import (
	"fmt"

	"github.com/kyleking/jojo/internal/jj"
)

// Moved labels a run of versions whose interdiff is empty and whose description did not change: a
// rebase, or a snapshot of a parent's edit, which moved the change without changing it.
const Moved = "rebased, no content change"

// Round is one step through a change's evolution: the versions First through Last (1-based, oldest
// first), compared from the commit before First to the commit Last made.
type Round struct {
	// From is the commit the interdiff starts at, "" for the round that created the change, which has
	// nothing before it and shows its own diff instead.
	From string
	// To is the commit the round ends at.
	To string
	// Verb is the jj command that made the round's last version ("rebase", "describe", "squash").
	Verb string
	Age  string
	// Before and After are the description either side, equal when the round left it alone.
	Before string
	After  string
	First  int
	Last   int
	// Files, Added, and Removed size the interdiff.
	Files   int
	Added   int
	Removed int
	// Moved marks a run of versions that changed neither the patch nor the description.
	Moved bool
}

// Group folds versions, oldest first, into rounds. Consecutive versions that moved the change
// without changing it become one Moved round, since a rebase onto a new trunk is a step worth
// knowing about and not worth reading three times.
func Group(versions []jj.Version) []Round {
	var out []Round

	for i := range versions {
		v := &versions[i]
		before := ""
		if i > 0 {
			before = versions[i-1].Description
		}

		moved := i > 0 && v.Files == 0 && v.Description == before

		if moved && len(out) > 0 && out[len(out)-1].Moved {
			last := &out[len(out)-1]
			last.Last, last.To, last.Age, last.Verb = i+1, v.CommitID, v.Age, jj.VerbOf(v.Operation)

			continue
		}

		out = append(out, Round{
			From:    v.Previous(),
			To:      v.CommitID,
			Verb:    jj.VerbOf(v.Operation),
			Age:     v.Age,
			Before:  before,
			After:   v.Description,
			First:   i + 1,
			Last:    i + 1,
			Files:   v.Files,
			Added:   v.Added,
			Removed: v.Removed,
			Moved:   moved,
		})
	}

	return out
}

// Label is what the round did, in a word or a phrase.
func (r Round) Label() string {
	switch {
	case r.Moved:
		return Moved
	case r.From == "":
		return r.Verb + ", the first version"
	case r.Files == 0 && r.Before != r.After:
		return r.Verb + ", description only"
	}

	return r.Verb
}

// Span names the round's versions out of total: "version 3 of 5", or "versions 2–4 of 5" for a
// folded run.
func (r Round) Span(total int) string {
	if r.First == r.Last {
		return fmt.Sprintf("version %d of %d", r.First, total)
	}

	return fmt.Sprintf("versions %d–%d of %d", r.First, r.Last, total)
}

// Banner is the line the docked diff carries while it shows the round: where it is in the
// evolution, what made it, and when.
func (r Round) Banner(total int) string {
	return r.Span(total) + " · " + r.Label() + " · " + r.Age
}

// Size is the interdiff's size as a row shows it, "" when it is empty.
func (r Round) Size() string {
	if r.Files == 0 {
		return ""
	}

	return fmt.Sprintf("%df +%d -%d", r.Files, r.Added, r.Removed)
}

// Prose is what the docked diff draws above the interdiff: the description before and after when the
// round changed it, and a sentence for a round that has nothing to diff.
func (r Round) Prose() string {
	var out string

	if r.From != "" && r.Before != r.After {
		out += "description was: " + firstLine(r.Before) + "\n" + "description now: " + firstLine(r.After) + "\n"
	}

	if r.Moved {
		out += "(" + Moved + ")\n"
	}

	return out
}

func firstLine(text string) string {
	for i, c := range text {
		if c == '\n' {
			return text[:i]
		}
	}

	if text == "" {
		return "(no description set)"
	}

	return text
}
