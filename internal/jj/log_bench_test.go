package jj_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// benchRepoEnv names a repository to time the log against, such as a synthetic 3000-commit history.
// Without it the benchmark builds a small linear one, so `mise run bench` stays quick.
const benchRepoEnv = "JOJO_BENCH_REPO"

// benchHistory is how many commits the fallback repository gets.
const benchHistory = 60

func benchRepo(b *testing.B) *jj.Client {
	b.Helper()

	if dir := os.Getenv(benchRepoEnv); dir != "" {
		return jj.NewClient(dir)
	}

	client, dir := testRepo(b)

	for i := range benchHistory {
		writeFile(b, dir, "file.txt", fmt.Sprintf("line %d\n", i))
		runJJ(b, dir, "commit", "-m", fmt.Sprintf("commit %d", i))
	}

	return client
}

// BenchmarkLog_StartupWindow times the read the first frame waits on: the startup window of rows.
func BenchmarkLog_StartupWindow(b *testing.B) {
	client := benchRepo(b)
	ctx := context.Background()

	for b.Loop() {
		if _, err := client.Log(ctx, "::@", 500); err != nil {
			b.Fatalf("Log: %v", err)
		}
	}
}

// firstPaintRows is roughly a tall terminal plus the rows the model reads ahead of it.
const firstPaintRows = 80

// BenchmarkLog_FirstPaint is what the first frame now waits on: the graph without stats, then the
// stats of the rows on screen. Compare it with BenchmarkLog_StatsForWholeWindow, the old cost.
func BenchmarkLog_FirstPaint(b *testing.B) {
	client := benchRepo(b)
	ctx := context.Background()

	for b.Loop() {
		rows, err := client.Log(ctx, "::@", 500)
		if err != nil {
			b.Fatalf("Log: %v", err)
		}

		if _, err := client.Stats(ctx, commitIDs(rows, firstPaintRows)); err != nil {
			b.Fatalf("Stats: %v", err)
		}
	}
}

// BenchmarkLog_StatsForWholeWindow reads the graph and the stats of every row in it, which is what
// a single log template carrying the stats used to cost.
func BenchmarkLog_StatsForWholeWindow(b *testing.B) {
	client := benchRepo(b)
	ctx := context.Background()

	for b.Loop() {
		rows, err := client.Log(ctx, "::@", 500)
		if err != nil {
			b.Fatalf("Log: %v", err)
		}

		if _, err := client.Stats(ctx, commitIDs(rows, len(rows))); err != nil {
			b.Fatalf("Stats: %v", err)
		}
	}
}

func commitIDs(rows []jj.LogRow, limit int) []string {
	ids := make([]string, 0, limit)

	for _, row := range rows {
		if row.Revision != nil && len(ids) < limit {
			ids = append(ids, row.Revision.CommitID)
		}
	}

	return ids
}
