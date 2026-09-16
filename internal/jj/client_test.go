package jj_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func TestCheckInstalledAndRoot(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	ctx := context.Background()

	if err := client.CheckInstalled(ctx); err != nil {
		t.Fatalf("CheckInstalled: %v", err)
	}

	root, err := client.Root(ctx)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	if root == "" {
		t.Fatal("Root returned an empty path")
	}
}

func TestDefaultRevset(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	revset, err := client.DefaultRevset(context.Background())
	if err != nil {
		t.Fatalf("DefaultRevset: %v", err)
	}

	if revset == "" {
		t.Fatal("DefaultRevset returned an empty revset")
	}
}

func TestLogParsesRealRepoShapes(t *testing.T) {
	t.Parallel()

	client, _ := seedRepo(t)

	rows, err := client.Log(context.Background(), "::", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	byDescription, graphLines := indexLogRows(t, rows)

	if graphLines == 0 {
		t.Error("expected at least one graph-only connector line from the merge shape")
	}

	merge := byDescription["merge-conflict"]
	root := byDescription[""]
	empty := byDescription["empty change"]
	bookmarked := byDescription["second"]

	if merge == nil {
		t.Fatal("did not find the merge-conflict revision")
	}

	if !merge.Conflict {
		t.Error("merge-conflict revision should report Conflict=true")
	}

	if !merge.Empty {
		t.Error("merge-conflict revision should be empty (no working-copy edits of its own)")
	}

	if root == nil {
		t.Fatal("did not find the immutable root revision")
	}

	if !root.Immutable {
		t.Error("root revision should report Immutable=true")
	}

	if empty == nil {
		t.Fatal("did not find the empty-change revision")
	}

	if bookmarked == nil {
		t.Fatal("did not find the bookmarked 'second' revision")
	}

	if len(bookmarked.Bookmarks) != 1 || bookmarked.Bookmarks[0] != "main" {
		t.Errorf("expected bookmarked revision to carry [main], got %v", bookmarked.Bookmarks)
	}

	if bookmarked.Files != 1 || bookmarked.Added != 1 {
		t.Errorf("expected 'second' to add 1 file/1 line, got files=%d added=%d", bookmarked.Files, bookmarked.Added)
	}
}

// indexLogRows keys revisions by description, which the fixture keeps unique, and counts graph-only lines.
func indexLogRows(t *testing.T, rows []jj.LogRow) (map[string]*jj.Revision, int) {
	t.Helper()

	byDescription := make(map[string]*jj.Revision)

	graphLines := 0

	for _, row := range rows {
		if row.Graph != nil {
			graphLines++

			continue
		}

		rev := row.Revision
		if rev.Graph == "" {
			t.Errorf("revision %q has no graph prefix", rev.ChangeID)
		}

		byDescription[rev.Description] = rev
	}

	return byDescription, graphLines
}

func TestLogBadRevsetReturnsSyntaxError(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	_, err := client.Log(context.Background(), "bogus(((", 0)
	if !errors.Is(err, jj.ErrRevsetSyntax) {
		t.Fatalf("expected ErrRevsetSyntax, got %v", err)
	}
}

func TestEditImmutableReturnsImmutableError(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	ctx := context.Background()

	err := client.Edit(ctx, "root()")
	if !errors.Is(err, jj.ErrImmutable) {
		t.Fatalf("expected ErrImmutable, got %v", err)
	}
}

func TestUndoRoundTripsADescribe(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	before := descriptionOf(t, dir)

	if err := client.Describe(ctx, "@", "changed by test"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	after := descriptionOf(t, dir)
	if after == before {
		t.Fatalf("Describe did not change the description")
	}

	if err := client.Undo(ctx); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	restored := descriptionOf(t, dir)
	if restored != before {
		t.Fatalf("Undo did not restore the description: got %q, want %q", restored, before)
	}
}

func TestOpLogAndOpRestore(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	ops, err := client.OpLog(ctx, 0)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	if len(ops) == 0 {
		t.Fatal("expected at least one operation")
	}

	if !ops[0].Current {
		t.Error("expected the newest operation to be marked Current")
	}

	before := descriptionOf(t, dir)

	if err := client.Describe(ctx, "@", "changed by test"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if err := client.OpRestore(ctx, ops[0].ID); err != nil {
		t.Fatalf("OpRestore: %v", err)
	}

	restored := descriptionOf(t, dir)
	if restored != before {
		t.Fatalf("OpRestore did not restore state: got %q, want %q", restored, before)
	}
}

func TestRebaseRequiresExplicitScopeAndDestination(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	ctx := context.Background()

	onlyDestination := jj.RebaseOptions{Destination: jj.RebaseDestination{Revisions: []string{"@"}}}
	if err := client.Rebase(ctx, onlyDestination); err == nil {
		t.Fatal("expected an error when no revisions are given")
	}

	if err := client.Rebase(ctx, jj.RebaseOptions{Revisions: []string{"@"}}); err == nil {
		t.Fatal("expected an error when no destination is given")
	}
}
