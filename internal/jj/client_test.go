package jj_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyleking/aragonite/hermetic"

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
}

// The log leaves diff stats out, because they cost ten times the graph; Stats reads them for the
// commits named, keyed by the commit id the log reported.
func TestStats_ReadsTheSizeTheLogLeavesOut(t *testing.T) {
	t.Parallel()

	client, _ := seedRepo(t)
	ctx := context.Background()

	rows, err := client.Log(ctx, "::", 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	byDescription, _ := indexLogRows(t, rows)

	second := byDescription["second"]
	if second == nil || second.CommitID == "" || second.Files != 0 {
		t.Fatalf("the log row is %+v, want a commit id and no stats yet", second)
	}

	stats, err := client.Stats(ctx, []string{second.CommitID})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if got := stats[second.CommitID]; got.Files != 1 || got.Added != 1 {
		t.Errorf("expected 'second' to add 1 file/1 line, got %+v", got)
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

	client.AcquireQueue()

	done := make(chan struct{})
	go func() {
		defer close(done)

		if _, err := client.Log(context.Background(), "@", 1); err != nil {
			t.Errorf("Log: %v", err)
		}
	}()

	// A read that ignores the working copy writes nothing, so it does not wait for the queue.
	if _, err := client.Root(context.Background()); err != nil {
		t.Fatalf("Root while the queue was held: %v", err)
	}

	select {
	case <-done:
		t.Fatal("the snapshotting log read ran while the queue was held; an interactive verb must block it")
	case <-time.After(100 * time.Millisecond):
	}

	client.ReleaseQueue()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("jj did not run after the queue was released")
	}
}

// A diff editor set in jj's config is the user's choice, which jojo must see before naming its own.
func TestDiffEditorConfigured_SeesARepoLevelSetting(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	hermetic.RepoConfig(t, dir, "[ui]\ndiff-editor = \":builtin\"")

	set, err := client.DiffEditorConfigured(context.Background())
	if err != nil || !set {
		t.Errorf("DiffEditorConfigured = %v, %v, want true", set, err)
	}
}

// Only the log read snapshots the working copy. Every other read sees @ as the last log read left it
// and writes no operation, so moving the cursor or the dock cannot fill the op log with snapshots.
func TestReads_DoNotSnapshotTheWorkingCopy(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	if _, err := client.Log(ctx, "@", 1); err != nil {
		t.Fatalf("Log: %v", err)
	}

	before := len(jjOut(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", `id ++ "\n"`))

	writeFile(t, dir, "edited.txt", "edited\n")

	if text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: "@"}); err != nil || text != "" {
		t.Fatalf("GitDiff(@) before a log read = %q, %v; want the empty diff the last log read saw", text, err)
	}

	if _, err := client.OpLog(ctx, 5); err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	after := len(jjOut(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", `id ++ "\n"`))
	if after != before {
		t.Fatalf("reads wrote operations: the op log went from %d to %d bytes", before, after)
	}

	if _, err := client.Log(ctx, "@", 1); err != nil {
		t.Fatalf("Log: %v", err)
	}

	text, err := client.GitDiff(ctx, jj.DiffTarget{Revision: "@"})
	if err != nil || !strings.Contains(text, "edited.txt") {
		t.Fatalf("GitDiff(@) after a log read = %q, %v; want edited.txt", text, err)
	}
}

// jj undo snapshots before it reverses, so pressing u with an edit on disk would reverse the snapshot
// of that edit and delete it from disk. Undo refuses that, and leaves the file where it was.
func TestUndo_RefusesToDeleteAnEditOnDisk(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	runJJ(t, dir, "describe", "-m", "described")
	writeFile(t, dir, "edited.txt", "edited\n")

	if _, err := client.Undo(ctx); !errors.Is(err, jj.ErrUndoRefused) {
		t.Fatalf("Undo with an edit on disk = %v, want ErrUndoRefused", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "edited.txt")); err != nil {
		t.Fatalf("the edit is gone after a refused undo: %v", err)
	}

	if _, ok, err := client.HeadUndo(ctx); err != nil || ok {
		t.Fatalf("HeadUndo = %v, %v; want nothing offered over a snapshot", ok, err)
	}
}
