package prompt_test

import (
	"strings"
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

	if submitted.Value != full+"\n" {
		t.Errorf("submitted %q, want every original line plus the line break typed at the end", submitted.Value)
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

	if asked.Value != "subjectx" || m.IsOpen() {
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

// The cursor starts at the end of a description, so typing adds to it: it started at the beginning,
// and "hello world" typed into "feat: wip work" read "hello worldfeat: wip work".
func TestOpenText_TypingAddsAtTheEnd(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenText("describe abc", "feat: wip work\n\nbody")

	for _, r := range " more" {
		m.Update(tuitest.Key(string(r)))
	}

	if got := m.Value(); got != "feat: wip work\n\nbody more" {
		t.Errorf("typing into the description gave %q", got)
	}
}

// Every prompt keeps one contract, and its keys say so: enter submits, ctrl+j breaks the line where
// the value can hold one, and esc cancels.
func TestHints_SayTheContract(t *testing.T) {
	t.Parallel()

	text := prompt.New(testSkin())
	text.OpenText("describe abc", "")

	line := prompt.New(testSkin())
	line.Open("new bookmark", "")

	for _, tc := range []struct {
		name string
		m    prompt.Model
		want []string
	}{
		{"multi-line", text, []string{"enter submit", "ctrl+j new line", "alt+e $EDITOR", "esc cancel"}},
		{"one line", line, []string{"enter submit", "esc cancel"}},
	} {
		got := make([]string, 0, len(tc.want))
		for _, h := range tc.m.Hints() {
			got = append(got, h.Key+" "+h.What)
		}

		if strings.Join(got, ", ") != strings.Join(tc.want, ", ") {
			t.Errorf("%s hints %v, want %v", tc.name, got, tc.want)
		}
	}

	if _, ok := drainMsg[prompt.SubmittedMsg](text.Update(tuitest.Key("ctrl+j"))); ok || text.Value() != "\n" {
		t.Errorf("ctrl+j submitted or did not break the line: %q", text.Value())
	}
}
