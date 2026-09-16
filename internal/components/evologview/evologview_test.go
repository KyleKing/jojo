package evologview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/evologview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 60, 12

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

// entries returns three evolution entries, newest first, as Client.Evolog
// does, with commit ids "c0" (newest) through "c2" (oldest).
func entries() []jj.Evolution {
	const n = 3

	out := make([]jj.Evolution, 0, n)
	for i := range n {
		out = append(out, jj.Evolution{
			CommitID:    "c" + string(rune('0'+i)),
			Description: "desc",
			Age:         "1h ago",
			Operation:   "describe commit",
		})
	}

	return out
}

func TestOpen_CursorStartsOnNewestEntry(t *testing.T) {
	t.Parallel()

	m := evologview.New(testSkin())
	m.Open("abc", entries())

	id, ok := drainMsg[evologview.ShowMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit evologview.ShowMsg")
	}

	if id.CommitID != "c0" {
		t.Errorf("initial cursor commit = %q, want %q (newest)", id.CommitID, "c0")
	}
}

// The newest version is on top, as jj evolog lists it, and the numbering counts from the oldest so a
// version keeps its number as newer ones arrive.
func TestViewListsNewestFirstAndNumbersFromTheOldest(t *testing.T) {
	t.Parallel()

	m := evologview.New(testSkin())
	m.Open("abc", entries())

	view := tuitest.StripANSI(m.View(testWidth, testHeight))

	first := strings.Index(view, "[3]")
	last := strings.Index(view, "[1]")

	if first < 0 || last < 0 {
		t.Fatalf("View() = %q, want three numbered entries", view)
	}

	if first > last {
		t.Errorf("View() = %q, want the newest entry above the oldest", view)
	}
}

func TestMoveCursor_ClampsAtBothEnds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		want    string
		presses []string
	}{
		{name: "up clamps at the newest", presses: []string{"up", "up"}, want: "c0"},
		{name: "down clamps at the oldest", presses: []string{"down", "down", "down"}, want: "c2"},
		{name: "home jumps to the newest", presses: []string{"end", "home"}, want: "c0"},
		{name: "end jumps to the oldest", presses: []string{"end"}, want: "c2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := evologview.New(testSkin())
			m.Open("abc", entries())

			for _, key := range tc.presses {
				m.Update(tuitest.Key(key))
			}

			id, ok := drainMsg[evologview.ShowMsg](m.Update(tuitest.Key("enter")))
			if !ok {
				t.Fatal("Update(enter) did not emit evologview.ShowMsg")
			}

			if id.CommitID != tc.want {
				t.Errorf("commit after %v = %q, want %q", tc.presses, id.CommitID, tc.want)
			}
		})
	}
}

func TestInterdiff_OrdersFromOldestToNewest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		wantFrom string
		wantTo   string
		markAt   []string
		cursorAt []string
	}{
		{
			name:     "mark older than cursor",
			markAt:   []string{"end"},
			cursorAt: []string{"home"},
			wantFrom: "c2",
			wantTo:   "c0",
		},
		{
			name:     "mark newer than cursor",
			markAt:   []string{},
			cursorAt: []string{"end"},
			wantFrom: "c2",
			wantTo:   "c0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := evologview.New(testSkin())
			m.Open("abc", entries())

			for _, key := range tc.markAt {
				m.Update(tuitest.Key(key))
			}

			m.Update(tuitest.Key("space"))

			for _, key := range tc.cursorAt {
				m.Update(tuitest.Key(key))
			}

			diff, ok := drainMsg[evologview.InterdiffMsg](m.Update(tuitest.Key("d")))
			if !ok {
				t.Fatal("Update(d) did not emit evologview.InterdiffMsg")
			}

			if diff.From != tc.wantFrom || diff.To != tc.wantTo {
				t.Errorf("interdiff = {From: %q, To: %q}, want {From: %q, To: %q}",
					diff.From, diff.To, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestInterdiff_RefusesWithoutTwoDistinctMarks(t *testing.T) {
	t.Parallel()

	t.Run("no mark set", func(t *testing.T) {
		t.Parallel()

		m := evologview.New(testSkin())
		m.Open("abc", entries())

		if _, ok := drainMsg[evologview.InterdiffMsg](m.Update(tuitest.Key("d"))); ok {
			t.Fatal("Update(d) with no mark emitted evologview.InterdiffMsg")
		}

		if !containsAll(m.View(testWidth, testHeight), "mark a second version with space") {
			t.Error("View() does not warn that a second version needs marking")
		}
	})

	t.Run("mark on cursor row", func(t *testing.T) {
		t.Parallel()

		m := evologview.New(testSkin())
		m.Open("abc", entries())
		m.Update(tuitest.Key("space"))

		if _, ok := drainMsg[evologview.InterdiffMsg](m.Update(tuitest.Key("d"))); ok {
			t.Fatal("Update(d) with mark on cursor row emitted evologview.InterdiffMsg")
		}

		if !containsAll(m.View(testWidth, testHeight), "mark a second version with space") {
			t.Error("View() does not warn that a second version needs marking")
		}
	})
}

func TestSpace_MovesMarkRatherThanAddingSecond(t *testing.T) {
	t.Parallel()

	m := evologview.New(testSkin())
	m.Open("abc", entries())
	m.Update(tuitest.Key("space")) // mark c0
	m.Update(tuitest.Key("end"))
	m.Update(tuitest.Key("space")) // move the mark from c0 to c2
	m.Update(tuitest.Key("home"))  // cursor back to c0

	diff, ok := drainMsg[evologview.InterdiffMsg](m.Update(tuitest.Key("d")))
	if !ok {
		t.Fatal("Update(d) did not emit evologview.InterdiffMsg")
	}

	if diff.From != "c2" || diff.To != "c0" {
		t.Errorf("interdiff = {From: %q, To: %q}, want {From: %q, To: %q}", diff.From, diff.To, "c2", "c0")
	}
}

func TestRestore_EmitsCursorCommitID(t *testing.T) {
	t.Parallel()

	m := evologview.New(testSkin())
	m.Open("abc", entries())
	m.Update(tuitest.Key("down"))

	id, ok := drainMsg[evologview.RestoreMsg](m.Update(tuitest.Key("r")))
	if !ok {
		t.Fatal("Update(r) did not emit evologview.RestoreMsg")
	}

	if id.CommitID != "c1" {
		t.Errorf("restore commit = %q, want %q", id.CommitID, "c1")
	}
}

func TestEscAndQ_CancelAndClose(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"esc", "q"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := evologview.New(testSkin())
			m.Open("abc", entries())

			if _, ok := drainMsg[evologview.CancelledMsg](m.Update(tuitest.Key(key))); !ok {
				t.Fatalf("Update(%s) did not emit evologview.CancelledMsg", key)
			}

			if m.IsOpen() {
				t.Errorf("Update(%s) left the view open", key)
			}
		})
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := evologview.New(testSkin())
	m.Open("abc123", []jj.Evolution{
		{CommitID: "c0", Description: "fix typo", Age: "2h ago", Operation: "describe commit"},
		{CommitID: "c1", Description: "", Age: "3h ago", Operation: "new commit"},
	})
	m.Update(tuitest.Key("end"))
	m.Update(tuitest.Key("space"))
	m.Update(tuitest.Key("home"))

	tuitest.AssertGolden(t, tuitest.StripANSI(m.View(testWidth, testHeight)))
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}

	return true
}
