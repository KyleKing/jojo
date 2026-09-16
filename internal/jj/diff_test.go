package jj_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyleking/jojo/internal/jj"
)

func TestDiffArgsRevision(t *testing.T) {
	t.Parallel()

	client := jj.NewClient(t.TempDir())

	args, err := client.DiffArgs(jj.DiffTarget{Revision: "abc"})
	if err != nil {
		t.Fatalf("DiffArgs: %v", err)
	}

	if want := []string{"diff", "-r", "abc"}; !equalArgs(args, want) {
		t.Errorf("DiffArgs = %v, want %v", args, want)
	}
}

func TestDiffArgsFromTo(t *testing.T) {
	t.Parallel()

	client := jj.NewClient(t.TempDir())

	args, err := client.DiffArgs(jj.DiffTarget{From: "a", To: "b"})
	if err != nil {
		t.Fatalf("DiffArgs: %v", err)
	}

	if want := []string{"diff", "--from", "a", "--to", "b"}; !equalArgs(args, want) {
		t.Errorf("DiffArgs = %v, want %v", args, want)
	}
}

func TestDiffArgsOperation(t *testing.T) {
	t.Parallel()

	client := jj.NewClient(t.TempDir())

	args, err := client.DiffArgs(jj.DiffTarget{Operation: "op123"})
	if err != nil {
		t.Fatalf("DiffArgs: %v", err)
	}

	if want := []string{"op", "show", "op123"}; !equalArgs(args, want) {
		t.Errorf("DiffArgs = %v, want %v", args, want)
	}
}

func TestDiffArgsRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	client := jj.NewClient(t.TempDir())

	cases := map[string]jj.DiffTarget{
		"empty":                  {},
		"revision and operation": {Revision: "a", Operation: "op1"},
		"from without to":        {From: "a"},
		"to without from":        {To: "b"},
		"all three forms":        {Revision: "a", From: "b", To: "c"},
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.DiffArgs(target); !errors.Is(err, jj.ErrInvalidDiffTarget) {
				t.Errorf("DiffArgs(%+v) = %v, want ErrInvalidDiffTarget", target, err)
			}
		})
	}
}

func equalArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}

// baseRepo builds a repo with one committed file, so a stress fixture diffs a known working-copy
// change against a known parent instead of jj's initial empty root.
func baseRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)
	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")

	return client, dir
}

func workingCopyDiff(t *testing.T, client *jj.Client) string {
	t.Helper()

	text, err := client.DiffText(context.Background(), jj.DiffTarget{Revision: "@"})
	if err != nil {
		t.Fatalf("DiffText: %v", err)
	}

	return text
}

type diffStressCase struct {
	name  string
	setup func(t *testing.T, dir string)
	want  []string
}

// diffStressCases each write into the working copy on top of baseRepo. Jj snapshots the working
// directory itself, so setup only needs to leave files in the shape a diff tool has to survive.
func diffStressCases() []diffStressCase {
	return []diffStressCase{
		{
			name: "binary file",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "bin.dat"),
					[]byte{0x00, 0x01, 0xff, 0xfe, 0x00}, fixtureFileMode); err != nil {
					t.Fatalf("write bin.dat: %v", err)
				}
			},
			want: []string{"(binary)"},
		},
		{
			name: "CRLF line endings",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, "crlf.txt", "line1\r\nline2\r\n")
			},
			want: []string{"line1", "line2"},
		},
		{
			name: "path with spaces and non-ASCII",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				sub := filepath.Join(dir, "dir with space")
				if err := os.Mkdir(sub, fixtureDirMode); err != nil {
					t.Fatalf("mkdir: %v", err)
				}

				if err := os.WriteFile(filepath.Join(sub, "nämé étrange.txt"),
					[]byte("hello\n"), fixtureFileMode); err != nil {
					t.Fatalf("write unicode path: %v", err)
				}
			},
			want: []string{"dir with space/nämé étrange.txt"},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.Symlink("base.txt", filepath.Join(dir, "link.txt")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			},
			want: []string{"Added symlink link.txt"},
		},
		{
			name: "empty file added",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, "empty_add.txt", "")
			},
			want: []string{"Added regular file empty_add.txt", "(empty)"},
		},
		{
			name: "no trailing newline",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, dir, "no_trailing_nl.txt", "no newline at end")
			},
			want: []string{"no newline at end"},
		},
	}
}

func TestDiffTextStressShapes(t *testing.T) {
	t.Parallel()

	for _, tc := range diffStressCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, dir := baseRepo(t)
			tc.setup(t, dir)
			text := workingCopyDiff(t, client)

			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("diff missing %q, got:\n%s", want, text)
				}
			}
		})
	}
}

