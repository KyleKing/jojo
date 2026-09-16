package palette_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/palette"
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

func testCommands() []palette.Command {
	return []palette.Command{
		{ID: "o", Key: "o", Name: "operation log", What: "browse jj's operation log"},
		{ID: "R", Key: "R", Name: "restore workspace", What: "restore from the operation log"},
		{ID: "ctrl+r", Key: "ctrl+r", Name: "refresh", What: "reread the working copy"},
	}
}

func typeString(m *palette.Model, s string) {
	for _, r := range s {
		m.Update(tuitest.Key(string(r)))
	}
}

func TestFilterByName(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	typeString(&m, "restore")

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after filtering by name did not emit palette.RunMsg")
	}

	if run.ID != "R" {
		t.Errorf("filter by name ran %q, want %q", run.ID, "R")
	}
}

func TestFilterByKey(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	typeString(&m, "ctrl+r")

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after filtering by key did not emit palette.RunMsg")
	}

	if run.ID != "ctrl+r" {
		t.Errorf("filter by key ran %q, want %q", run.ID, "ctrl+r")
	}
}

func TestOrderSurvivesFilter(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open([]palette.Command{
		{ID: "z", Key: "z", Name: "zzz match", What: "one"},
		{ID: "a", Key: "a", Name: "aaa match", What: "two"},
	})
	typeString(&m, "match")

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after filtering did not emit palette.RunMsg")
	}

	if run.ID != "z" {
		t.Errorf("cursor after filtering settled on %q, want the first supplied match %q", run.ID, "z")
	}
}

func TestEnterWithNoMatchEmitsNothing(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	typeString(&m, "nonexistent")

	if _, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter"))); ok {
		t.Fatal("Update(enter) with no match emitted palette.RunMsg")
	}
}

func TestEscCancelsWithoutRunning(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())

	if _, ok := drainMsg[palette.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("Update(esc) did not emit palette.CancelledMsg")
	}
}

func TestLetterTypesIntoFilterInsteadOfMoving(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	m.Update(tuitest.Key("j"))

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if ok {
		t.Fatalf("Update(enter) after typing %q ran %q, want no match ('j' has no key or name hit)", "j", run.ID)
	}
}

func TestBackspaceDeletesARune(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	typeString(&m, "refreshx")
	m.Update(tuitest.Key("backspace"))

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after backspacing to a valid filter did not emit palette.RunMsg")
	}

	if run.ID != "ctrl+r" {
		t.Errorf("filter after backspace ran %q, want %q", run.ID, "ctrl+r")
	}
}

func TestCursorClampsAtEitherEnd(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())

	m.Update(tuitest.Key("up"))

	run, ok := drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after moving up past the top did not emit palette.RunMsg")
	}

	if run.ID != "o" {
		t.Errorf("cursor clamped past the top and ran %q, want the first command %q", run.ID, "o")
	}

	m.Open(testCommands())

	for range len(testCommands()) + 1 {
		m.Update(tuitest.Key("down"))
	}

	run, ok = drainMsg[palette.RunMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) after moving down past the bottom did not emit palette.RunMsg")
	}

	if run.ID != "ctrl+r" {
		t.Errorf("cursor clamped past the bottom and ran %q, want the last command %q", run.ID, "ctrl+r")
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := palette.New(testSkin())
	m.Open(testCommands())
	m.Update(tuitest.Key("down"))

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
