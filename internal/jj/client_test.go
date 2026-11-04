package jj_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kyleking/jojo/internal/jj"
)

func TestCheckInstalledAndRoot(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	ctx := context.Background()

	if err := client.CheckInstalled(); err != nil {
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

func TestRevsetAliasesIncludesTheBuiltins(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	names, err := client.RevsetAliases(context.Background())
	if err != nil {
		t.Fatalf("RevsetAliases: %v", err)
	}

	want := map[string]bool{"trunk": true, "immutable_heads": true, "mutable": true}
	for _, name := range names {
		delete(want, name)
	}

	if len(want) != 0 {
		t.Errorf("RevsetAliases = %v, missing built-ins %v", names, want)
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

	_, err := client.Edit(ctx, "root()")
	if !errors.Is(err, jj.ErrImmutable) {
		t.Fatalf("expected ErrImmutable, got %v", err)
	}
}

func TestUndoRoundTripsADescribe(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	before := descriptionOf(t, dir)

	if _, err := client.Describe(ctx, "@", "changed by test"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	after := descriptionOf(t, dir)
	if after == before {
		t.Fatalf("Describe did not change the description")
	}

	if _, err := client.Undo(ctx); err != nil {
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

	if _, err := client.Describe(ctx, "@", "changed by test"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if _, err := client.OpRestore(ctx, ops[0].ID); err != nil {
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

	onlyDestination := jj.RebaseOptions{Destination: []string{"@"}}
	if _, err := client.Rebase(ctx, onlyDestination); err == nil {
		t.Fatal("expected an error when no revisions are given")
	}

	if _, err := client.Rebase(ctx, jj.RebaseOptions{Revisions: []string{"@"}}); err == nil {
		t.Fatal("expected an error when no destination is given")
	}
}

// Two described revisions are what make jj reach for $EDITOR to combine them,
// which would block a TUI holding the terminal.
func TestSquashBetweenTwoDescribedRevisionsDoesNotBlock(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := t.Context()

	writeFile(t, dir, "a.txt", "one\n")
	runJJ(t, dir, "describe", "-m", "destination")
	runJJ(t, dir, "new", "-m", "source")
	writeFile(t, dir, "b.txt", "two\n")

	rows, err := client.Log(ctx, "@ | @-", 0)
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	source, destination := rows[0].Revision.ChangeID, rows[1].Revision.ChangeID

	done := make(chan error, 1)
	go func() {
		_, err := client.Squash(ctx, source, destination)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("squash: %v", err)
		}
	case <-time.After(squashTimeout):
		t.Fatal("squash blocked, most likely waiting on an editor")
	}

	after, err := client.Log(ctx, destination, 0)
	if err != nil {
		t.Fatalf("log after squash: %v", err)
	}

	if got := after[0].Revision.Description; got != "destination" {
		t.Errorf("description after squash = %q, want the destination's kept", got)
	}
}

// squashTimeout is long enough for a cold jj on a loaded machine and far short
// of an editor that never returns.
const squashTimeout = 30 * time.Second

// jj exits zero when a command moves nothing, so a caller that only checks the
// exit status would report an edit that never happened.
func TestMutationThatMovesNothingReportsIt(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := t.Context()

	runJJ(t, dir, "describe", "-m", "base")
	runJJ(t, dir, "new", "-m", "child")

	rows, err := client.Log(ctx, "@ | @-", 0)
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	child, base := rows[0].Revision.ChangeID, rows[1].Revision.ChangeID

	opts := jj.RebaseOptions{
		Revisions:   []string{child},
		Destination: []string{base},
		Scope:       jj.RebaseScopeRevisions,
	}

	if _, err := client.Rebase(ctx, opts); !errors.Is(err, jj.ErrNothingChanged) {
		t.Errorf("rebasing a revision onto its own parent returned %v, want ErrNothingChanged", err)
	}
}

func TestNewDuplicateAndAbandon(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)
	ctx := context.Background()

	if _, err := client.Describe(ctx, "@", "first"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	if _, err := client.New(ctx, []string{"@"}); err != nil {
		t.Fatalf("New: %v", err)
	}

	parent, err := client.Log(ctx, "@-", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	if rev := onlyRevision(t, parent); rev.Description != "first" {
		t.Fatalf("New did not land on top of @: parent describes %q, want %q", rev.Description, "first")
	}

	if _, err := client.Duplicate(ctx, []string{"@-"}); err != nil {
		t.Fatalf("Duplicate: %v", err)
	}

	all, err := client.Log(ctx, "all()", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	if got := countRevisionsDescribing(all, "first"); got != 2 {
		t.Fatalf("Duplicate left %d revisions describing %q, want 2", got, "first")
	}

	if _, err := client.Abandon(ctx, []string{"@-"}); err != nil {
		t.Fatalf("Abandon: %v", err)
	}

	rebasedParent, err := client.Log(ctx, "@-", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	if rev := onlyRevision(t, rebasedParent); rev.Description != "" {
		t.Fatalf("Abandon did not rebase @ onto the abandoned revision's parent: parent describes %q, want empty",
			rev.Description)
	}
}

func countRevisionsDescribing(rows []jj.LogRow, description string) int {
	n := 0

	for _, row := range rows {
		if row.Revision != nil && row.Revision.Description == description {
			n++
		}
	}

	return n
}

func onlyRevision(t *testing.T, rows []jj.LogRow) *jj.Revision {
	t.Helper()

	for _, row := range rows {
		if row.Revision != nil {
			return row.Revision
		}
	}

	t.Fatalf("no revision row among %d rows", len(rows))

	return nil
}

func TestClient_SerializesBehindTheQueueLock(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	client.Lock()

	done := make(chan struct{})
	go func() {
		defer close(done)

		if _, err := client.Root(context.Background()); err != nil {
			t.Errorf("Root: %v", err)
		}
	}()

	select {
	case <-done:
		t.Fatal("jj ran while the queue was held; an interactive verb must block every other call")
	case <-time.After(100 * time.Millisecond):
	}

	client.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("jj did not run after the queue was released")
	}
}
