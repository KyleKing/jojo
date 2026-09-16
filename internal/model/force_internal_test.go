package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

var errUnrelated = fmt.Errorf("%w: some other failure", jj.ErrCommandFailed)

// A refusal is where most people meet jj's immutability rule, so it offers the way past rather than
// reporting a dead end. Nothing is forced without the dialog: the retry only exists once one ran.
func TestImmutableRefusalOffersTheEscape(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())

	cmd := model.act("abc", func(context.Context, *jj.Client) (string, error) {
		return "jj edit abc", fmt.Errorf("%w: abc is immutable", jj.ErrImmutable)
	})

	if cmd == nil {
		t.Fatal("the action produced no command")
	}

	if model.forcible.fn == nil {
		t.Fatal("the action did not record how to retry itself")
	}

	next, _, handled := model.handleOverlayMsg(refusedMsg{err: jj.ErrImmutable})
	if !handled {
		t.Fatal("a refusal was not handled")
	}

	offered := asModel(t, next)
	if !offered.confirm.IsOpen() {
		t.Fatal("a refusal did not offer the escape hatch")
	}
}

// Without a recorded retry there is nothing to force, so the refusal has to report itself rather
// than open a dialog whose yes would do nothing.
func TestRefusalWithNothingToRetryJustReports(t *testing.T) {
	t.Parallel()

	next, _ := testModel(t, twoRows()).offerForce(refusedMsg{err: jj.ErrImmutable})

	reported := asModel(t, next)
	if reported.confirm.IsOpen() {
		t.Error("a refusal with no retry recorded still opened the dialog")
	}

	if reported.message == "" {
		t.Error("a refusal with no retry recorded said nothing")
	}
}

// Confirming clears the retry, or the next unrelated refusal would offer to force the wrong edit.
func TestConfirmingTheEscapeClearsIt(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())
	model.forcible = forcible{what: "abc", fn: func(context.Context, *jj.Client) (string, error) {
		return "jj edit abc", nil
	}}

	next, cmd := model.handleConfirmed(confirm.ConfirmedMsg{Key: confirmForce})
	if cmd == nil {
		t.Fatal("confirming produced no command")
	}

	if asModel(t, next).forcible.fn != nil {
		t.Error("confirming left the retry in place")
	}
}

// A push jj declines for a fixable reason carries the fix, because the status line is the only place
// the reader is looking when it happens.
func TestPushProblemNamesTheFix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		name string
		want string
	}{
		{
			name: "no description",
			err: fmt.Errorf("%w: Won't push commit b189 since it has no description (Rejected commit: x)",
				jj.ErrPushRefused),
			want: "Won't push commit b189 since it has no description. Press d to describe it",
		},
		{
			name: "unclassified failures are passed through",
			err:  errUnrelated,
			want: errUnrelated.Error(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := pushProblem(tc.err); got != tc.want {
				t.Errorf("pushProblem = %q, want %q", got, tc.want)
			}
		})
	}
}

// The dialog shows jj's own dry run, minus the line saying it was one, because the dialog is the
// thing about to not stop short.
func TestPushConfirmationShowsTheDryRun(t *testing.T) {
	t.Parallel()

	notice := "Changes to push to origin:\n  bookmark: main [add to 311d3d63]\nDry-run requested, not pushing.\n"

	next, _, handled := testModel(t, twoRows()).offerPush(pushPreviewMsg{what: "main", notice: notice})
	if !handled {
		t.Fatal("the push preview was not handled")
	}

	model := asModel(t, next)
	if !model.confirm.IsOpen() {
		t.Fatal("the push preview did not open the dialog")
	}

	view := model.confirm.View(90, 20)
	if want := "bookmark: main"; !contains(view, want) {
		t.Errorf("the dialog does not show %q:\n%s", want, view)
	}

	if contains(view, "Dry-run requested") {
		t.Error("the dialog still says it is a dry run")
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(tuitest.StripANSI(haystack), needle)
}
