package jj_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func TestEvologReturnsEveryVersionNewestFirst(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "one\n")
	runJJ(t, dir, "describe", "-m", "first wording")
	runJJ(t, dir, "describe", "-m", "second wording")

	entries, err := client.Evolog(ctx, "@")
	if err != nil {
		t.Fatalf("Evolog: %v", err)
	}

	if len(entries) < 2 {
		t.Fatalf("expected at least two versions, got %d", len(entries))
	}

	if entries[0].Description != "second wording" {
		t.Errorf("newest entry is %q, want the latest description first", entries[0].Description)
	}

	for i, entry := range entries {
		if entry.CommitID == "" || entry.Age == "" || entry.Operation == "" {
			t.Errorf("entry %d is missing a field: %+v", i, entry)
		}
	}
}

// TestRestoreBringsBackAnEarlierVersionsContent covers the evolution timeline's restore key, which is
// the only way back to a version that `jj undo` can no longer reach in one step.
func TestRestoreBringsBackAnEarlierVersionsContent(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "original\n")
	runJJ(t, dir, "describe", "-m", "keep me")
	runJJ(t, dir, "new", "-m", "later")
	writeFile(t, dir, "a.txt", "replaced\n")

	entries, err := client.Evolog(ctx, "@-")
	if err != nil {
		t.Fatalf("Evolog: %v", err)
	}

	if err := client.Restore(ctx, entries[0].CommitID, "@"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if content := readFile(t, dir, "a.txt"); content != "original\n" {
		t.Errorf("a.txt is %q, want the restored content", content)
	}
}

func TestRestoreThatChangesNothingReportsIt(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "same\n")
	runJJ(t, dir, "describe", "-m", "only change")

	entries, err := client.Evolog(ctx, "@")
	if err != nil {
		t.Fatalf("Evolog: %v", err)
	}

	if err := client.Restore(ctx, entries[0].CommitID, "@"); !errors.Is(err, jj.ErrNothingChanged) {
		t.Errorf("restoring a revision from itself returned %v, want ErrNothingChanged", err)
	}
}
