package jj_test

import (
	"context"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func findBookmark(marks []jj.Bookmark, ref string) *jj.Bookmark {
	for i := range marks {
		if marks[i].Ref() == ref {
			return &marks[i]
		}
	}

	return nil
}

func TestBookmarkLifecycle(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")
	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "describe", "-m", "second")

	if _, err := client.BookmarkSet(ctx, "feature", "@-"); err != nil {
		t.Fatalf("BookmarkSet: %v", err)
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	created := findBookmark(marks, "feature")
	if created == nil {
		t.Fatal("the new bookmark is not listed")
	}

	if created.Remote != "" || created.ChangeID == "" || created.Description != "first" {
		t.Errorf("bookmark reads %+v, want a local bookmark on the first change", *created)
	}

	if _, err := client.BookmarkSet(ctx, "feature", "@"); err != nil {
		t.Fatalf("BookmarkSet onto @: %v", err)
	}

	if marks, err = client.Bookmarks(ctx); err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if moved := findBookmark(marks, "feature"); moved == nil || moved.Description != "second" {
		t.Errorf("bookmark did not move onto @: %+v", moved)
	}

	if _, err := client.BookmarkRename(ctx, "feature", "renamed"); err != nil {
		t.Fatalf("BookmarkRename: %v", err)
	}

	if _, err := client.BookmarkDelete(ctx, "renamed"); err != nil {
		t.Fatalf("BookmarkDelete: %v", err)
	}

	if marks, err = client.Bookmarks(ctx); err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if findBookmark(marks, "renamed") != nil {
		t.Error("the deleted bookmark is still listed")
	}
}

// Forget drops jj's record of a bookmark outright, unlike Delete, which keeps a tombstone to
// propagate on the next push.
func TestBookmarkForgetDropsItWithNoTombstone(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")

	if _, err := client.BookmarkSet(ctx, "temp", "@"); err != nil {
		t.Fatalf("BookmarkSet: %v", err)
	}

	if _, err := client.BookmarkForget(ctx, "temp"); err != nil {
		t.Fatalf("BookmarkForget: %v", err)
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if findBookmark(marks, "temp") != nil {
		t.Error("the forgotten bookmark is still listed")
	}
}

// A colocated repository reports its own git refs as a "git" remote, which cannot be pushed to or
// tracked, so the panel must never offer it as a target.
func TestBookmarksDropsTheColocatedGitRemote(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "only change")
	runJJ(t, dir, "bookmark", "create", "-r", "@", "main")
	runJJ(t, dir, "git", "export")

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	for _, mark := range marks {
		if mark.Remote == "git" {
			t.Errorf("the colocated git remote is listed: %+v", mark)
		}
	}
}

func TestBookmarkAdvanceMovesTheClosestBookmark(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "commit", "-m", "first")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "main")
	writeFile(t, dir, "b.txt", "b\n")
	runJJ(t, dir, "describe", "-m", "second")

	if _, err := client.BookmarkAdvance(ctx, []string{"main"}, "@"); err != nil {
		t.Fatalf("BookmarkAdvance: %v", err)
	}

	marks, err := client.Bookmarks(ctx)
	if err != nil {
		t.Fatalf("Bookmarks: %v", err)
	}

	if advanced := findBookmark(marks, "main"); advanced == nil || advanced.Description != "second" {
		t.Errorf("main did not advance: %+v", advanced)
	}
}