func TestDiffTextRename(t *testing.T) {
	t.Parallel()

	client, dir := baseRepo(t)
	writeFile(t, dir, "renameme.txt", "content\n")
	runJJ(t, dir, "commit", "-m", "add renameme")

	if err := os.Rename(filepath.Join(dir, "renameme.txt"), filepath.Join(dir, "renamed.txt")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	text := workingCopyDiff(t, client)
	if !strings.Contains(text, "renameme.txt => renamed.txt") {
		t.Errorf("diff missing the rename marker, got:\n%s", text)
	}
}

func TestDiffTextEmptyFileDeleted(t *testing.T) {
	t.Parallel()

	client, dir := baseRepo(t)
	writeFile(t, dir, "empty_del.txt", "")
	runJJ(t, dir, "commit", "-m", "add empty file")

	if err := os.Remove(filepath.Join(dir, "empty_del.txt")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	text := workingCopyDiff(t, client)
	if !strings.Contains(text, "Removed regular file empty_del.txt") || !strings.Contains(text, "(empty)") {
		t.Errorf("diff missing the empty-file removal, got:\n%s", text)
	}
}

const (
	largeFileBytes  = 2_000_000
	largeFileBudget = 5 * time.Second
)

// TestDiffTextLargeSingleLineFile guards against a diff path that is quadratic in line length: jj
// itself has to snapshot and diff a file with no internal line breaks at all.
func TestDiffTextLargeSingleLineFile(t *testing.T) {
	if testing.Short() {
		t.Skip("large-file diff is slow; skip with -short")
	}

	t.Parallel()

	client, dir := baseRepo(t)
	allowLargeFiles(t, dir)
	writeFile(t, dir, "large.txt", strings.Repeat("x", largeFileBytes)+"\n")

	start := time.Now()
	text := workingCopyDiff(t, client)

	if elapsed := time.Since(start); elapsed > largeFileBudget {
		t.Errorf("diffing a %d-byte single-line file took %s, want under %s", largeFileBytes, elapsed, largeFileBudget)
	}

	if !strings.Contains(text, "Added regular file large.txt") {
		t.Errorf("diff missing the large file entry (%d bytes returned)", len(text))
	}
}

const manyFilesCount = 300

// TestDiffTextManyChangedFiles guards the argument list and captured output against a revision wide
// enough to matter, since DiffText builds one flag set for the whole diff rather than per file.
func TestDiffTextManyChangedFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("many-file diff is slow; skip with -short")
	}

	t.Parallel()

	client, dir := baseRepo(t)
	for i := range manyFilesCount {
		writeFile(t, dir, fmt.Sprintf("file_%d.txt", i), fmt.Sprintf("content %d\n", i))
	}

	text := workingCopyDiff(t, client)
	if got := strings.Count(text, "Added regular file"); got != manyFilesCount {
		t.Errorf("diff reports %d added files, want %d", got, manyFilesCount)
	}
}

// TestDiffTextConflictedRevision covers seedRepo's merge-conflict revision. Jj resolves it as an
// automatic merge of its two parents with no changes of its own, so its own diff is empty; the
// from/to form is what actually surfaces the conflicting content on each side.
func TestDiffTextConflictedRevision(t *testing.T) {
	t.Parallel()

	client, _ := seedRepo(t)
	ctx := context.Background()

	own, err := client.DiffText(ctx, jj.DiffTarget{Revision: "@"})
	if err != nil {
		t.Fatalf("DiffText on the conflict revision: %v", err)
	}

	if own != "" {
		t.Errorf("diff of the unmodified merge-conflict revision = %q, want empty", own)
	}

	sides, err := client.DiffText(ctx, jj.DiffTarget{From: "side-a", To: "side-b"})
	if err != nil {
		t.Fatalf("DiffText from side-a to side-b: %v", err)
	}

	if !strings.Contains(sides, "conflict-a-content") || !strings.Contains(sides, "conflict-b-content") {
		t.Errorf("diff between the conflicting sides missing their content, got:\n%s", sides)
	}
}

// TestDiffTextMergeRevision builds a merge with a real content-bearing parent on each side and a
// further edit made in the merge commit itself, since diff -r on a merge has to combine both parents.
func TestDiffTextMergeRevision(t *testing.T) {
	t.Parallel()

	client, dir := baseRepo(t)
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "base")

	writeFile(t, dir, "left.txt", "left\n")
	runJJ(t, dir, "commit", "-m", "left")
	runJJ(t, dir, "bookmark", "create", "-r", "@-", "leftrev")

	runJJ(t, dir, "new", "base", "-m", "right")
	writeFile(t, dir, "right.txt", "right\n")
	runJJ(t, dir, "bookmark", "create", "-r", "@", "rightrev")

	runJJ(t, dir, "new", "leftrev", "rightrev", "-m", "merge")
	writeFile(t, dir, "extra.txt", "extra\n")

	text := workingCopyDiff(t, client)
	if !strings.Contains(text, "Added regular file extra.txt") {
		t.Errorf("diff of the merge revision missing its own added file, got:\n%s", text)
	}
}

func TestDiffTextOperation(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := t.Context()

	writeFile(t, dir, "a.txt", "one\n")
	runJJ(t, dir, "describe", "-m", "first wording")

	ops, err := client.OpLog(ctx, 1)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	if len(ops) == 0 {
		t.Fatal("expected at least one operation")
	}

	text, err := client.DiffText(ctx, jj.DiffTarget{Operation: ops[0].ID})
	if err != nil {
		t.Fatalf("DiffText operation form: %v", err)
	}

	if !strings.Contains(text, "Changed commits") {
		t.Errorf("operation diff missing its summary, got:\n%s", text)
	}
}
