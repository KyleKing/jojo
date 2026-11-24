package jj_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/kyleking/aragonite/hermetic"

	"github.com/kyleking/jojo/internal/jj"
)

// fingerprint is every file under the repository's .jj and .git with its content hash, plus the op
// heads and refs jj and git report. Two equal fingerprints mean nothing was written, not merely that
// nothing visible changed.
func fingerprint(t *testing.T, dir string) string {
	t.Helper()

	var lines []string

	for _, sub := range []string{".jj", ".git"} {
		root := filepath.Join(dir, sub)
		if _, err := os.Stat(root); err != nil {
			continue
		}

		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}

			data, err := os.ReadFile(path) //nolint:gosec // G304: a file inside this test's own repository.
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}

			sum := sha256.Sum256(data)
			lines = append(lines, path+" "+hex.EncodeToString(sum[:]))

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	sort.Strings(lines)

	return strings.Join(lines, "\n")
}

func changeOf(t *testing.T, dir, revset string) string {
	t.Helper()

	return strings.TrimSpace(jjOut(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r", revset,
		"-T", "change_id.short(8)"))
}

func jjOut(t *testing.T, dir string, args ...string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jj %v: %v", args, err)
	}

	return string(out)
}

func openSandbox(t *testing.T, client *jj.Client) *jj.Sandbox {
	t.Helper()

	sandbox, err := client.OpenSandbox(t.Context())
	if err != nil {
		t.Fatalf("OpenSandbox: %v", err)
	}

	t.Cleanup(func() {
		if err := sandbox.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	return sandbox
}

func revisionOf(rows []jj.LogRow, id string) *jj.Revision {
	for _, row := range rows {
		if row.Revision != nil && row.Revision.ChangeID == id {
			return row.Revision
		}
	}

	return nil
}

// A preview is the whole point of the sandbox: the log it returns shows the write, and the
// repository it came from has not had a single byte written to it, op heads and git refs included.
func TestSandbox_PreviewLeavesNoTraceInTheRepository(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	first, third := changeOf(t, dir, "description(exact:'first\n')"), changeOf(t, dir, "description(exact:'third\n')")

	// The sandbox snapshots the working copy, so the fingerprint is taken after that has happened.
	sandbox := openSandbox(t, client)
	before := fingerprint(t, dir)
	opsBefore := jjOut(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", "id ++ \"\\n\"")

	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{first}})

	preview, err := sandbox.Preview(t.Context(), args, "all()", 0)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	moved := revisionOf(preview.Rows, third)
	if moved == nil {
		t.Fatalf("the preview lost the rebased revision %s", third)
	}

	parents := strings.TrimSpace(jjOut(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r", third+"-",
		"-T", "change_id.short(8)"))
	if parents == first {
		t.Fatal("the rebase reached the real repository")
	}

	if after := fingerprint(t, dir); after != before {
		t.Error("previewing a rebase wrote to the repository's .jj or .git")
	}

	opsAfter := jjOut(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", "id ++ \"\\n\"")
	if opsAfter != opsBefore {
		t.Error("previewing a rebase added an operation to the repository's op log")
	}
}

// Holding j sends a preview per row. However many land, and in whatever order, each starts from the
// state the sandbox was copied at, and none of them reaches the repository.
func TestSandbox_RapidPreviewsEachStartFromTheCopy(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	sandbox := openSandbox(t, client)
	before := fingerprint(t, dir)

	third := changeOf(t, dir, "description(exact:'third\n')")
	destinations := strings.Fields(jjOut(t, dir, "log", "--no-graph", "--ignore-working-copy",
		"-r", "::main ~ root()", "-T", "change_id.short(8) ++ \" \""))

	var wg sync.WaitGroup

	for range 3 {
		for _, dest := range destinations {
			wg.Go(func() {
				args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{dest}})
				if _, err := sandbox.Preview(context.Background(), args, "all()", 0); err != nil &&
					!errors.Is(err, jj.ErrNothingChanged) {
					t.Errorf("Preview onto %s: %v", dest, err)
				}
			})
		}
	}

	wg.Wait()

	// The last preview of a rebase onto the root shows the change there, which it would not if an
	// earlier preview had been left in place and the rebase had nothing to do.
	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{"root()"}})

	preview, err := sandbox.Preview(t.Context(), args, "all()", 0)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if preview.Unchanged {
		t.Error("an earlier preview survived into the next one")
	}

	if after := fingerprint(t, dir); after != before {
		t.Error("a burst of previews wrote to the repository")
	}
}

// A write jj refuses is reported as the refusal, with the repository still untouched, so the picker
// can say why Enter would fail before anyone presses it.
func TestSandbox_RefusedWriteReportsTheRefusal(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	sandbox := openSandbox(t, client)
	before := fingerprint(t, dir)

	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{"root()"}, Destination: []string{"@"}})

	if _, err := sandbox.Preview(t.Context(), args, "all()", 0); err == nil {
		t.Fatal("rebasing the root previewed without complaint")
	}

	if after := fingerprint(t, dir); after != before {
		t.Error("a refused preview wrote to the repository")
	}
}

