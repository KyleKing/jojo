package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func ctrlR() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
}

// Redo is jj's own: ctrl+r runs it straight away rather than waiting for jojo to have recorded an
// undo, so an undo from the CLI or an earlier session redoes too.
func TestRedo_RunsWithoutAnUndoRecordedHere(t *testing.T) {
	t.Parallel()

	if _, cmd := testModel(t, twoRows()).Update(ctrlR()); cmd == nil {
		t.Fatal("ctrl+r produced no command")
	}
}

// The redo line used to run the operation id into the hint ("jj op restore 5a8834d6redone, press u").
func TestRedo_ReportsTheCommandAndTheHintApart(t *testing.T) {
	t.Parallel()

	msg, ok := wrote("jj redo", undoHint, nil).(messageMsg)
	if !ok || msg.text != "jj redo, press u to undo" {
		t.Errorf("redo reported %#v", msg)
	}
}
