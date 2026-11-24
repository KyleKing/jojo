package jj_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// TestRedo_ReappliesWhatUndoReverted pins the redo key on jj's own redo (0.33+), which replaced an
// emulation that restored the operation recorded before the undo.
func TestRedo_ReappliesWhatUndoReverted(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "first")
	runJJ(t, dir, "new", "-m", "second")

	if _, err := client.Undo(ctx); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if got := strings.TrimSpace(descriptionOf(t, dir)); got == "second" {
		t.Fatal("undo left the working copy where it was")
	}

	ran, err := client.Redo(ctx)
	if err != nil {
		t.Fatalf("Redo: %v", err)
	}

	if ran != "jj redo" {
		t.Errorf("ran %q, want jj redo", ran)
	}

	if got := strings.TrimSpace(descriptionOf(t, dir)); got != "second" {
		t.Errorf("working copy is %q after the redo, want %q", got, "second")
	}
}

func TestRedo_WithNothingUndoneSaysSo(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	_, err := client.Redo(context.Background())
	if !errors.Is(err, jj.ErrCommandFailed) || !strings.Contains(err.Error(), "Nothing to redo") {
		t.Errorf("Redo with nothing undone = %v, want jj's own refusal", err)
	}
}
