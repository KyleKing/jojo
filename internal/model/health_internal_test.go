package model

import (
	"fmt"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
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
