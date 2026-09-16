package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/evologview"
	"github.com/kyleking/jojo/internal/jj"
)

func openEvolog(t *testing.T) Model {
	t.Helper()

	opened, _ := testModel(t, twoRows()).applyEvolog(evologLoadedMsg{
		changeID: "aaaa",
		entries:  []jj.Evolution{{CommitID: "c0"}, {CommitID: "c1"}},
	})

	return asModel(t, opened)
}

// Enter asks for a version's diff rather than acting on the log underneath, which still has its own
// meaning for the same key.
func TestTimelineTakesKeysWhileOpen(t *testing.T) {
	t.Parallel()

	model := openEvolog(t)

	if !model.evolog.IsOpen() {
		t.Fatal("the timeline did not open")
	}

	next, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}

	show, ok := cmd().(evologview.ShowMsg)
	if !ok {
		t.Fatalf("enter produced %T, want a request to show a version", cmd())
	}

	if show.CommitID != "c0" {
		t.Errorf("showing %q, want the newest version", show.CommitID)
	}

	if _, cmd = asModel(t, next).Update(show); cmd == nil {
		t.Error("showing a version produced no command")
	}
}

// Restore puts the version back into the change the timeline belongs to, which is the only revision
// that version was ever part of.
func TestRestoreTargetsTheTimelinesChange(t *testing.T) {
	t.Parallel()

	model := openEvolog(t)

	next, cmd := model.restoreVersion(evologview.RestoreMsg{CommitID: "c1"})
	if cmd == nil {
		t.Fatal("restoring produced no command")
	}

	if asModel(t, next).evolog.IsOpen() {
		t.Error("the timeline stayed open after restoring")
	}
}

func TestTimelineFailureReportsRatherThanOpening(t *testing.T) {
	t.Parallel()

	next, _ := testModel(t, twoRows()).applyEvolog(evologLoadedMsg{err: jj.ErrCommandFailed})

	failed := asModel(t, next)
	if failed.evolog.IsOpen() {
		t.Error("a failed evolog still opened the timeline")
	}

	if failed.message == "" {
		t.Error("a failed evolog reported nothing")
	}
}
