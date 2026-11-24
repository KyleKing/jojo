package model

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

// bookmarkAt is the first line of the description of the revision name points at, "" when jj
// cannot resolve it to one (missing, or conflicted).
func bookmarkAt(t *testing.T, dir, name string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", "log", "--no-graph", "--ignore-working-copy",
		"-r", name, "-T", "description.first_line()")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// bookmarkRepo is liveRepo with each named bookmark set on the revision described, and the model
// over it with the cursor on cursorOn.
func bookmarkRepo(t *testing.T, cursorOn string, marks ...[2]string) (string, Model) {
	t.Helper()

	dir := liveRepo(t)
	for _, mark := range marks {
		jjRun(t, dir, "bookmark", "set", mark[0], "-r", changeFor(t, dir, mark[1]), "--allow-backwards")
	}

	return dir, liveModel(t, dir, cursorOn)
}

// b s with one other bookmark in the repository goes straight to the picker over it, previews the
// move in the sandbox, and enter moves it as one operation.
func TestBookmarkMenuLive_SetPreviewsThenMoves(t *testing.T) {
	t.Parallel()

	dir, m := bookmarkRepo(t, "top", [2]string{"feat", "bottom"})
	ops := opCount(t, dir)

	m = press(t, m, "b", "s")
	if !m.pick.active || m.pick.verb != pickBookmark || m.pick.bookmark != "feat" {
		t.Fatalf("b s opened %+v, want the bookmark picker over feat", m.pick.verb)
	}

	if m.pick.previewErr != nil || m.pick.shownFor != m.pick.command() {
		t.Fatalf("the preview did not land: %v", m.pick.previewErr)
	}

	if got := m.pick.command(); !strings.Contains(got, "jj bookmark set feat -r ") {
		t.Errorf("the footer's command is %q", got)
	}

	if opCount(t, dir) != ops {
		t.Fatal("previewing the move wrote to the repository")
	}

	m = press(t, m, "enter")

	if got := bookmarkAt(t, dir, "feat"); got != "top" {
		t.Errorf("feat is on %q, want top", got)
	}

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("the move took %d operations, want 1", got)
	}

	if !strings.Contains(m.message, "jj bookmark set feat") {
		t.Errorf("the status line says %q, want the command echoed", m.message)
	}
}

// jj refuses a backwards move; the preview says so and names -B, enter says why rather than failing,
// and -B makes the same move go through.
func TestBookmarkMenuLive_BackwardsNeedsTheFlag(t *testing.T) {
	t.Parallel()

	dir, m := bookmarkRepo(t, "middle", [2]string{"feat", "top"})

	m = press(t, m, "b", "s")
	if !jj.IsBackwards(m.pick.previewErr) {
		t.Fatalf("moving feat back to middle previewed %v, want jj's refusal", m.pick.previewErr)
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, "-B allows it") {
		t.Errorf("the footer does not name -B:\n%s", frame)
	}

	next, cmd := m.Update(tuitest.Key("enter"))
	m = asModel(t, next)

	if got := said(flatten(cmd)); !strings.Contains(got, "backwards or sideways") || !m.pick.active {
		t.Fatalf("enter on a refused move said %q (picker open %v)", got, m.pick.active)
	}

	m = press(t, m, "-", "B")
	if !m.pick.backwards || m.pick.previewErr != nil {
		t.Fatalf("-B did not clear the refusal: %v", m.pick.previewErr)
	}

	press(t, m, "enter")

	if got := bookmarkAt(t, dir, "feat"); got != "middle" {
		t.Errorf("feat is on %q, want middle", got)
	}
}

// b a tugs the nearest bookmark below the cursor up to it.
func TestBookmarkMenuLive_AdvanceTugsTheBookmarkBelow(t *testing.T) {
	t.Parallel()

	dir, m := bookmarkRepo(t, "top", [2]string{"feat", "bottom"})

	press(t, m, "b", "a")

	if got := bookmarkAt(t, dir, "feat"); got != "top" {
		t.Errorf("after b a feat is on %q, want top", got)
	}
}

