package jj_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// repoWithRemote builds a repository with a bare git remote named origin, so a push has somewhere
// real to go without reaching the network.
func repoWithRemote(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")

	//nolint:gosec // G204: fixture helper runs the literal git binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "git", "init", "--bare", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	runJJ(t, dir, "git", "remote", "add", "origin", remote)

	return client, dir
}

func TestPushRefusesWithNothingNamed(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	if _, err := client.Push(context.Background(), jj.PushOptions{}); !errors.Is(err, jj.ErrNothingToPush) {
		t.Errorf("Push with no target returned %v, want ErrNothingToPush", err)
	}
}

func TestPushSendsOneBookmarkAndReportsTracking(t *testing.T) {
	t.Parallel()

	client, dir := repoWithRemote(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "feature")

	if _, err := client.Push(ctx, jj.PushOptions{Bookmarks: []string{"feature"}}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	pushed := findBookmark(marks, "feature@origin")
	if pushed == nil {
		t.Fatal("the pushed bookmark has no remote entry")
	}

	if !pushed.Tracked || pushed.Ahead != 0 || pushed.Behind != 0 {
		t.Errorf("remote bookmark reads %+v, want tracked and level", *pushed)
	}

	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "commit", "-m", "second")
	runJJ(t, dir, "bookmark", "set", "feature", "-r", "@-")

	if marks, err = client.Bookmarks(ctx); err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if ahead := findBookmark(marks, "feature@origin"); ahead == nil || ahead.Behind != 1 {
		t.Errorf("remote bookmark reads %+v, want one commit behind the local one", ahead)
	}
}

// A dry run is what the confirmation dialog shows before the real push, so it must not move the
// remote.
func TestPushDryRunLeavesTheRemoteAlone(t *testing.T) {
	t.Parallel()

	client, dir := repoWithRemote(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "feature")

	notice, err := client.Push(ctx, jj.PushOptions{Bookmarks: []string{"feature"}, DryRun: true})
	if err != nil {
		t.Fatalf("Push dry run: %v", err)
	}

	if notice == "" {
		t.Error("a dry run reported nothing about what it would push")
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if findBookmark(marks, "feature@origin") != nil {
		t.Error("a dry run created a remote bookmark")
	}
}
