package model

import (
	"testing"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

const fullDescription = "fix: side branch\n\nLong body paragraph explaining why.\nSigned-off-by: X"

func describing(t *testing.T) Model {
	t.Helper()

	m, _ := deliver(testModel(t, twoRows()),
		jj.Detail{ChangeID: "aaaa", Description: fullDescription}, nil, openDescribe)

	if !m.prompt.IsOpen() {
		t.Fatal("describe did not open the prompt")
	}

	return m
}

// The log row carries the first line only, and seeding the prompt with it dropped the body and the
// trailers on Enter. The prompt starts from the whole description.
func TestDescribe_SeedsThePromptWithTheBodyAndTrailers(t *testing.T) {
	t.Parallel()

	if got := describing(t).prompt.Value(); got != fullDescription {
		t.Errorf("prompt holds %q, want the full description", got)
	}
}

// Enter on an unedited description used to rewrite the revision, an operation to undo for nothing.
func TestDescribe_EnterOnAnUnchangedDescriptionWritesNothing(t *testing.T) {
	t.Parallel()

	m := describing(t)

	_, cmd := m.handleSubmitted(prompt.SubmittedMsg{Value: fullDescription + "\n"})
	if cmd == nil {
		t.Fatal("submitting produced nothing to say")
	}

	msg, ok := cmd().(messageMsg)
	if !ok || msg.text != "description of aaaa unchanged" {
		t.Errorf("unchanged submit produced %#v, want the unchanged message and no write", cmd())
	}
}

func TestDescribe_AltEHandsTheTypedTextToTheEditor(t *testing.T) {
	t.Parallel()

	m := describing(t)

	next, cmd := m.Update(tuitest.Key("alt+e"))
	if cmd == nil {
		t.Fatal("alt+e produced no command")
	}

	asked, ok := cmd().(prompt.EditorRequestedMsg)
	if !ok {
		t.Fatalf("alt+e produced %T, want the editor request", cmd())
	}

	done, run := asModel(t, next).Update(asked)
	if run == nil {
		t.Fatal("the editor request ran nothing")
	}

	if model := asModel(t, done); model.onEditor != nil || model.onSubmit != nil || model.prompt.IsOpen() {
		t.Error("the describe survived the hand-off, so a later prompt would run it")
	}
}
