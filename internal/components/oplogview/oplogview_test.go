package oplogview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 60, 12

func testSkin() ui.Skin {
	return ui.New(theme.Macchiato())
}

//nolint:ireturn // T is concrete at every call site, only the signature is generic.
func drainMsg[T any](cmd tea.Cmd) (T, bool) {
	var zero T

	if cmd == nil {
		return zero, false
	}

	msg, ok := cmd().(T)

	return msg, ok
}

func op(id, description string, current bool) jj.Operation {
	return jj.Operation{ID: id, Description: description, Age: "1m", Current: current}
}

func TestOpenPlacesCursorOnCurrentOperation(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{
		op("aaa", "one", false),
		op("bbb", "two", true),
		op("ccc", "three", false),
	})

	m.Update(tuitest.Key("up"))

	restored, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after moving up from the current operation did not emit oplogview.RestoreMsg")
	}

	if restored.Operation.ID != "aaa" {
		t.Errorf("cursor after Open+up = %q, want %q", restored.Operation.ID, "aaa")
	}
}

func TestOpenWithNoCurrentOperationStartsAtFirst(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", false), op("bbb", "two", false)})

	restored, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit oplogview.RestoreMsg")
	}

	if restored.Operation.ID != "aaa" {
		t.Errorf("cursor after Open with no current op = %q, want %q", restored.Operation.ID, "aaa")
	}
}

func TestCursorClampsAtTop(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", false), op("bbb", "two", false)})
	m.Update(tuitest.Key("up"))

	restored, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) at the top did not emit oplogview.RestoreMsg")
	}

	if restored.Operation.ID != "aaa" {
		t.Errorf("cursor after up at top = %q, want %q (should not wrap)", restored.Operation.ID, "aaa")
	}
}

func TestCursorClampsAtBottom(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", false), op("bbb", "two", false)})
	m.Update(tuitest.Key("G"))
	m.Update(tuitest.Key("down"))

	restored, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) at the bottom did not emit oplogview.RestoreMsg")
	}

	if restored.Operation.ID != "bbb" {
		t.Errorf("cursor after down at bottom = %q, want %q (should not wrap)", restored.Operation.ID, "bbb")
	}
}

func TestEnterOnNonCurrentOperationEmitsRestore(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", true), op("bbb", "two", false)})
	m.Update(tuitest.Key("down"))

	restored, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) on a non-current operation did not emit oplogview.RestoreMsg")
	}

	if restored.Operation.ID != "bbb" {
		t.Errorf("restored operation = %q, want %q", restored.Operation.ID, "bbb")
	}

	if m.IsOpen() {
		t.Error("Update(enter) did not close the view")
	}
}

func TestEnterOnCurrentOperationWarnsWithoutRestoring(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", true), op("bbb", "two", false)})

	if _, ok := drainMsg[oplogview.RestoreMsg](m.Update(tuitest.Key("enter"))); ok {
		t.Fatal("Update(enter) on the current operation emitted oplogview.RestoreMsg")
	}

	if !m.IsOpen() {
		t.Error("Update(enter) on the current operation closed the view")
	}

	if !strings.Contains(m.View(testWidth, testHeight), "already at this operation") {
		t.Error("View() does not explain why enter on the current operation was refused")
	}
}

func TestCursorMoveClearsWarning(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", true), op("bbb", "two", false)})
	m.Update(tuitest.Key("enter"))
	m.Update(tuitest.Key("down"))

	if strings.Contains(m.View(testWidth, testHeight), "already at this operation") {
		t.Error("View() still shows the warning after the cursor moved")
	}
}

func TestEscCancelsWithoutRestoring(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{op("aaa", "one", true)})

	if _, ok := drainMsg[oplogview.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("Update(esc) did not emit oplogview.CancelledMsg")
	}

	if m.IsOpen() {
		t.Error("Update(esc) did not close the view")
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := oplogview.New(testSkin())
	m.Open([]jj.Operation{
		op("aaa111", "snapshot working copy", true),
		op("bbb222", "describe commit bbb", false),
		op("ccc333", "rebase", false),
	})
	m.Update(tuitest.Key("down"))

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
