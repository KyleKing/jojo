package rounds_test

import (
	"testing"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/rounds"
)

func version(commit, prev, op, desc string, files int) jj.Version {
	v := jj.Version{ChangeID: "c", CommitID: commit, Operation: op, Description: desc, Files: files, Age: "1h ago"}
	if prev != "" {
		v.Predecessors = []jj.Predecessor{{ChangeID: "c", CommitID: prev}}
	}

	return v
}

// Consecutive rebases that change nothing fold into one round, a content change between them splits
// the run, and a describe that touched only the message stays a round of its own.
func TestGroup_FoldsRunsOfMovesOnly(t *testing.T) {
	t.Parallel()

	versions := []jj.Version{
		version("v1", "", "new empty commit", "feat", 1),
		version("v2", "v1", "rebase commit a", "feat", 0),
		version("v3", "v2", "rebase commit b", "feat", 0),
		version("v4", "v3", "snapshot working copy", "feat", 2),
		version("v5", "v4", "describe commit c", "feat: better", 0),
		version("v6", "v5", "rebase commit d", "feat: better", 0),
	}

	got := rounds.Group(versions)

	want := []struct {
		label       string
		from, to    string
		first, last int
	}{
		{"new, the first version", "", "v1", 1, 1},
		{rounds.Moved, "v1", "v3", 2, 3},
		{"snapshot", "v3", "v4", 4, 4},
		{"describe, description only", "v4", "v5", 5, 5},
		{rounds.Moved, "v5", "v6", 6, 6},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d rounds, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		r := got[i]
		if r.Label() != w.label || r.First != w.first || r.Last != w.last || r.From != w.from || r.To != w.to {
			t.Errorf("round %d = %s %d–%d %s..%s, want %s %d–%d %s..%s",
				i, r.Label(), r.First, r.Last, r.From, r.To, w.label, w.first, w.last, w.from, w.to)
		}
	}

	if b := got[1].Banner(6); b != "versions 2–3 of 6 · "+rounds.Moved+" · 1h ago" {
		t.Errorf("banner %q", b)
	}

	if b := got[2].Banner(6); b != "version 4 of 6 · snapshot · 1h ago" {
		t.Errorf("banner %q", b)
	}
}

// A squash names the squashed change's commit as a predecessor too; the round compares against the
// change's own previous version.
func TestVersion_PreviousPrefersTheChangesOwnPredecessor(t *testing.T) {
	t.Parallel()

	v := jj.Version{ChangeID: "c", Predecessors: []jj.Predecessor{
		{ChangeID: "other", CommitID: "o1"}, {ChangeID: "c", CommitID: "c1"},
	}}

	if got := v.Previous(); got != "c1" {
		t.Errorf("Previous() = %q, want the change's own c1", got)
	}

	split := jj.Version{ChangeID: "new", Predecessors: []jj.Predecessor{{ChangeID: "orig", CommitID: "o1"}}}
	if got := split.Previous(); got != "o1" {
		t.Errorf("Previous() of a split = %q, want the commit it was cut from", got)
	}
}
