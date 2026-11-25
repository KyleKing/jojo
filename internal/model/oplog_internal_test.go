package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
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

	model, _ = deliver(model, operations(), nil, showOpLog)

	if !model.opLog.IsOpen() {
		t.Fatal("the operation log did not open")
	}

	model = asModel(t, update(model, tea.KeyPressMsg{Code: 'j', Text: "j"}))

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

	failed, _ := deliver(model, []jj.Operation(nil), jj.ErrCommandFailed, showOpLog)

	if failed.opLog.IsOpen() {
		t.Error("a failed operation log still opened the view")
	}

	if failed.message == "" {
		t.Error("a failed operation log reported nothing")
	}
}

// The operation's diff opens in the pane rather than handing the terminal to jj's pager.
func TestOpLogShow_ReadsTheDiffIntoThePane(t *testing.T) {
	t.Parallel()

	_, cmd := testModel(t, twoRows()).showOperation(oplogview.ShowMsg{Operation: operations()[0]})
	if cmd == nil {
		t.Fatal("showing an operation ran nothing")
	}

	if msg, ok := cmd().(diffLoadedMsg); !ok || msg.title != "operation aaa111" {
		t.Errorf("showing an operation produced %#v, want a diff read for the pane", msg)
	}
}

// Opened over the log as it stood, the op log showed six operations of two hundred on a 35-row
// terminal. It takes half the frame, so a reader sees enough of the list to choose by.
func TestOpLogFrame_ListsEnoughOperations(t *testing.T) {
	t.Parallel()

	ops := make([]jj.Operation, 200)
	for i := range ops {
		ops[i] = jj.Operation{ID: fmt.Sprintf("op%04d", i), Description: "describe commit", Age: "1h"}
	}

	ops[0].Current = true

	for _, size := range []struct{ height, want int }{{35, 10}, {24, 5}} {
		m := testModel(t, twoRows())
		m, _ = deliver(m, ops, nil, showOpLog)
		m.travel.active = true

		frame := tuitest.StripANSI(m.opLogFrame(120, size.height))
		if got := strings.Count(frame, "describe commit"); got < size.want {
			t.Errorf("at %d rows the op log lists %d operations, want at least %d:\n%s", size.height, got,
				size.want, frame)
		}
	}
}
