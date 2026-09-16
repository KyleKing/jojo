package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

func testOptions(t *testing.T) Options {
	t.Helper()

	keys, err := help.NewKeymap(nil)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	return Options{Keys: keys}
}

func testModel(t *testing.T, rows []jj.LogRow) Model {
	t.Helper()

	m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", testOptions(t))
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: rows})

	return m
}

func twoRows() []jj.LogRow {
	return []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa", Description: "source"}},
		{Revision: &jj.Revision{ChangeID: "bbbb", Description: "destination"}},
	}
}

// The picker collects an argument for an action chosen earlier, so the action has to survive every
// message between the keypress and the pick.
func TestPickerRunsTheActionThatOpenedIt(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"s", "r", "R"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			next, _, handled := testModel(t, twoRows()).handleActionKey(key)
			if !handled {
				t.Fatalf("%q was not handled", key)
			}

			opened := asModel(t, next)
			if opened.onPick == nil {
				t.Fatal("the picker opened with no action to run")
			}

			if !opened.destPicker.IsOpen() {
				t.Fatal("picker did not open")
			}

			done, cmd := opened.handlePicked(destpicker.PickedMsg{Revision: jj.Revision{ChangeID: "bbbb"}})
			if cmd == nil {
				t.Error("picking a destination ran nothing")
			}

			if asModel(t, done).onPick != nil {
				t.Error("the action survived the pick, so the next one would run it again")
			}
		})
	}
}

// The whole point of routing through Update is that nothing on the way resets
// the action, which is what a value receiver mutating a copy would do.
func TestPickerSurvivesTheUpdateLoop(t *testing.T) {
	t.Parallel()

	var model tea.Model = testModel(t, twoRows())

	model, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})

	opened, ok := model.(Model)
	if !ok {
		t.Fatalf("Update returned %T", model)
	}

	if opened.onPick == nil {
		t.Fatal("typing into the filter lost the action the picker was collecting for")
	}

	_, cmd := model.Update(destpicker.PickedMsg{Revision: jj.Revision{ChangeID: "bbbb"}})
	if cmd == nil {
		t.Error("picking a destination through Update ran nothing")
	}
}

// Typing into the picker's filter must not disturb the action it is collecting
// for, which is the exact path the PTY found broken.
func TestPickerSurvivesAFilteredPick(t *testing.T) {
	t.Parallel()

	var model tea.Model = testModel(t, twoRows())

	model, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})

	for _, r := range "destin" {
		model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a filtered match ran nothing")
	}

	if msg := cmd(); msg == nil {
		t.Fatal("the pick produced no message")
	}
}
