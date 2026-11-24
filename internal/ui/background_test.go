package ui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/kyleking/jojo/internal/ui"
)

// A row is several styled cells, each ending in a reset. Wrapping it in one style highlighted only
// the first cell, which is the bug the cursor row had.
func TestOnBackground_ReappliesTheFaceAfterEveryReset(t *testing.T) {
	t.Parallel()

	face := lipgloss.NewStyle().Reverse(true)
	open, _, _ := strings.Cut(face.Render("\x00"), "\x00")

	cases := []struct {
		name string
		line string
	}{
		{name: "plain", line: "abc"},
		{name: "one styled cell", line: "a\x1b[1mb\x1b[0mc"},
		{name: "short resets", line: "\x1b[31ma\x1b[m \x1b[32mb\x1b[m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ui.OnBackground(tc.line, face)
			resets := strings.Count(tc.line, "\x1b[0m") + strings.Count(tc.line, "\x1b[m")

			if n := strings.Count(got, open); n != resets+1 {
				t.Errorf("face opened %d times, want %d (once, then after every reset): %q", n, resets+1, got)
			}
		})
	}
}
