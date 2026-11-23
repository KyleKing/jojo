package jj_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// stagingRepo is the shape the staging verbs expect: a mutable, described parent standing in for the
// index, and a working copy changing two files on top of it.
func stagingRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "staged.txt", "staged\n")
	writeFile(t, dir, "unstaged.txt", "unstaged\n")
	runJJ(t, dir, "describe", "-m", "work in progress")

	return client, dir
}

func filesIn(t *testing.T, client *jj.Client, rev string) []string {
	t.Helper()

	files, err := client.Files(context.Background(), rev)
	if err != nil {
		t.Fatalf("Files(%s): %v", rev, err)
	}

	paths := pathsOf(files)
	slices.Sort(paths)

	return paths
}

// logOf renders template for rev from stdout alone, as descriptionOf does for @.
func logOf(t *testing.T, dir, rev, template string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", "log", "--no-graph", "-r", rev, "-T", template)
	cmd.Dir = dir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("jj log -r %s: %v", rev, err)
	}

	return stdout.String()
}

// assertOnDisk is the invariant every staging verb keeps: the files in the working copy read the same
// afterwards, whichever revision now records them.
func assertOnDisk(t *testing.T, dir string, want map[string]string) {
	t.Helper()

	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: a fixture path the test wrote.
		if err != nil {
			t.Errorf("%s is gone from the working copy: %v", name, err)

			continue
		}

		if string(got) != content {
			t.Errorf("%s reads %q, want %q", name, got, content)
		}
	}
}

var stagingFiles = map[string]string{
	"base.txt": "base\n", "staged.txt": "staged\n", "unstaged.txt": "unstaged\n",
}

func TestStage_MovesOnlyTheNamedFileAndLeavesTheRestInTheWorkingCopy(t *testing.T) {
	t.Parallel()

	client, dir := stagingRepo(t)

	ran, err := client.Stage(context.Background(), "staged.txt")
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if want := "jj squash --from @ --into @- --use-destination-message --keep-emptied staged.txt"; ran != want {
		t.Errorf("Stage ran %q, want %q", ran, want)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"unstaged.txt"}) {
		t.Errorf("@ holds %v, want the unselected unstaged.txt to remain", got)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"base.txt", "staged.txt"}) {
		t.Errorf("@- holds %v, want base.txt and staged.txt", got)
	}

	assertOnDisk(t, dir, stagingFiles)
}

// Staging the last change in @ would let jj abandon it, taking its description and change id with
// it, so the working-copy change has to survive empty.
func TestStage_KeepsTheWorkingCopyChangeWhenEverythingIsStaged(t *testing.T) {
	t.Parallel()

	client, dir := stagingRepo(t)
	before := logOf(t, dir, "@", "change_id")

	if _, err := client.Stage(context.Background(), "staged.txt", "unstaged.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if after := logOf(t, dir, "@", "change_id"); after != before {
		t.Errorf("@ is now %s, want the same change %s kept", after, before)
	}

	if got := descriptionOf(t, dir); got != "work in progress\n" {
		t.Errorf("@ is described %q, want its description kept", got)
	}

	assertOnDisk(t, dir, stagingFiles)
}

func TestUnstage_ReturnsTheFileToTheWorkingCopy(t *testing.T) {
	t.Parallel()

	client, dir := stagingRepo(t)
	ctx := context.Background()

	if _, err := client.Stage(ctx, "staged.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if _, err := client.Unstage(ctx, "staged.txt"); err != nil {
		t.Fatalf("Unstage: %v", err)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"staged.txt", "unstaged.txt"}) {
		t.Errorf("@ holds %v, want both files back", got)
	}

	if got := filesIn(t, client, "@-"); !slices.Equal(got, []string{"base.txt"}) {
		t.Errorf("@- holds %v, want only base.txt", got)
	}

	assertOnDisk(t, dir, stagingFiles)
}

