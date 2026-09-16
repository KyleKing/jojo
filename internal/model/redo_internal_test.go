package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func ctrlR() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
}

// Redo is only meaningful once an undo has recorded where the repository was, so the key has to say
// it has nothing to go back to rather than restoring some arbitrary operation.
func TestRedoNeedsAnUndoFirst(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())

	next, cmd := model.Update(ctrlR())
	if cmd == nil {
		t.Fatal("ctrl+r produced no command")
	}

	msg, ok := cmd().(messageMsg)
	if !ok {
		t.Fatalf("ctrl+r produced %T, want a message", cmd())
	}

	if msg.text != "nothing to redo" {
		t.Errorf("message is %q, want the empty-history message", msg.text)
	}

	recorded, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}

	after, _ := recorded.Update(redoPointMsg{id: "abcd1234"})

	model, ok = after.(Model)
	if !ok {
		t.Fatalf("Update returned %T", after)
	}

	if model.redoOp != "abcd1234" {
		t.Fatalf("redoOp = %q, want the recorded operation", model.redoOp)
	}

	if _, cmd = model.Update(ctrlR()); cmd == nil {
		t.Fatal("ctrl+r produced no command with an operation recorded")
	}
}
