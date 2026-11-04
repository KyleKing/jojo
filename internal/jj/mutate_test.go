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

	// Racing describes split one change's op heads into a divergent change, which is also the
	// exact interleaving the client's queue exists to prevent. One round can still reconcile
	// cleanly, so retry until jj reports the divergence.
	var sides []jj.LogRow
	for round := 0; round < 10 && len(sides) == 0; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				//nolint:gosec // G204: the binary is a literal and the args are test constants.
				cmd := exec.CommandContext(ctx, "jj", "describe", "-m", fmt.Sprintf("side-%d", i))
				cmd.Dir = dir
				_ = cmd.Run()
			}(i)
		}

		wg.Wait()

		rows, err := client.Log(ctx, jj.DivergentRevset, 0)
		if err != nil {
			t.Fatalf("Log over %q: %v", jj.DivergentRevset, err)
		}

		sides = rows
	}

	if len(sides) == 0 {
		t.Skip("the raced describes reconciled cleanly every round; no divergent change produced")
	}

	for _, row := range sides {
		if row.Revision == nil {
			continue
		}

		if !row.Revision.Divergent {
			t.Errorf("revision %s in the divergent set lacks the Divergent marker", row.Revision.ChangeID)
		}
	}

	if _, err := client.AbandonDivergent(ctx); err != nil {
		t.Fatalf("AbandonDivergent: %v", err)
	}

	rows, err := client.Log(ctx, jj.DivergentRevset, 0)
	if err != nil {
		t.Fatalf("Log after abandon: %v", err)
	}

	for _, row := range rows {
		if row.Revision != nil {
			t.Fatalf("divergent side %s survived the abandon", row.Revision.ChangeID)
		}
	}
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