// b r asks for the new name; b d and b f each drop the bookmark, and with two on the revision the
// verb asks which.
func TestBookmarkMenuLive_RenameDeleteForget(t *testing.T) {
	t.Parallel()

	dir, m := bookmarkRepo(t, "middle", [2]string{"feat", "middle"})

	m = press(t, m, "b", "r")
	if !m.prompt.IsOpen() {
		t.Fatal("b r did not ask for the new name")
	}

	if got := m.prompt.Value(); got != "feat" {
		t.Errorf("the rename prompt opened on %q, want the old name", got)
	}

	m = press(t, m, "2", "enter")
	if bookmarkAt(t, dir, "feat2") != "middle" || bookmarkAt(t, dir, "feat") != "" {
		t.Fatalf("rename left feat2 on %q and feat on %q", bookmarkAt(t, dir, "feat2"), bookmarkAt(t, dir, "feat"))
	}

	jjRun(t, dir, "bookmark", "set", "other", "-r", changeFor(t, dir, "middle"))
	m = pump(t, m, m.loadLog("all()"))

	m = press(t, m, "b", "d")
	if !m.bookmarks.IsOpen() {
		t.Fatal("with two bookmarks b d did not ask which")
	}

	m = press(t, m, "enter")
	if bookmarkAt(t, dir, "feat2") != "" || bookmarkAt(t, dir, "other") != "middle" {
		t.Fatalf("b d deleted the wrong one: feat2 %q, other %q",
			bookmarkAt(t, dir, "feat2"), bookmarkAt(t, dir, "other"))
	}

	press(t, m, "b", "f")

	if bookmarkAt(t, dir, "other") != "" {
		t.Error("b f left the bookmark")
	}
}

// b t untracks a tracked remote bookmark and tracks it again.
func TestBookmarkMenuLive_TrackToggles(t *testing.T) {
	t.Parallel()

	dir, _ := bookmarkRepo(t, "middle", [2]string{"feat", "middle"})
	remote := filepath.Join(t.TempDir(), "remote.git")

	//nolint:gosec // G204: fixture helper runs the literal git binary on a temp path.
	git := exec.CommandContext(context.Background(), "git", "init", "-q", "--bare", remote)
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	jjRun(t, dir, "git", "remote", "add", "origin", remote)
	jjRun(t, dir, "git", "push", "-b", "feat")

	m := liveModel(t, dir, "middle")

	tracked := func() bool {
		return strings.Contains(jjRun(t, dir, "bookmark", "list", "--tracked", "--ignore-working-copy"), "@origin")
	}

	if !tracked() {
		t.Fatal("the fixture's push did not track feat@origin")
	}

	m = press(t, m, "b", "t")
	if tracked() {
		t.Fatalf("b t left feat@origin tracked: %q", m.message)
	}

	press(t, m, "b", "t")

	if !tracked() {
		t.Error("a second b t did not track it again")
	}
}

// A bookmark that moved both ways resolves with b x: the picker opens on it, and enter settles it
// where the cursor is.
func TestBookmarkMenuLive_ResolveConflict(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	// The bookmark has to exist before the two moves split, or the colocated git ref settles it.
	jjRun(t, dir, "bookmark", "set", "feat", "-r", changeFor(t, dir, "bottom"))
	jjRun(t, dir, "bookmark", "set", "feat", "-r", changeFor(t, dir, "middle"))
	jjRun(t, dir, "--at-op", "@-", "bookmark", "set", "feat", "-r", changeFor(t, dir, "side"))
	jjRun(t, dir, "log", "-r", "@")

	m := liveModel(t, dir, "middle")
	if !strings.Contains(fmt.Sprint(m.log.Selected().Bookmarks), "feat??") {
		t.Fatalf("the fixture's bookmark is not conflicted: %v", m.log.Selected().Bookmarks)
	}

	m = press(t, m, "b", "x")
	if !m.pick.active || m.pick.bookmark != "feat" {
		t.Fatal("b x did not open the picker on feat")
	}

	m = press(t, m, "enter")

	if got := bookmarkAt(t, dir, "feat"); got != "middle" {
		t.Errorf("feat resolved to %q, want middle", got)
	}

	if m.log.Selected() == nil || strings.Contains(fmt.Sprint(m.log.Selected().Bookmarks), "??") {
		t.Error("the log still draws feat conflicted")
	}
}

// Each verb that needs a bookmark on the revision says so rather than doing nothing.
func TestBookmarkMenu_GatesSayWhy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ key, want string }{
		{"r", "has no bookmark; b c creates one"},
		{"d", "has no bookmark"},
		{"f", "has no bookmark"},
		{"t", "has no remote bookmark"},
		{"x", "has no conflicted bookmark"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))
			m, _ = pressOne(t, m, "b")
			_, msgs := pressOne(t, m, tc.key)

			if got := said(msgs); !strings.Contains(got, tc.want) {
				t.Errorf("b %s said %q, want %q", tc.key, got, tc.want)
			}
		})
	}

	m := asModel(t, update(testModel(t, stackRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))
	m.log.Focus("twnsvwmr")
	m, _ = pressOne(t, m, "b")

	if _, msgs := pressOne(t, m, "a"); !strings.Contains(said(msgs), "holds parse already") {
		t.Errorf("b a on a bookmarked revision said %q", said(msgs))
	}
}
