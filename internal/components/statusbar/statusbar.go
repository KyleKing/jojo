// Package statusbar renders the one-line footer: the bookmarks near @, the
// active revset or a transient status message, and the key hints for what a
// reader can do right now.
package statusbar

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/ui"
)

// ellipsis marks a hint dropped for width, same glyph keyhint uses for a
// truncated word.
const ellipsis = "…"

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
	prefix := m.prefix(ctx)
	line := fitHints(prefix, ctx.Hints, m.skin.KeyHintStyles(), width)

	return padTo(line, width)
}

func (m Model) prefix(ctx Context) string {
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

	return strings.Join(parts, "  ")
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

// fitHints joins prefix and the rendered hints, dropping whole hints from the
// right until the result fits width, never cutting a hint's own escapes.
func fitHints(prefix string, hints []keyhint.Hint, styles keyhint.Styles, width int) string {
	rendered := make([]string, len(hints))
	for i, h := range hints {
		rendered[i] = keyhint.One(styles, h)
	}

	join := func(parts []string) string {
		line := strings.Join(parts, keyhint.Gap)

		switch {
		case prefix == "":
			return line
		case line == "":
			return prefix
		default:
			return prefix + "  " + line
		}
	}

	full := join(rendered)
	if lipgloss.Width(full) <= width || len(rendered) < 2 {
		return full
	}

	lead, last := rendered[:len(rendered)-1], rendered[len(rendered)-1]

	for n := len(lead) - 1; n >= 0; n-- {
		kept := make([]string, 0, n+2)
		kept = append(kept, lead[:n]...)
		kept = append(kept, ellipsis, last)

		if candidate := join(kept); lipgloss.Width(candidate) <= width {
			return candidate
		}
	}

	return join([]string{last})
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
