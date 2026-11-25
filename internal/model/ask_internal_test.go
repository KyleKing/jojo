package model

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/kyleking/jojo/internal/tuitest"
)

// An empty change has nothing to lose, so a abandons it on the one key.
func TestAbandon_EmptyChangeGoesOnOneKey(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	m = press(t, m, "k")

	if rev := m.log.Selected(); rev == nil || !rev.WorkingCopy || !rev.Empty {
		t.Fatalf("the fixture's cursor is on %+v, want the empty @", rev)
	}

	ops := opCount(t, dir)
	m = press(t, m, "a")

	if m.asking != nil {
		t.Fatalf("a on an empty change asked %q", m.asking.question)
	}

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("a on an empty change took %d operations, want 1", got)
	}
}

// A change with files asks first, naming them; any key but y keeps it and does nothing else, and y
// abandons it.
func TestAbandon_ChangeWithFilesAsksFirst(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "middle")
	id := changeFor(t, dir, "middle")
	ops := opCount(t, dir)

	m = press(t, m, "a")
	if m.asking == nil {
		t.Fatalf("a on a change with a file abandoned it at once: %q", m.message)
	}

	want := "abandon " + id + " and its 1 changed file?"
	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, want) ||
		!strings.Contains(frame, "y abandon") || !strings.Contains(frame, "n keep") {
		t.Errorf("the footer does not ask %q with its keys:\n%s", want, frame)
	}

	m = press(t, m, "j")
	if m.asking != nil || m.log.Selected().ChangeID != id {
		t.Error("the key that declined also moved the cursor")
	}

	if opCount(t, dir) != ops || bookmarkAt(t, dir, id) != "middle" || !strings.Contains(m.message, "kept "+id) {
		t.Fatalf("declining wrote to the repository or said %q", m.message)
	}

	press(t, m, "a", "y")

	if got := bookmarkAt(t, dir, id); got != "" {
		t.Errorf("a y left %s in the log as %q", id, got)
	}
}

// Checked changes ask once for all of them, with the files they take together.
func TestAbandon_CheckedChangesAskWithTheirFiles(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	m = press(t, m, "space", "j", "space")

	m = press(t, m, "a")
	if m.asking == nil {
		t.Fatalf("a on two checked changes with files abandoned them at once: %q", m.message)
	}

	if q := m.asking.question; !strings.Contains(q, " and ") || !strings.HasSuffix(q, "their 2 changed files?") {
		t.Errorf("the question is %q, want both changes and their 2 files", q)
	}

	press(t, m, "y")

	for _, desc := range []string{"top", "middle"} {
		if got := jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy",
			"-r", "description(exact:'"+desc+"\n')", "-T", "change_id"); got != "" {
			t.Errorf("a y left the checked change %s in place", desc)
		}
	}
}

// The question is cut before its keys are: at any width the footer is one row that still says how
// to answer it.
func TestAskFooter_KeepsItsKeysAtEveryWidth(t *testing.T) {
	t.Parallel()

	for _, width := range []int{40, 80, 120} {
		m := testModel(t, twoRows())
		m.width = width
		m.openAsk(ask{question: "abandon " + strings.Repeat("x", 200) + "?", verb: "abandon", undoable: true})

		line := tuitest.StripANSI(m.askFooter())
		if lipgloss.Width(line) != width || !strings.Contains(line, "y abandon") || !strings.Contains(line, "n keep") {
			t.Errorf("at %d the ask reads %q", width, line)
		}
	}
}
