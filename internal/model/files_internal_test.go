package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

func openFiles(t *testing.T, files []jj.FileChange) Model {
	t.Helper()

	model, _ := deliver(testModel(t, twoRows()), files, nil, showFiles(jj.Revision{ChangeID: "abc"}))

	return model
}

// The cursor lands on the conflict because it is the only row that blocks the rest of the work, and
// a merge's conflicts sit below the files its own diff reports.
func TestFilePanelStartsOnTheConflict(t *testing.T) {
	t.Parallel()

	model := openFiles(t, []jj.FileChange{
		{Status: "M", Path: "a.txt"},
		{Path: "b.txt", Conflict: true},
	})

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd == nil {
		t.Fatal("x produced no command")
	}

	resolve, ok := cmd().(fileview.ResolveMsg)
	if !ok {
		t.Fatalf("c produced %T, want a resolve", cmd())
	}

	if resolve.Path != "b.txt" {
		t.Errorf("resolving %q, want the conflicted file", resolve.Path)
	}

	if closed, _ := asModel(t, next).Update(resolve); asModel(t, closed).files.IsOpen() {
		t.Error("the panel stayed open while the merge tool holds the terminal")
	}
}

// Resolving a file that carries no conflict would hand the terminal to a merge tool with nothing to
// merge, so the panel says so and stays put.
func TestFilePanelRefusesToResolveACleanFile(t *testing.T) {
	t.Parallel()

	model := openFiles(t, []jj.FileChange{{Status: "M", Path: "a.txt"}})

	if _, cmd := model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
		t.Fatalf("c on a clean file produced %T, want nothing", cmd())
	}
}

// A revision that changes nothing has no panel worth opening, and an empty frame says less than the
// status line does.
func TestFilePanelStaysClosedWhenTheRevisionTouchesNothing(t *testing.T) {
	t.Parallel()

	model := openFiles(t, nil)

	if model.files.IsOpen() {
		t.Error("the panel opened over an empty file list")
	}

	if model.message == "" {
		t.Error("an empty file list reported nothing")
	}
}

func TestResolveKeyRefusesACleanRevision(t *testing.T) {
	t.Parallel()

	cmd := testModel(t, twoRows()).resolveKey(jj.Revision{ChangeID: "abc"})
	if cmd == nil {
		t.Fatal("c on a clean revision produced no command")
	}

	if _, ok := cmd().(messageMsg); !ok {
		t.Fatalf("c on a clean revision produced %T, want a message", cmd())
	}
}

// A revision carrying no bookmark has nothing to push, and a confirmation over a no-op teaches the
// wrong thing about what the gate is for.
func TestPushSaysNothingToPushRatherThanAsking(t *testing.T) {
	t.Parallel()

	preview := pushPreviewMsg{what: "abc", notice: "Nothing changed.\n"}

	next, _, handled := testModel(t, twoRows()).offerPush(preview)
	if !handled {
		t.Fatal("the push preview was not handled")
	}

	model := asModel(t, next)
	if model.confirm.IsOpen() {
		t.Error("a push that would do nothing opened the confirmation")
	}

	if model.message == "" {
		t.Error("a push that would do nothing reported nothing")
	}
}

// Moving one file has to name that file in the squash, or it moves the whole revision.
func TestMoveFileSquashesOnlyThatPath(t *testing.T) {
	t.Parallel()

	model := openFiles(t, []jj.FileChange{{Status: "M", Path: "a.txt"}})

	next, cmd := model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("s produced no command")
	}

	move, ok := cmd().(fileview.MoveMsg)
	if !ok {
		t.Fatalf("m produced %T, want a move", cmd())
	}

	picking := asModel(t, next)
	picking = asModel(t, update(picking, move))

	if !picking.destPicker.IsOpen() {
		t.Fatal("moving a file did not ask for a destination")
	}

	if picking.files.IsOpen() {
		t.Error("the file list stayed open under the destination picker")
	}
}

func openWorkingCopyFiles(t *testing.T, files []jj.FileChange) Model {
	t.Helper()

	rev := jj.Revision{ChangeID: "aaaa", Description: "wip", WorkingCopy: true}
	model, _ := deliver(testModel(t, twoRows()), files, nil, showFiles(rev))

	return model
}

// l on the working copy opens the same list as anywhere else, with c added, and nothing staged.
func TestFilesKey_OnTheWorkingCopyOffersCommit(t *testing.T) {
	t.Parallel()

	model := openWorkingCopyFiles(t, []jj.FileChange{{Status: "M", Path: "a.txt"}})

	if !model.files.IsOpen() {
		t.Fatal("the working copy's file list did not open")
	}

	view := tuitest.StripANSI(model.files.View(100, 12))
	for _, want := range []string{"a.txt", "check", "[c]"} {
		if !strings.Contains(view, want) {
			t.Errorf("the list is missing %q:\n%s", want, view)
		}
	}

	if strings.Contains(view, "stage") {
		t.Errorf("the list still speaks of staging:\n%s", view)
	}
}

// The prompt sits under the file list in the stacking order, so the list has to close for the
// message to be typed where it can be seen.
func TestCommit_ClosesTheListAndAsksForTheMessage(t *testing.T) {
	t.Parallel()

	model := openWorkingCopyFiles(t, []jj.FileChange{{Status: "M", Path: "a.txt"}})

	next, cmd := model.Update(tuitest.Key("c"))
	if cmd == nil {
		t.Fatal("c produced no command")
	}

	commit, ok := cmd().(fileview.CommitMsg)
	if !ok {
		t.Fatalf("c produced %T, want a commit", cmd())
	}

	asking := asModel(t, update(asModel(t, next), commit))

	if asking.files.IsOpen() {
		t.Error("the file list stayed open over the commit prompt")
	}

	if !asking.prompt.IsOpen() || asking.onSubmit == nil {
		t.Fatal("committing did not ask for a message")
	}
}
