package jj_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// immutableRepo pins trunk() as immutable, which is what jj's default immutable_heads() does in a
// repository that has a main bookmark.
func immutableRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "published")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "main")
	runJJ(t, dir, "config", "set", "--repo", `revset-aliases."immutable_heads()"`, "main")
	runJJ(t, dir, "describe", "-m", "local work")

	return client, dir
}

// The escape hatch is the only way to edit a revision jj protects, and it has to stay off unless a
// caller asks for it by name, because it rewrites history other people may already have.
func TestIgnoringImmutableIsTheOnlyWayPast(t *testing.T) {
	t.Parallel()

	client, dir := immutableRepo(t)
	ctx := context.Background()

	err := client.Describe(ctx, "main", "rewritten")
	if !errors.Is(err, jj.ErrImmutable) {
		t.Fatalf("describing an immutable revision returned %v, want ErrImmutable", err)
	}

	if err = client.IgnoringImmutable().Describe(ctx, "main", "rewritten"); err != nil {
		t.Fatalf("IgnoringImmutable().Describe: %v", err)
	}

	if !strings.Contains(descriptionOf(t, dir), "local work") {
		t.Error("the working copy changed, so the rewrite landed on the wrong revision")
	}

	if err = client.Describe(ctx, "main", "again"); !errors.Is(err, jj.ErrImmutable) {
		t.Errorf("the original client stopped refusing immutable edits: %v", err)
	}
}

func TestParallelizeMakesAChainIntoSiblings(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")
	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "commit", "-m", "second")

	if err := client.Parallelize(ctx, []string{"@--::@-"}); err != nil {
		t.Fatalf("Parallelize: %v", err)
	}

	// Both revisions are now parents of the working copy, which is what makes them siblings.
	parents, err := client.Log(ctx, "@-", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	if got := countRevisions(parents); got != 2 {
		t.Errorf("the working copy has %d parents, want the two parallelized revisions", got)
	}
}

func countRevisions(rows []jj.LogRow) int {
	n := 0

	for _, row := range rows {
		if row.Revision != nil {
			n++
		}
	}

	return n
}

// Revert undoes one operation and leaves everything done after it alone, which is what separates it
// from restore throwing away every later operation.
func TestOpRevertKeepsLaterOperations(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "first")
	runJJ(t, dir, "bookmark", "create", "-r", "@", "doomed")

	ops, err := client.OpLog(ctx, 1)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	bookmarkOp := ops[0].ID

	runJJ(t, dir, "new", "-m", "later work")

	if err = client.OpRevert(ctx, bookmarkOp); err != nil {
		t.Fatalf("OpRevert: %v", err)
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if findBookmark(marks, "doomed") != nil {
		t.Error("the reverted operation's bookmark is still there")
	}

	if !strings.Contains(descriptionOf(t, dir), "later work") {
		t.Error("reverting one operation threw away the work done after it")
	}
}
