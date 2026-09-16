package jj_test

import (
	"context"
	"strings"
	"testing"
)

// TestOpRestoreToTheOperationBeforeAnUndoRedoesIt pins the mechanism the redo key is built on,
// because jj has no redo command: the way forward is restoring to the operation the undo left.
func TestOpRestoreToTheOperationBeforeAnUndoRedoesIt(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "first")
	runJJ(t, dir, "new", "-m", "second")

	ops, err := client.OpLog(ctx, 1)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	before := ops[0].ID

	if _, err := client.Undo(ctx); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if got := strings.TrimSpace(descriptionOf(t, dir)); got == "second" {
		t.Fatal("undo left the working copy where it was")
	}

	if _, err := client.OpRestore(ctx, before); err != nil {
		t.Fatalf("OpRestore: %v", err)
	}

	if got := strings.TrimSpace(descriptionOf(t, dir)); got != "second" {
		t.Errorf("working copy is %q after the redo, want %q", got, "second")
	}
}
