package jjdiff_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kyleking/jojo/internal/jjdiff"
)

// The probe runs jj-diff once per binary: a second launch answers from the cache, and a rebuilt
// binary (a new modification time) is asked again.
func TestProbeCached_AsksOncePerBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bin := filepath.Join(dir, "jj-diff")

	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	find := func(string) (string, error) { return bin, nil }
	asked := 0
	run := func(ctx context.Context, path string, args ...string) ([]byte, error) {
		asked++

		return runner("0.12.1", "")(ctx, path, args...)
	}

	cache := filepath.Join(dir, "cache")

	for range 3 {
		if tool := jjdiff.ProbeCached(t.Context(), find, run, cache); !tool.Modes || tool.Version != "0.12.1" {
			t.Fatalf("ProbeCached = %+v, want 0.12.1 with its modes", tool)
		}
	}

	if asked != 1 {
		t.Fatalf("jj-diff was asked %d times over three launches, want once", asked)
	}

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(bin, later, later); err != nil {
		t.Fatal(err)
	}

	jjdiff.ProbeCached(t.Context(), find, run, cache)

	if asked != 2 {
		t.Fatalf("a rebuilt jj-diff was asked %d times in all, want a second probe", asked)
	}
}
