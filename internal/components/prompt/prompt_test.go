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

// A description's body and trailers are part of it. The single-line field dropped every line after
// the first, so Enter on an unedited prompt rewrote the description without them.
func TestOpenText_KeepsEveryLineThroughSubmit(t *testing.T) {
	t.Parallel()

	full := "fix: side branch\n\nLong body paragraph explaining why.\nSigned-off-by: X"

	m := prompt.New(testSkin())
	m.OpenText("describe abc", full)

	if got := m.Value(); got != full {
		t.Fatalf("seeded %q, want %q", got, full)
	}

	m.Update(tuitest.Key("alt+enter"))

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("enter did not submit")
	}

	if submitted.Value != "\n"+full {
		t.Errorf("submitted %q, want the line break typed at the start plus every original line", submitted.Value)
	}
}

func TestOpenText_AltEAsksForTheEditorWithWhatWasTyped(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenText("describe abc", "subject")
	m.Update(tuitest.Key("x"))

	asked, ok := drainMsg[prompt.EditorRequestedMsg](m.Update(tuitest.Key("alt+e")))
	if !ok {
		t.Fatal("alt+e did not ask for the editor")
	}

	if asked.Value != "xsubject" || m.IsOpen() {
		t.Errorf("asked with %q (open %v), want the typed text and a closed prompt", asked.Value, m.IsOpen())
	}
}

// The one-line prompts (bookmark names, workspace names) have no editor to hand off to.
func TestOpen_AltEDoesNothingOnASingleLinePrompt(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("new bookmark", "")

	if _, ok := drainMsg[prompt.EditorRequestedMsg](m.Update(tuitest.Key("alt+e"))); ok {
		t.Error("a single-line prompt offered the editor")
	}
}
