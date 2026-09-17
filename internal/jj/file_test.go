package jj_test

import (
	"context"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func pathsOf(files []jj.FileChange) []string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}

	return paths
}

func TestFiles(t *testing.T) {
	t.Parallel()

	client, _ := seedRepo(t)
	ctx := context.Background()

	t.Run("a revision's own changes carry jj's status letter", func(t *testing.T) {
		t.Parallel()

		files, err := client.Files(ctx, `description(substring:"third")`)
		if err != nil {
			t.Fatalf("Files: %v", err)
		}

		if len(files) != 1 || files[0].Path != "c.txt" || files[0].Status != "A" {
			t.Fatalf("Files() = %#v, want one added c.txt", files)
		}
	})

	t.Run("a conflicted merge lists the conflict its own diff omits", func(t *testing.T) {
		t.Parallel()

		files, err := client.Files(ctx, `description(substring:"merge-conflict")`)
		if err != nil {
			t.Fatalf("Files: %v", err)
		}

		if len(files) != 1 || files[0].Path != "b.txt" || !files[0].Conflict {
			t.Fatalf("Files() = %#v, want b.txt marked conflicted", files)
		}
	})
}

// A path with a space is the case both parsers get wrong by splitting on whitespace.
func TestFilesKeepsAPathWithASpaceIntact(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	writeFile(t, dir, "sp ace.txt", "spaced\n")

	files, err := client.Files(context.Background(), "@")
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if paths := pathsOf(files); len(paths) != 1 || paths[0] != "sp ace.txt" {
		t.Fatalf("Files() paths = %v, want the spaced path intact", paths)
	}
}

func TestRestoreFile(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "keep.txt", "one\n")
	runJJ(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "keep.txt", "two\n")
	writeFile(t, dir, "drop.txt", "gone\n")

	if _, err := client.RestoreFile(ctx, "@", "drop.txt"); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	files, err := client.Files(ctx, "@")
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if paths := pathsOf(files); len(paths) != 1 || paths[0] != "keep.txt" {
		t.Fatalf("Files() = %v, want only keep.txt still changed", paths)
	}
}

func TestFetchWithoutARemoteFails(t *testing.T) {
	t.Parallel()

	client, _ := testRepo(t)

	if _, err := client.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch() on a repository with no remote = nil, want an error naming the problem")
	}
}

// Squashing with a path moves that file and leaves the rest of the revision where it is, which is
// what makes the file-level move worth having beside the whole-revision one.
func TestSquashMovesOnlyTheNamedPath(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "moved.txt", "moved\n")
	writeFile(t, dir, "stays.txt", "stays\n")
	runJJ(t, dir, "describe", "-m", "source")

	if _, err := client.Squash(ctx, "@", "@-", "moved.txt"); err != nil {
		t.Fatalf("Squash: %v", err)
	}

	source, err := client.Files(ctx, "@")
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if paths := pathsOf(source); len(paths) != 1 || paths[0] != "stays.txt" {
		t.Errorf("the source still holds %v, want only stays.txt", paths)
	}

	dest, err := client.Files(ctx, "@-")
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	if paths := pathsOf(dest); len(paths) != 2 {
		t.Errorf("the destination holds %v, want base.txt and moved.txt", paths)
	}
}
