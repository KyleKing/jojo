package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"
)

// ellipsis marks a dropped hint, the same glyph keyhint uses for a truncated word.
const ellipsis = "…"

// FitHints renders hints into at most width cells, dropping whole hints from the right and never
// cutting one. The last hint survives every drop, so callers put the route to the full legend there.
// Below the width of that last hint the line goes blank rather than being cut, since cutting a
// styled string can land inside its escape sequence.
func FitHints(styles keyhint.Styles, hints []keyhint.Hint, width int) string {
	rendered := make([]string, len(hints))
	for i, h := range hints {
		rendered[i] = keyhint.One(styles, h)
	}

	join := func(parts []string) string { return strings.Join(parts, keyhint.Gap) }

	if full := join(rendered); lipgloss.Width(full) <= width || len(rendered) < 2 {
		return fitOrBlank(full, width)
	}

	lead, last := rendered[:len(rendered)-1], rendered[len(rendered)-1]

	for n := len(lead) - 1; n >= 0; n-- {
		kept := append(append([]string{}, lead[:n]...), ellipsis, last)
		if candidate := join(kept); lipgloss.Width(candidate) <= width {
			return candidate
		}
	}

	return fitOrBlank(last, width)
}

func fitOrBlank(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}

	return ""
}

// TooSmallMessage replaces an overlay's frame once the terminal cannot fit its border and padding
// around anything.
const TooSmallMessage = "too small"

// TooSmall renders TooSmallMessage centered in width by height cells. Below this size aragonite's
// overlay.Center stops clipping, so a component that keeps reaching for it hands back a frame wider
// than the terminal.
func TooSmall(width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		table.Truncate(TooSmallMessage, width))
}
