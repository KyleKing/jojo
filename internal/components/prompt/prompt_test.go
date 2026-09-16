package prompt_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 60, 20

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

func TestEnterSubmitsEmptyValue(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "")

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit prompt.SubmittedMsg")
	}

	if submitted.Value != "" {
		t.Errorf("SubmittedMsg.Value = %q, want empty", submitted.Value)
	}

	if m.IsOpen() {
		t.Error("prompt still open after submitting")
	}
}

func TestEnterSubmitsTypedValue(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "")

	for _, r := range "wip" {
		m.Update(tuitest.Key(string(r)))
	}

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit prompt.SubmittedMsg")
	}

	if submitted.Value != "wip" {
		t.Errorf("SubmittedMsg.Value = %q, want %q", submitted.Value, "wip")
	}
}

func TestEscCancelsWithoutSubmitting(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "existing")

	if _, ok := drainMsg[prompt.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("Update(esc) did not emit prompt.CancelledMsg")
	}

	if m.IsOpen() {
		t.Error("prompt still open after canceling")
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe change", "wip: ")

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
