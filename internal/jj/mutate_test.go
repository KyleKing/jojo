package jj_test

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func TestAbandonDivergent_RemovesTheStaleSides(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	runJJ(t, dir, "describe", "-m", "base")

	for _, row := range divergentSet(ctx, t, client, dir) {
		if row.Revision != nil && !row.Revision.Divergent {
			t.Errorf("revision %s in the divergent set lacks the Divergent marker", row.Revision.ChangeID)
		}
	}

	if _, err := client.AbandonDivergent(ctx); err != nil {
		t.Fatalf("AbandonDivergent: %v", err)
	}

	for _, row := range mustLog(ctx, t, client, jj.DivergentRevset) {
		if row.Revision != nil {
			t.Fatalf("divergent side %s survived the abandon", row.Revision.ChangeID)
		}
	}
}

// divergentSet races describes until jj reports the divergence, retrying because one round can
// still reconcile cleanly. Racing commands is also the exact interleaving the client's queue
// exists to prevent.
func divergentSet(ctx context.Context, t *testing.T, client *jj.Client, dir string) []jj.LogRow {
	t.Helper()

	for range 10 {
		var wg sync.WaitGroup
		for i := range 4 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				//nolint:gosec // G204: the binary is a literal and the args are test constants.
				cmd := exec.CommandContext(ctx, "jj", "describe", "-m", fmt.Sprintf("side-%d", i))
				cmd.Dir = dir

				if err := cmd.Run(); err != nil {
					// A losing racer fails to snapshot, which is expected interleaving, not a bug.
					t.Logf("raced describe %d: %v", i, err)
				}
			}(i)
		}

		wg.Wait()

		if rows := mustLog(ctx, t, client, jj.DivergentRevset); len(rows) > 0 {
			return rows
		}
	}

	t.Skip("the raced describes reconciled cleanly every round; no divergent change produced")

	return nil
}

func mustLog(ctx context.Context, t *testing.T, client *jj.Client, revset string) []jj.LogRow {
	t.Helper()

	rows, err := client.Log(ctx, revset, 0)
	if err != nil {
		t.Fatalf("Log over %q: %v", revset, err)
	}

	return rows
}

func TestAbandonDivergent_SucceedsWithNothingToClean(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	ran, err := client.AbandonDivergent(context.Background())
	if err != nil {
		t.Fatalf("AbandonDivergent: %v", err)
	}

	if ran != "jj abandon 'divergent() & ~::trunk()'" {
		t.Errorf("ran = %q, want the divergent cleanup invocation", ran)
	}
}
