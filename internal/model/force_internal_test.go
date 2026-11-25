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
			want: "Won't push commit b189 since it has no description: d describes it",
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

const newBookmarkNotice = "Warning: Refusing to create new remote bookmark feature@origin\n" +
	"Hint: Run `jj bookmark track feature@origin` and try again.\nNothing changed.\n"

// jj refuses to create a remote bookmark from a push by revision and the warning was discarded, so P
// on a new bookmark said "nothing to push". It asks again by name, which jj creates and tracks.
func TestPush_NewBookmarkIsOfferedByNameRatherThanNothingToPush(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{{Revision: &jj.Revision{ChangeID: "aaaa", LocalBookmarks: []string{"feature"}}}}
	byRevision := jj.PushOptions{Revisions: []string{"aaaa"}}
	refused := pushPreviewMsg{what: "aaaa", notice: newBookmarkNotice, opts: byRevision}

	next, cmd, _ := testModel(t, rows).offerPush(refused)
	if cmd == nil {
		t.Fatalf("a refused new bookmark ended the push: %q", asModel(t, next).message)
	}

	if asModel(t, next).message != "" {
		t.Errorf("reported %q while asking again", asModel(t, next).message)
	}

	byName := pushPreviewMsg{
		what: "feature", why: "feature is new on origin", opts: jj.PushOptions{Bookmarks: []string{"feature"}},
		notice: "Changes to push to origin:\n  bookmark: feature [add to 311d3d63]\n",
	}

	opened, _, _ := testModel(t, rows).offerPush(byName)
	if view := asModel(t, opened).confirm.View(100, 20); !contains(view, "feature is new on origin") {
		t.Errorf("the dialog does not say why it pushes by name:\n%s", view)
	}
}

// A revision with no bookmark has nothing a push by revision sends, so P offers jj's generated one.
func TestPush_RevisionWithoutABookmarkOffersAGeneratedOne(t *testing.T) {
	t.Parallel()

	byRevision := jj.PushOptions{Revisions: []string{"aaaa"}}
	preview := pushPreviewMsg{what: "aaaa", notice: "Nothing changed.\n", opts: byRevision}

	_, cmd, _ := testModel(t, twoRows()).offerPush(preview)
	if cmd == nil {
		t.Fatal("a revision with no bookmark was not offered jj git push -c")
	}
}

// jj names the refused commit by commit id; the line names it by change id and the cursor lands on
// it, so the d it offers describes that revision and not whatever the cursor was on.
func TestPushRefusal_LandsOnTheCommitItNames(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "qrnkttwl", CommitID: "6a492b1a70ff", Description: "fix"}},
		{Revision: &jj.Revision{ChangeID: "mytstwqm", CommitID: "071738586b65"}},
	}
	m := testModel(t, rows)
	err := fmt.Errorf("%w: Won't push commit 071738586b65 since it has no description (Rejected commit: x)",
		jj.ErrPushRefused)

	next, _ := m.Update(pushRefusedMsg{err: err})
	m = asModel(t, next)

	if got := m.log.Selected().ChangeID; got != "mytstwqm" {
		t.Errorf("the cursor is on %s after the refusal, want mytstwqm", got)
	}

	if want := "Won't push mytstwqm since it has no description: d describes it"; !strings.HasPrefix(m.message, want) {
		t.Errorf("the refusal reads %q, want it to start %q", m.message, want)
	}
}
