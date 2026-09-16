package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/jj"
)

func openFiles(t *testing.T, files []jj.FileChange) Model {
	t.Helper()

	model, _ := deliver(testModel(t, twoRows()), files, nil, showFiles("abc"))

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

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c produced no command")
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

	if _, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Text: "c"}); cmd != nil {
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
