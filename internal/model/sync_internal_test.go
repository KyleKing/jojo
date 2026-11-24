package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/tuitest"
)

// syncRepos is liveRepo with main on bottom pushed to a bare remote, and a second clone of that
// remote, which plays the teammate whose push the sync fetches.
func syncRepos(t *testing.T) (string, string) {
	t.Helper()

	dir := liveRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")

	//nolint:gosec // G204: fixture helper runs the literal git binary on a temp path.
	if out, err := exec.CommandContext(context.Background(), "git", "init", "-q", "--bare", remote).
		CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	jjRun(t, dir, "git", "remote", "add", "origin", remote)
	jjRun(t, dir, "bookmark", "set", "main", "-r", changeFor(t, dir, "bottom"))
	jjRun(t, dir, "git", "push", "-b", "main")

	mate := filepath.Join(t.TempDir(), "mate")
	jjRun(t, filepath.Dir(mate), "git", "clone", remote, mate)

	return dir, mate
}

// teammatePushes lands a commit on main in the clone and pushes it.
func teammatePushes(t *testing.T, mate string) {
	t.Helper()

	jjRun(t, mate, "bookmark", "track", "main@origin")
	jjRun(t, mate, "new", "main")

	if err := os.WriteFile(filepath.Join(mate, "upstream.txt"), []byte("upstream\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	jjRun(t, mate, "commit", "-m", "upstream")
	jjRun(t, mate, "bookmark", "set", "main", "-r", "@-")
	jjRun(t, mate, "git", "push", "-b", "main")
}

// F fetches, reports what moved (main locally and on origin, and trunk), offers the restack, and y
// moves both stacks off the old trunk onto the new one in one operation.
func TestSyncLive_ReportsAndRestacks(t *testing.T) {
	t.Parallel()

	dir, mate := syncRepos(t)
	m := liveModel(t, dir, "top")

	teammatePushes(t, mate)

	m = press(t, m, "F")
	if !m.confirm.IsOpen() {
		t.Fatalf("F did not offer the restack; status %q, output open %v", m.message, m.output.IsOpen())
	}

	frame := tuitest.StripANSI(m.render())
	for _, want := range []string{"main@origin", "trunk() moved", "y restacks 2 stacks", "jj git fetch --all-remotes"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the report is missing %q:\n%s", want, frame)
		}
	}

	ops := opCount(t, dir)
	m = press(t, m, "y")

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("the restack took %d operations, want 1", got)
	}

	for _, stack := range []string{"middle", "side"} {
		parent := strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy",
			"-r", changeFor(t, dir, stack)+"-", "-T", "description.first_line()"))
		if parent != "upstream" {
			t.Errorf("after y %s sits on %q, want upstream", stack, parent)
		}
	}

	if m.synced.trunkMoved() {
		t.Error("the coach still offers the restack after it ran")
	}
}

// With nothing new on the remote the report says so and offers nothing.
func TestSyncLive_NothingMoved(t *testing.T) {
	t.Parallel()

	dir, _ := syncRepos(t)
	m := press(t, liveModel(t, dir, "top"), "F")

	if m.confirm.IsOpen() || !m.output.IsOpen() {
		t.Fatal("a sync that moved nothing offered a restack")
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, "fetched: nothing moved") {
		t.Errorf("the report does not say nothing moved:\n%s", frame)
	}
}

// gt puts the cursor on trunk(), and in a picker makes it the destination.
func TestTrunkLive_JumpsThereInTheLogAndThePicker(t *testing.T) {
	t.Parallel()

	dir, _ := syncRepos(t)
	m := press(t, liveModel(t, dir, "top"), "g", "t")

	if got := m.log.Selected(); got == nil || got.Description != "bottom" {
		t.Fatalf("gt put the cursor on %+v, want bottom", got)
	}

	m = liveModel(t, dir, "top")
	m = press(t, m, "r", "g", "t")

	if m.pick.dest != changeFor(t, dir, "bottom") {
		t.Errorf("gt in the rebase picker chose %q, want bottom", m.pick.dest)
	}
}

// The sync report at both sizes, with trunk moved under two stacks.
func TestSync_Frames(t *testing.T) {
	t.Parallel()

	report := syncReport{
		ran: "jj git fetch --all-remotes",
		moved: []string{
			"main  nptwozlo → qxlmrsvn", "main@origin  nptwozlo → qxlmrsvn", "parse@origin  new at twnsvwmr",
		},
		trunkFrom: "nptwozlo", trunkTo: "qxlmrsvn", stacks: []string{"koopqtrk", "zsrvmpwl"},
		revset: "@",
	}

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, size, "")
			r := report
			after := stackRows()
			after[2].Revision.Conflict = false
			after[3].Revision.Immutable = true
			r.rows = after

			m, _ = m.landSync(r)
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))

			assertVerbFrame(t, m.render(), size)
		})
	}
}