// Rebasing a subtree onto the parent it already has is jj's "nothing changed", which the preview
// reports rather than drawing the unchanged log as though something had moved.
func TestSandbox_NoOpPreviewSaysSo(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	sandbox := openSandbox(t, client)

	second := changeOf(t, dir, "main")
	third := changeOf(t, dir, "description(exact:'third\n')")
	args := jj.RebaseArgs(jj.RebaseOptions{
		Revisions: []string{third}, Destination: []string{second}, Scope: jj.RebaseScopeSource,
	})

	preview, err := sandbox.Preview(t.Context(), args, "all()", 0)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !preview.Unchanged {
		t.Error("a rebase onto the current parent did not report that nothing changed")
	}
}

// Close removes everything the sandbox wrote, and a preview that arrives after it fails instead of
// recreating any of it.
func TestSandbox_CloseRemovesItAndRefusesLaterPreviews(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)

	sandbox, err := client.OpenSandbox(t.Context())
	if err != nil {
		t.Fatalf("OpenSandbox: %v", err)
	}

	if err := sandbox.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(sandbox.Dir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the sandbox directory survived Close: %v", err)
	}

	third := changeOf(t, dir, "description(exact:'third\n')")
	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{"root()"}})

	if _, err := sandbox.Preview(t.Context(), args, "all()", 0); err == nil {
		t.Error("a preview after Close succeeded")
	}
}

// The command a preview drew is the command Enter runs, and it lands as exactly one operation, so a
// single u takes it back.
func TestApply_RunsThePreviewedCommandAsOneOperation(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	first, third := changeOf(t, dir, "description(exact:'first\n')"), changeOf(t, dir, "description(exact:'third\n')")

	sandbox := openSandbox(t, client)
	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{first}})

	if _, err := sandbox.Preview(t.Context(), args, "all()", 0); err != nil {
		t.Fatalf("Preview: %v", err)
	}

	countOps := func() int {
		ids := jjOut(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", "id ++ \" \"")

		return len(strings.Fields(ids))
	}

	before := countOps()

	if _, err := client.Apply(t.Context(), args); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := countOps() - before; got != 1 {
		t.Errorf("applying the previewed rebase added %d operations, want 1", got)
	}

	if parent := changeOf(t, dir, third+"-"); parent != first {
		t.Errorf("after apply %s sits on %s, want %s", third, parent, first)
	}
}

// The sandbox carries the repository's own settings, so a revset alias defined for this repository
// alone resolves in the preview the way it does in the log.
func TestSandbox_CarriesRepositoryConfig(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)
	hermetic.RepoConfig(t, dir, "[revset-aliases]\nfirstrev = \"description(exact:'first\\n')\"")

	sandbox := openSandbox(t, client)
	third := changeOf(t, dir, "description(exact:'third\n')")
	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{third}, Destination: []string{"firstrev"}})

	preview, err := sandbox.Preview(t.Context(), args, "firstrev::", 0)
	if err != nil {
		t.Fatalf("Preview with a repo alias: %v", err)
	}

	if revisionOf(preview.Rows, third) == nil {
		t.Error("the preview of a rebase onto a repo alias did not show the revision under it")
	}
}

// The past is read without writing the present: an operation's log comes back as it was, and the op
// log is no longer for having been looked at.
func TestLogAt_ReadsThePastWithoutWriting(t *testing.T) {
	t.Parallel()

	client, dir := seedRepo(t)

	ops, err := client.OpLog(t.Context(), 0)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	before := len(ops)
	runJJ(t, dir, "abandon", "description(exact:'third\n')")

	rows, err := client.LogAt(t.Context(), ops[0].ID, "all()", 0)
	if err != nil {
		t.Fatalf("LogAt: %v", err)
	}

	found := false

	for _, row := range rows {
		if row.Revision != nil && row.Revision.Description == "third" {
			found = true
		}
	}

	if !found {
		t.Error("the log at the earlier operation lost a revision abandoned since")
	}

	after, err := client.OpLog(t.Context(), 0)
	if err != nil {
		t.Fatalf("OpLog: %v", err)
	}

	if len(after) != before+1 {
		t.Errorf("op log grew by %d, want 1 (the abandon alone)", len(after)-before)
	}

	if after[1].Args == "" || !strings.Contains(after[0].Args, "abandon") {
		t.Errorf("the op log lost the command lines: %q, %q", after[0].Args, after[1].Args)
	}
}

// jj keys a repository's own settings by a config-id under the user's jj config directory, and a
// copied id is answered by copying that whole directory under a new one. The sandbox must not, or
// every picker session would leave a directory in the user's config.
func TestSandbox_WritesNothingIntoTheUserConfig(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("JJ_CONFIG", "")

	client, dir := seedRepo(t)
	hermetic.RepoConfig(t, dir, "[revset-aliases]\nmine2 = \"@\"")

	list := func() string {
		entries, _ := os.ReadDir(filepath.Join(config, "jj", "repos")) //nolint:errcheck // absent reads as empty.

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		return strings.Join(names, ",")
	}

	before := list()
	sandbox := openSandbox(t, client)

	args := jj.RebaseArgs(jj.RebaseOptions{Revisions: []string{"@"}, Destination: []string{"root()"}})
	if _, err := sandbox.Preview(t.Context(), args, "mine2", 0); err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if after := list(); after != before {
		t.Errorf("the sandbox added to the user's jj config: %q, was %q", after, before)
	}
}
