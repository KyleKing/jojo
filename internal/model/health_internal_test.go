package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

func TestWrote_ReportsStaleAsRecoveryNotDeadEnd(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("jj log: %w", jj.ErrStaleWorkingCopy)
	if _, ok := wrote("jj log", "", err).(staleMsg); !ok {
		t.Fatal("a stale working copy should open the update-stale offer, not a status line")
	}
}

func TestStaleMsg_OpensTheRecoveryDialog(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())

	next, _ := m.Update(staleMsg{err: fmt.Errorf("wrap: %w", jj.ErrStaleWorkingCopy)})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if !got.confirm.IsOpen() {
		t.Fatal("a stale working copy should open the update-stale confirmation")
	}
}

func TestOfferDivergent_ListsWhatWouldGo(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())

	next, _ := m.Update(divergentPreviewMsg{sides: []string{"aaaa dead side", "bbbb other side"}})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if !got.confirm.IsOpen() {
		t.Fatal("divergent sides should open the abandon confirmation")
	}
}

func TestOfferDivergent_SaysWhenThereIsNothing(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())

	next, cmd := m.Update(divergentPreviewMsg{})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if got.confirm.IsOpen() {
		t.Fatal("an empty divergent set must not open a confirmation")
	}

	msg, ok := cmd().(messageMsg)
	if !ok || msg.text != "no divergent revisions" {
		t.Errorf("message = %#v, want %q", msg, "no divergent revisions")
	}
}

// Divergence has a native answer since jj 0.45. Pressing y converges, and abandoning the stale
// sides, which was the only offer before, stays on a second key.
func TestOfferDivergent_ConvergesOnYAndAbandonsOnA(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()),
		divergentPreviewMsg{sides: []string{"aaaa one side", "aaaa other side"}, focus: "aaaa"}))

	if !m.confirm.IsOpen() || m.converging != "aaaa" {
		t.Fatalf("the dialog did not open over the change under the cursor (converging %q)", m.converging)
	}

	if view := tuitest.StripANSI(m.confirm.View(100, 20)); !strings.Contains(view, "converge aaaa?") ||
		!strings.Contains(view, "[a]bandon stale sides") {
		t.Errorf("the dialog does not offer both ways:\n%s", view)
	}

	for _, key := range []string{"y", "a"} {
		cmd := m.confirm.Update(tuitest.Key(key))
		if cmd == nil {
			t.Fatalf("%s did nothing", key)
		}

		msg, ok := cmd().(confirm.ConfirmedMsg)
		if !ok || msg.Alt != (key == "a") {
			t.Errorf("%s confirmed %#v", key, msg)
		}

		m.confirm.Open(confirm.Request{Key: confirmDivergent, AltKey: "a"})
	}
}

func TestConverge_ScopesToTheChangeUnderTheCursor(t *testing.T) {
	t.Parallel()

	if got := strings.Join(convergeArgs("aaaa"), " "); got != "converge -r change_id(aaaa)" {
		t.Errorf("converge on a divergent row runs %q", got)
	}

	if got := strings.Join(convergeArgs(""), " "); got != "converge" {
		t.Errorf("converge with no divergent row runs %q", got)
	}
}
