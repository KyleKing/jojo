// Package listview is the one scrollable overlay every scoped view in jojo is built from: a title, an
// optional warning, a clipped list with a scroll position, and a key hint line. Each view supplies
// its own rows, keys, and messages, so a fix to how a frame is measured lands in all of them at once.
package listview

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// Row is one item as the list draws it: the text, already chosen but not yet fitted, and the style
// it takes. The list owns the truncation, because cutting a string that is already styled can land
// inside an escape sequence.
type Row struct {
	Text  string
	Style lipgloss.Style
}

// Model is a list of T. The zero cursor is the first item.
type Model[T any] struct {
	skin    ui.Skin
	title   string
	warning string
	items   []T
	cursor  int
	open    bool
}

// New returns a closed list drawn in skin.
func New[T any](skin ui.Skin) Model[T] {
	return Model[T]{skin: skin}
}

// Open shows items under title with the cursor at index, clamped into range.
func (m *Model[T]) Open(title string, items []T, cursor int) {
	m.title, m.items, m.warning, m.open = title, items, "", true
	m.cursor = clamp(cursor, 0, len(items)-1)
}

// Close hides the list.
func (m *Model[T]) Close() { m.open = false }

// IsOpen reports whether the list is taking keys.
func (m Model[T]) IsOpen() bool { return m.open }

// Title is what the list was opened under.
func (m Model[T]) Title() string { return m.title }

// Items are what the list is showing.
func (m Model[T]) Items() []T { return m.items }

// Cursor is the index of the selected item.
func (m Model[T]) Cursor() int { return m.cursor }

// Selected reports the item under the cursor, false when the list is empty.
func (m Model[T]) Selected() (T, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		var zero T

		return zero, false
	}

	return m.items[m.cursor], true
}

// Warn sets the line shown above the rows until the cursor moves. It is how a list refuses an action
// without closing or stealing the screen.
func (m *Model[T]) Warn(text string) { m.warning = text }

// pageSize is how far the page keys move, since a list does not know its own height until it draws.
const pageSize = 10

// MoveKey handles the movement every list shares and reports whether it took the key. Movement
// clamps rather than wrapping, so holding a key cannot silently carry the cursor past the end.
func (m *Model[T]) MoveKey(key string) bool {
	switch key {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+u":
		m.move(-pageSize)
	case "pgdown", "ctrl+d":
		m.move(pageSize)
	case "g", "home":
		m.move(-len(m.items))
	case "G", "end":
		m.move(len(m.items))
	default:
		return false
	}

	return true
}

func (m *Model[T]) move(delta int) {
	if len(m.items) == 0 {
		return
	}

	m.cursor = clamp(m.cursor+delta, 0, len(m.items)-1)
	m.warning = ""
}

func clamp(n, lo, hi int) int {
	if hi < lo {
		return lo
	}

	return min(max(n, lo), hi)
}

// A frame smaller than this has no room for a title, a row, and the hints, so the overlay says so
// on one line instead of drawing a box with nothing in it.
const (
	minWidth = 12
	minRows  = 3
	tooSmall = "terminal too small"
)

// scrollCenterDivisor centers the cursor in the visible window rather than pinning it to an edge.
const scrollCenterDivisor = 2

// footerRows is what a clipped list gives up to the scroll-position line.
const footerRows = 1

// View renders the frame at width by height: the title, a warning when set, the visible rows, a
// scroll position when the list is clipped, and the hint line. Hints are dropped from the right
// rather than cut, and every row is truncated rather than wrapped, so the frame never grows.
func (m Model[T]) View(width, height int, row func(item T, selected bool) Row, hints []keyhint.Hint) string {
	styles := m.skin.OverlayStyles()
	inner := overlay.ContentWidth(width, styles)
	rows := overlay.ContentHeight(height, styles)

	if inner < minWidth || rows < minRows {
		return m.skin.Error.Render(table.Truncate(tooSmall, width))
	}

	head := []string{m.skin.Title.Render(table.Truncate(m.title, inner))}
	if m.warning != "" {
		head = append(head, m.skin.Error.Render(table.Truncate(m.warning, inner)))
	}

	hint := ui.FitHints(m.skin.KeyHintStyles(), hints, inner)

	body := max(1, rows-len(head)-1)
	clipped := len(m.items) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	lines := head
	start, end := window(m.cursor, len(m.items), body)

	for i := start; i < end; i++ {
		drawn := row(m.items[i], i == m.cursor)
		lines = append(lines, drawn.Style.Render(table.Truncate(drawn.Text, inner)))
	}

	if clipped {
		lines = append(lines, m.position(start, end, inner))
	}

	return overlay.Center(strings.Join(append(lines, hint), "\n"), width, height, styles)
}

func (m Model[T]) position(start, end, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, len(m.items))

	return m.skin.Subtitle.Render(table.Pad(table.Truncate(text, width), width, table.AlignRight))
}

// window is the half-open range of items to draw, keeping the cursor inside it.
func window(cursor, total, body int) (int, int) {
	if body <= 0 || total <= body {
		return 0, total
	}

	start := clamp(cursor-body/scrollCenterDivisor, 0, total-body)

	return start, start + body
}
