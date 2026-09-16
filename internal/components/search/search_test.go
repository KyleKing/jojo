package search_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/search"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

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

func typeString(m *search.Model, s string) {
	for _, r := range s {
		m.Update(tuitest.Key(string(r)))
	}
}

func TestOpenClearsPattern(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	typeString(&m, "old")
	m.Open()

	if got := tuitest.StripANSI(m.View(40)); got != "/" {
		t.Errorf("View after Open() = %q, want %q", got, "/")
	}
}

func TestTypingEmitsChanged(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	m.Open()

	changed, ok := drainMsg[search.ChangedMsg](m.Update(tuitest.Key("f")))
	if !ok || changed.Pattern != "f" {
		t.Fatalf("Update(f) = %+v, %v, want ChangedMsg{Pattern: \"f\"}", changed, ok)
	}

	changed, ok = drainMsg[search.ChangedMsg](m.Update(tuitest.Key("backspace")))
	if !ok || changed.Pattern != "" {
		t.Fatalf("Update(backspace) = %+v, %v, want ChangedMsg{Pattern: \"\"}", changed, ok)
	}
}

func TestStepKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want int
	}{
		{"down", 1},
		{"ctrl+n", 1},
		{"up", -1},
		{"ctrl+p", -1},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			m := search.New(testSkin())
			m.Open()

			stepped, ok := drainMsg[search.SteppedMsg](m.Update(tuitest.Key(tc.key)))
			if !ok || stepped.Delta != tc.want {
				t.Fatalf("Update(%s) = %+v, %v, want Delta %d", tc.key, stepped, ok, tc.want)
			}
		})
	}
}

func TestEnterAcceptsAndCloses(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	m.Open()

	_, ok := drainMsg[search.AcceptedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit AcceptedMsg")
	}

	if m.IsOpen() {
		t.Error("bar still open after enter")
	}
}

func TestEscCancelsAndCloses(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	m.Open()

	_, ok := drainMsg[search.CancelledMsg](m.Update(tuitest.Key("esc")))
	if !ok {
		t.Fatal("Update(esc) did not emit CancelledMsg")
	}

	if m.IsOpen() {
		t.Error("bar still open after esc")
	}
}

func TestClosedIgnoresKeys(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())

	if cmd := m.Update(tuitest.Key("f")); cmd != nil {
		t.Error("closed bar produced a command from a keypress")
	}
}

const testWidth = 40

func TestViewReportsPosition(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pattern string
		status  string
		matches int
		index   int
	}{
		"no pattern":       {pattern: "", matches: 0, index: 0, status: ""},
		"matches found":    {pattern: "wk", matches: 12, index: 3, status: "3/12"},
		"pattern no match": {pattern: "zz", matches: 0, index: 0, status: "no match"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := search.New(testSkin())
			m.Open()
			typeString(&m, tc.pattern)
			m.SetMatches(tc.matches, tc.index)

			left := "/" + tc.pattern
			want := left

			if tc.status != "" {
				want = left + strings.Repeat(" ", testWidth-len(left)-len(tc.status)) + tc.status
			}

			if got := tuitest.StripANSI(m.View(testWidth)); got != want {
				t.Errorf("View(%d) = %q, want %q", testWidth, got, want)
			}
		})
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	m.Open()
	typeString(&m, "wk")
	m.SetMatches(3, 2)

	tuitest.AssertGolden(t, m.View(40))
}
