// Package statusbar renders the one-line footer: the bookmarks near @, the
// active revset or a transient status message, and the key hints for what a
// reader can do right now.
package statusbar

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// Bookmark is one bookmark near @, shown because jj has no single "current
// branch" for the footer to pick instead.
type Bookmark struct {
	Name string
	// AtWorkingCopy is true when the bookmark points at @ itself, false when
	// it is only an ancestor or descendant in range.
	AtWorkingCopy bool
}

// Context is what the footer describes.
type Context struct {
	Bookmarks []Bookmark
	Revset    string
	// Message is a transient status line ("abandoned mkyzvz, press u to undo")
	// shown in place of the bookmarks and revset while it is set.
	Message string
	// Undo names the operation u would reverse, kept visible rather than shown
	// only in the moments after an edit, so the way back is never a guess.
	Undo string
	// Hints are shown most important first; the last one is never dropped, so
	// callers put the route to the full legend (typically "?" help) there.
	Hints []keyhint.Hint
}

// Model is stateless: the footer is rendered entirely from the Context passed
// to each call.
type Model struct {
	skin ui.Skin
}

// New returns the footer renderer.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// View renders the footer to exactly width display cells, measured so a wide
// glyph cannot push it onto a second row, dropping hints from the right and keeping the last.
func (m Model) View(width int, ctx Context) string {
	prefix := m.prefix(ctx, width)
	hints := ui.FitHints(m.skin.KeyHintStyles(), ctx.Hints, width-lipgloss.Width(prefix)-len(partGap))

	// The prefix is dropped last: a footer that overflows wraps onto a second row and costs a line
	// of the log, which is worse than losing the revset from view.
	if hints == "" {
		return padTo(ui.FitHints(m.skin.KeyHintStyles(), ctx.Hints, width), width)
	}

	return padTo(prefix+partGap+hints, width)
}

// prefixBudget is the fraction of the line the left side may take before the undo segment is
// dropped, so the hint list always has room for more than its last entry.
const prefixBudget = 2

func (m Model) prefix(ctx Context, width int) string {
	main := m.state(ctx)
	if ctx.Undo == "" {
		return main
	}

	// jj writes long descriptions into its operation log, so the segment is trimmed to what is left
	// of the budget rather than dropped whole, which is what always happened before.
	room := width/prefixBudget - lipgloss.Width(main) - len(partGap) - len(undoLabel)
	if room < minUndoWidth {
		return main
	}

	undo := m.skin.Muted.Render(undoLabel + table.Truncate(ctx.Undo, room))

	if main == "" {
		return undo
	}

	return main + partGap + undo
}

const (
	partGap   = "  "
	undoLabel = "u undoes "
	// Below this the segment costs the hints their room and says nothing.
	minUndoWidth = 12
)

func (m Model) state(ctx Context) string {
	if ctx.Message != "" {
		return m.skin.Subtitle.Render(ctx.Message)
	}

	var parts []string

	if bookmarks := m.bookmarks(ctx.Bookmarks); bookmarks != "" {
		parts = append(parts, bookmarks)
	}

	if ctx.Revset != "" {
		parts = append(parts, m.skin.Muted.Render(ctx.Revset))
	}

	return strings.Join(parts, partGap)
}

func (m Model) bookmarks(bookmarks []Bookmark) string {
	if len(bookmarks) == 0 {
		return ""
	}

	rendered := make([]string, len(bookmarks))

	for i, b := range bookmarks {
		style := m.skin.Bookmark
		if b.AtWorkingCopy {
			style = m.skin.WorkingCopy
		}

		rendered[i] = style.Render(b.Name)
	}

	return strings.Join(rendered, " ")
}

// padTo pads s to exactly width cells and never truncates, since cutting an
// already-styled string risks cutting inside its escape sequence.
func padTo(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}

	return s
}