func TestCommitIndex_DescribesTheIndexAndStartsAnEmptyOne(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	runJJ(t, dir, "new", "--insert-before", "@", "--no-edit")
	writeFile(t, dir, "staged.txt", "staged\n")
	writeFile(t, dir, "unstaged.txt", "unstaged\n")

	if _, err := client.Stage(ctx, "staged.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	ran, err := client.CommitIndex(ctx, "feat: stage it")
	if err != nil {
		t.Fatalf("CommitIndex: %v (ran %s)", err, ran)
	}

	if got := logOf(t, dir, "@--", "description"); got != "feat: stage it\n" {
		t.Errorf("the committed revision is described %q, want the message", got)
	}

	if got := filesIn(t, client, "@--"); !slices.Equal(got, []string{"staged.txt"}) {
		t.Errorf("the committed revision holds %v, want staged.txt", got)
	}

	if got := logOf(t, dir, "@-", `if(empty, "empty", "changed")`); got != "empty" {
		t.Errorf("the new index is %s, want empty", got)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"unstaged.txt"}) {
		t.Errorf("@ holds %v, want the unstaged file left alone", got)
	}

	assertOnDisk(t, dir, stagingFiles)
}

func TestCommitIndex_RefusesWhenNothingIsStaged(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	runJJ(t, dir, "new", "--insert-before", "@", "--no-edit")
	writeFile(t, dir, "unstaged.txt", "unstaged\n")

	if _, err := client.CommitIndex(context.Background(), "nothing"); !errors.Is(err, jj.ErrNothingStaged) {
		t.Fatalf("CommitIndex() = %v, want ErrNothingStaged", err)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"unstaged.txt"}) {
		t.Errorf("@ holds %v after a refused commit, want it untouched", got)
	}
}

// Each refusal is a parent the staging verbs would have to rewrite past the working copy to use.
func TestStage_RefusesAParentThatCannotBeTheIndex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		build func(t *testing.T) (*jj.Client, string)
		name  string
	}{
		{
			name: "immutable parent",
			build: func(t *testing.T) (*jj.Client, string) {
				t.Helper()

				client, dir := testRepo(t)
				writeFile(t, dir, "unstaged.txt", "unstaged\n")

				return client, dir
			},
		},
		{
			name: "parent with another child",
			build: func(t *testing.T) (*jj.Client, string) {
				t.Helper()

				client, dir := stagingRepo(t)
				runJJ(t, dir, "new", "--no-edit", "@-", "-m", "sibling")

				return client, dir
			},
		},
		{
			name: "merge working copy",
			build: func(t *testing.T) (*jj.Client, string) {
				t.Helper()

				client, dir := testRepo(t)
				writeFile(t, dir, "a.txt", "a\n")
				runJJ(t, dir, "commit", "-m", "a")
				runJJ(t, dir, "new", "root()", "-m", "b")
				writeFile(t, dir, "b.txt", "b\n")
				runJJ(t, dir, "new", `description(substring:"a")`, "@", "-m", "merge")
				writeFile(t, dir, "unstaged.txt", "unstaged\n")

				return client, dir
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, dir := tc.build(t)
			before := filesIn(t, client, "@")

			if _, err := client.Stage(context.Background(), "unstaged.txt"); !errors.Is(err, jj.ErrNoIndex) {
				t.Fatalf("Stage() = %v, want ErrNoIndex", err)
			}

			if _, err := client.Unstage(context.Background(), "unstaged.txt"); !errors.Is(err, jj.ErrNoIndex) {
				t.Fatalf("Unstage() = %v, want ErrNoIndex", err)
			}

			if after := filesIn(t, client, "@"); !slices.Equal(after, before) {
				t.Errorf("@ holds %v after a refusal, want %v untouched", after, before)
			}

			assertOnDisk(t, dir, map[string]string{"unstaged.txt": "unstaged\n"})
		})
	}
}

// A path jj would read as fileset syntax has to reach it as the file it names, or staging it fails
// with a parse error about a file the list showed.
func TestStage_StagesAPathWithFilesetSyntaxInIt(t *testing.T) {
	t.Parallel()

	client, dir := stagingRepo(t)
	writeFile(t, dir, "sp ace(1).txt", "odd\n")

	if _, err := client.Stage(context.Background(), "sp ace(1).txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	if got := filesIn(t, client, "@-"); !slices.Contains(got, "sp ace(1).txt") {
		t.Errorf("@- holds %v, want the odd path staged", got)
	}

	if got := filesIn(t, client, "@"); !slices.Equal(got, []string{"staged.txt", "unstaged.txt"}) {
		t.Errorf("@ holds %v, want the other files left alone", got)
	}
}
