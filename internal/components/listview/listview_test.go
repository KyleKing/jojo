package listview_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func skin() ui.Skin { return ui.New(theme.Macchiato()) }

// items are deliberately awkward: a long line, wide CJK cells, and an emoji, because those are what
// break a frame that measures bytes instead of cells.
func items() []string {
	return []string{
		"kxryrzwz main fix the parser so the separator can appear inside a description without splitting it",
		//nolint:gosmopolitan // the point of this row is that its cells are twice as wide as its runes.
		"mursrsmx 変更をまとめる、日本語の説明はセル幅が二倍になる",
		"ovrvxwyt 🎉 released",
		"zrosnlqn short",
		"otpqtrvk another",
	}
}

func row(item string, selected bool) listview.Row {
	style := skin().Body
	if selected {
		style = skin().Selected
	}

	return listview.Row{Text: item, Style: style}
}

func hints() []keyhint.Hint {
	return []keyhint.Hint{
		{Key: "enter", What: "pick"},
		{Key: "space", What: "mark"},
		{Key: "esc", What: "cancel"},
	}
}

func opened() listview.Model[string] {
	m := listview.New[string](skin())
	m.Open("a title long enough to need trimming in a narrow frame", items(), 0)

	return m
}

// A frame that renders wider or taller than it was given wraps, and every row below it moves.
func TestViewFitsEveryFrame(t *testing.T) {
	t.Parallel()

	sizes := []struct{ width, height int }{
		{200, 60}, {80, 24}, {60, 20}, {40, 12}, {30, 8}, {20, 5}, {10, 4}, {1, 1},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			m := opened()
			m.Warn("a warning long enough that it too must be trimmed to the frame")

			frame := m.View(size.width, size.height, row, hints())

			for i, line := range strings.Split(frame, "\n") {
				if got := lipgloss.Width(line); got > size.width {
					t.Errorf("line %d is %d cells wide, want at most %d: %q", i, got, size.width, line)
				}
			}

			if lines := strings.Count(frame, "\n") + 1; frame != "" && lines > size.height {
				t.Errorf("frame is %d lines tall, want at most %d", lines, size.height)
			}
		})
	}
}

// Movement clamps rather than wrapping, so holding a key cannot carry the cursor past either end.
func TestMoveKeyClamps(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		keys  []string
		want  int
		taken bool
	}{
		{name: "down", keys: []string{"j", "j"}, want: 2, taken: true},
		{name: "up clamps at the top", keys: []string{"k", "k"}, want: 0, taken: true},
		{name: "end", keys: []string{"G"}, want: 4, taken: true},
		{name: "home after end", keys: []string{"G", "g"}, want: 0, taken: true},
		{name: "page down clamps at the bottom", keys: []string{"pgdown"}, want: 4, taken: true},
		{name: "an unrelated key is left alone", keys: []string{"x"}, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := opened()

			var taken bool
			for _, key := range tc.keys {
				taken = m.MoveKey(key)
			}

			if taken != tc.taken {
				t.Errorf("MoveKey reported %v, want %v", taken, tc.taken)
			}

			if m.Cursor() != tc.want {
				t.Errorf("cursor = %d, want %d", m.Cursor(), tc.want)
			}
		})
	}
}

// A warning is the answer to the key just pressed, so moving on clears it.
func TestMovingClearsTheWarning(t *testing.T) {
	t.Parallel()

	m := opened()
	m.Warn("cannot do that here")

	if !strings.Contains(tuitest.StripANSI(m.View(80, 20, row, hints())), "cannot do that here") {
		t.Fatal("the warning was not shown")
	}

	m.MoveKey("j")

	if strings.Contains(tuitest.StripANSI(m.View(80, 20, row, hints())), "cannot do that here") {
		t.Error("the warning survived a cursor move")
	}
}

func TestSelectedReportsEmptiness(t *testing.T) {
	t.Parallel()

	empty := listview.New[string](skin())
	empty.Open("nothing here", nil, 0)

	if _, ok := empty.Selected(); ok {
		t.Error("an empty list reported a selection")
	}

	if item, ok := opened().Selected(); !ok || item != items()[0] {
		t.Errorf("Selected = %q, %v, want the first item", item, ok)
	}
}
