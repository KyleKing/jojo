package jj_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/trail"

	"github.com/kyleking/jojo/internal/jj"
)

// Every call lands in the trail as the line a reader would type: a read as a read, a write with the
// operation it created so it can be undone back to, and a failure with all of jj's stderr.
func TestTrail_RecordsReadsWritesAndFailures(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	log := trail.New(0)
	client.WithTrail(log)

	if _, err := client.Log(t.Context(), "@", 1); err != nil {
		t.Fatalf("Log: %v", err)
	}

	ran, err := client.Describe(t.Context(), "@", "first")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if _, err := client.Edit(t.Context(), "nosuchrev"); err == nil {
		t.Fatal("editing a revision that does not exist succeeded")
	}

	calls := log.Calls(false)
	if len(calls) < 3 {
		t.Fatalf("the trail has %d calls, want the read, the write, and the failure", len(calls))
	}

	failed, wrote := calls[0], calls[1]
	if !failed.Failed() || !failed.Write || !strings.Contains(failed.Stderr, "nosuchrev") {
		t.Errorf("the failed edit is %+v, want a failed write with jj's stderr", failed)
	}

	if wrote.Shell() != ran || wrote.Failed() || wrote.Op == "" {
		t.Errorf("the describe is %q (op %q), want %q with the operation it made", wrote.Shell(), wrote.Op, ran)
	}

	if read, ok := log.Last(func(c *trail.Call) bool { return !c.Write }); !ok || read.Argv[1] != "log" {
		t.Errorf("the log read is %+v, want a read of jj log", read)
	}

	if got := log.Calls(true); len(got) != 2 {
		t.Errorf("writes only lists %d calls, want the describe and the failed edit", len(got))
	}
}

// A client with no trail records nothing and runs as it always has.
func TestTrail_NilRecordsNothing(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	if client.Trail() != nil {
		t.Fatal("a new client has a trail")
	}

	if _, err := client.Describe(t.Context(), "@", "x"); err != nil && !errors.Is(err, jj.ErrNothingChanged) {
		t.Fatalf("Describe: %v", err)
	}
}
