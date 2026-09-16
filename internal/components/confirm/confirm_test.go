package confirm_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/confirm"
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

func testRequest() confirm.Request {
	return confirm.Request{
		Key:    "push",
		Title:  "Push to remote?",
		Detail: []string{"main -> origin"},
		Danger: "cannot be undone once it leaves this machine",
	}
}

func TestUpdate_YConfirms(t *testing.T) {
	t.Parallel()

	m := confirm.New(testSkin())
	m.Open(testRequest())

	confirmed, ok := drainMsg[confirm.ConfirmedMsg](m.Update(tuitest.Key("y")))
	if !ok {
		t.Fatal("Update(y) did not emit confirm.ConfirmedMsg")
	}

	if confirmed.Key != "push" {
		t.Errorf("ConfirmedMsg.Key = %q, want %q", confirmed.Key, "push")
	}

	if m.IsOpen() {
		t.Error("dialog still open after confirming")
	}
}

func TestUpdate_Cancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "n", key: "n"},
		{name: "esc", key: "esc"},
		{name: "q", key: "q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := confirm.New(testSkin())
			m.Open(testRequest())

			if _, ok := drainMsg[confirm.CancelledMsg](m.Update(tuitest.Key(tt.key))); !ok {
				t.Fatalf("Update(%s) did not emit confirm.CancelledMsg", tt.key)
			}

			if m.IsOpen() {
				t.Errorf("dialog still open after canceling with %s", tt.key)
			}
		})
	}
}

func TestUpdate_OtherKeysDoNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{name: "enter", key: "enter"},
		{name: "plain letter", key: "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := confirm.New(testSkin())
			m.Open(testRequest())

			if cmd := m.Update(tuitest.Key(tt.key)); cmd != nil {
				t.Fatalf("Update(%s) returned a command, want nil", tt.key)
			}

			if !m.IsOpen() {
				t.Errorf("dialog closed after %s, want it to stay open", tt.key)
			}
		})
	}
}

func TestView_Golden(t *testing.T) {
	t.Parallel()

	m := confirm.New(testSkin())
	m.Open(testRequest())

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
