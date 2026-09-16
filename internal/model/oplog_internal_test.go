package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
)

func operations() []jj.Operation {
	return []jj.Operation{
		{ID: "aaa111", Description: "snapshot working copy", Age: "1m", Current: true},
		{ID: "bbb222", Description: "describe commit", Age: "2m"},
	}
}

// While the operation log is open it owns every key, or j would move the log's cursor underneath it.
func TestOpLogTakesKeysWhileOpen(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())

	opened, _ := model.applyOpLog(opLogLoadedMsg{ops: operations()})

	model, ok := opened.(Model)
	if !ok {
		t.Fatalf("applyOpLog returned %T", opened)
	}

	if !model.opLog.IsOpen() {
		t.Fatal("the operation log did not open")
	}

	moved, _ := model.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	model, ok = moved.(Model)
	if !ok {
		t.Fatalf("Update returned %T", moved)
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter produced no command")
	}

	if model.log.Selected().ChangeID != twoRows()[0].Revision.ChangeID {
		t.Error("j moved the log's cursor while the operation log was open")
	}
}

func TestOpLogFailureReportsRatherThanOpening(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())

	next, _ := model.applyOpLog(opLogLoadedMsg{err: jj.ErrCommandFailed})

	failed, ok := next.(Model)
	if !ok {
		t.Fatalf("applyOpLog returned %T", next)
	}

	if failed.opLog.IsOpen() {
		t.Error("a failed operation log still opened the view")
	}

	if failed.message == "" {
		t.Error("a failed operation log reported nothing")
	}
}
