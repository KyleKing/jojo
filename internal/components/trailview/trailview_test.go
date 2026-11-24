package trailview_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/trail"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/trailview"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// session is a trail of a read, a write that made an operation, a failed write, and a newest write.
func session() *trail.Trail {
	log := trail.New(0)
	at := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)

	log.Add(trail.Call{Argv: []string{"jj", "log", "-r", "@"}, Start: at})
	log.Add(trail.Call{
		Argv: []string{"jj", "squash", "--from", "a", "--into", "b"}, Write: true, Op: "1111", Start: at,
	})
	log.Add(trail.Call{
		Argv: []string{"jj", "git", "push"}, Write: true, Exit: 1, Start: at, Stderr: "Error: refused\nmore\n",
	})
	log.Add(trail.Call{Argv: []string{"jj", "describe", "-m", "x y"}, Write: true, Op: "2222", Start: at})

	return log
}

func open(failed bool) trailview.Model {
	m := trailview.New(ui.New(theme.Macchiato()))
	m.Open(session(), failed)

	return m
}

// answer presses key and runs the command it answers, nil when it answers none.
//
//nolint:ireturn // a tea.Msg is what a command answers; each test asserts the concrete one.
func answer(t *testing.T, m *trailview.Model, key string) tea.Msg {
	t.Helper()

	cmd := m.Update(tuitest.Key(key))
	if cmd == nil {
		return nil
	}

	return cmd()
}

// The list shows the writes, newest first, on the newest failure when one is on screen, and r adds
// the reads.
func TestOpen_ListsWritesAndLandsOnTheFailure(t *testing.T) {
	t.Parallel()

	m := open(true)
	if c, ok := m.Selected(); !ok || !c.Failed() {
		t.Fatalf("the cursor is on %+v, want the failed push", c)
	}

	frame := tuitest.StripANSI(m.View(100, 20))
	if strings.Contains(frame, "jj log -r @") || !strings.Contains(frame, "3 writes") {
		t.Errorf("the list shows reads or miscounts:\n%s", frame)
	}

	answer(t, &m, "r")

	if frame := tuitest.StripANSI(m.View(100, 20)); !strings.Contains(frame, "jj log -r @") {
		t.Errorf("r did not add the reads:\n%s", frame)
	}
}

// Enter asks for the whole stderr, Y copies the shell line, and u undoes a write: the newest with
// jj undo, an older one by its operation, and a failed one not at all.
func TestUpdate_AnswersEachKey(t *testing.T) {
	t.Parallel()

	m := open(false)

	if undo, ok := answer(t, &m, "u").(trailview.UndoMsg); !ok || !undo.Newest {
		t.Errorf("u on the newest write answered %+v, want jj undo", undo)
	}

	if copied, ok := answer(t, &m, "Y").(trailview.CopiedMsg); !ok || copied.Line != "jj describe -m 'x y'" {
		t.Errorf("Y answered %+v, want the quoted shell line", copied)
	}

	answer(t, &m, "j")

	if show, ok := answer(t, &m, "enter").(trailview.ShowMsg); !ok || !strings.Contains(show.Call.Stderr, "more") {
		t.Errorf("enter on the failure answered %+v, want all of its stderr", show)
	}

	if msg := answer(t, &m, "u"); msg != nil {
		t.Errorf("u on a failed write answered %+v, want nothing", msg)
	}

	answer(t, &m, "G")

	if undo, ok := answer(t, &m, "u").(trailview.UndoMsg); !ok || undo.Newest || undo.Call.Op != "1111" {
		t.Errorf("u on an older write answered %+v, want a revert of its operation", undo)
	}

	if _, ok := answer(t, &m, "esc").(trailview.CancelledMsg); !ok || m.IsOpen() {
		t.Error("esc did not close the list")
	}
}

// An empty trail says so rather than drawing a blank box.
func TestView_EmptyTrailSaysSo(t *testing.T) {
	t.Parallel()

	m := trailview.New(ui.New(theme.Macchiato()))
	m.Open(trail.New(0), false)

	if frame := tuitest.StripANSI(m.View(80, 12)); !strings.Contains(frame, "nothing has run yet") {
		t.Errorf("an empty trail drew:\n%s", frame)
	}
}
