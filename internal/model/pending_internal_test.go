package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

func testModel(t *testing.T, rows []jj.LogRow) Model {
	t.Helper()

	m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@")
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: rows})

	return m
}

func twoRows() []jj.LogRow {
	return []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa", Description: "source"}},
		{Revision: &jj.Revision{ChangeID: "bbbb", Description: "destination"}},
	}
}

// The picker collects an argument for an action chosen earlier, so the action
// has to survive every message between the keypress and the pick.
func TestPickerRunsTheActionThatOpenedIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		want pending
	}{
		{name: "squash", key: "s", want: pendingSquash},
		{name: "rebase revision", key: "r", want: pendingRebaseRevisions},
		{name: "rebase subtree", key: "R", want: pendingRebaseSource},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			next, _, handled := testModel(t, twoRows()).handleActionKey(tc.key)
			if !handled {
				t.Fatalf("%q was not handled", tc.key)
			}

			opened, ok := next.(Model)
			if !ok {
				t.Fatalf("handleActionKey returned %T", next)
			}

			if opened.pending != tc.want {
				t.Fatalf("pending = %v, want %v", opened.pending, tc.want)
			}

			if !opened.destPicker.IsOpen() {
				t.Fatal("picker did not open")
			}

			picked := destpicker.PickedMsg{Revision: jj.Revision{ChangeID: "bbbb"}}

			done, cmd := opened.handlePicked(picked)
			if cmd == nil {
				t.Error("picking a destination ran nothing")
			}

			if got := done.(Model).pending; got != pendingNone { //nolint:errcheck,forcetypeassert // asserted above.
				t.Errorf("pending = %v after the pick, want cleared", got)
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

	if opened.pending != pendingSquash {
		t.Fatalf("pending = %v after typing into the filter, want pendingSquash", opened.pending)
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
