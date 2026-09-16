package help

import (
	"fmt"
	"math"
	"strings"

	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// footerRows is how much of the frame a clipped legend gives up to the
// scroll-position line.
const footerRows = 1

// Model is the `?` overlay: a scroll position over the legend, rendered fresh
// from the keymap on every View.
type Model struct {
	skin   ui.Skin
	keys   Keymap
	offset int
}

// New returns a help overlay scrolled to the top, showing the keys in keys rather than the defaults,
// so a rebind reaches the legend too.
func New(skin ui.Skin, keys Keymap) Model {
	return Model{skin: skin, keys: keys}
}

// ScrollBy moves the offset by n lines, clamped at zero. View clamps the
// bottom, since the legend's height depends on the width it renders at.
func (m Model) ScrollBy(n int) Model {
	sum := m.offset + n
	if n > 0 && sum < m.offset {
		sum = math.MaxInt // overflow past ScrollToEnd's sentinel
	}

	m.offset = max(0, sum)

	return m
}

// ScrollToStart scrolls to the top of the legend.
func (m Model) ScrollToStart() Model {
	m.offset = 0

	return m
}

// ScrollToEnd scrolls to the bottom of the legend. The actual offset is
// resolved against the frame's height at render time.
func (m Model) ScrollToEnd() Model {
	m.offset = math.MaxInt

	return m
}

// View renders the legend centered in a width by height frame at the current
// offset, giving up its last row to a "12-24 of 40" line when it is clipped.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := keyhint.Help(m.skin.KeyHintStyles(), m.keys.AllHints(), contentWidth)

	body := contentHeight
	clipped := len(lines) > contentHeight

	if clipped {
		body = max(1, contentHeight-footerRows)
	}

	maxOffset := max(0, len(lines)-body)
	offset := min(max(m.offset, 0), maxOffset)

	visible := lines
	if clipped {
		end := min(offset+body, len(lines))
		footer := m.scrollFooter(offset, end, len(lines), contentWidth)
		visible = append(append([]string{}, lines[offset:end]...), footer)
	}

	return overlay.Center(strings.Join(visible, "\n"), width, height, styles)
}

// scrollFooter reports where the visible window sits in the legend, padded
// before it is styled so no later truncation can cut inside an escape.
func (m Model) scrollFooter(offset, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", offset+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
