// Package logview composes and renders the log: one row per revision, scanned
// rather than read.
package logview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/glyph"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// Column keys. A row is scanned for the change id, the bookmarks, and the
// description, so those hold their width while the metadata collapses.
const (
	ColChange    = "change"
	ColBookmarks = "bookmarks"
	ColDesc      = "desc"
	ColSize      = "size"
	ColAuthor    = "author"
	ColAge       = "age"
)

// CheckMark flags a row picked for a batch action.
const CheckMark = "●"

// NoDescription stands in for a revision nobody has described yet.
const NoDescription = "(no description set)"

// Column widths, in display cells. The change id is fixed because the log
// template pins jj's shortest-prefix length; the rest bound how much of the row
// metadata may take before it collapses.
const (
	changeIDWidth = 8
	bookmarksMin  = 6
	bookmarksMax  = 24
	descMin       = 20
	descWeight    = 4
	sizeMin       = 12
	// The widest compact size, "999f +9.9k -9.9k", fits sizeMax whole.
	sizeMax   = 16
	authorMin = 8
	authorMax = 14
	ageMin    = 4
	ageMax    = 6
)

// Collapse order: the lowest positive priority goes first, so the row gives up
// its metadata before it gives up what is scanned for.
const (
	prioAuthor = iota + 1
	prioAge
	prioSize
	prioBookmarks
)

// Columns orders the row and sets what collapse gives up first. Priority zero
// never collapses, so the description and the change id survive any width.
func Columns() []table.Column { return columnsFor(0, 0) }

// columnsFor is Columns with the bookmark column at least as wide as the widest bookmark text in the
// log, up to its cap: a narrow log (the dock beside it) gives up the author, the age, and the size
// before it cuts a bookmark name to "ttl-k…", since a cut name names nothing. The size column is as
// wide as the widest size, up to its cap, so a big change's counts are not cut at any width.
func columnsFor(bookmarks, sizes int) []table.Column {
	least := min(max(bookmarks, bookmarksMin), bookmarksMax)
	sized := min(max(sizes, sizeMin), sizeMax)

	return []table.Column{
		{Key: ColChange, Title: "change", Min: changeIDWidth, Max: changeIDWidth},
		{Key: ColBookmarks, Title: "bookmarks", Min: least, Max: bookmarksMax, Priority: prioBookmarks},
		{Key: ColDesc, Title: "description", Min: descMin, Weight: descWeight},
		{Key: ColSize, Title: "size", Min: sized, Max: sizeMax, Priority: prioSize, Align: table.AlignRight},
		{Key: ColAuthor, Title: "author", Min: authorMin, Max: authorMax, Priority: prioAuthor, Trim: table.TrimRight},
		{Key: ColAge, Title: "age", Min: ageMin, Max: ageMax, Priority: prioAge, Align: table.AlignRight},
	}
}

// Cells renders one revision's column text, unstyled and untruncated.
func Cells(r jj.Revision) map[string]string {
	return map[string]string{
		ColChange:    r.ChangeID,
		ColBookmarks: glyph.Settle(strings.Join(r.Bookmarks, " ")),
		ColDesc:      description(r),
		ColSize:      size(r),
		ColAuthor:    authorHandle(r.AuthorEmail),
		ColAge:       compactAge(r.Age),
	}
}

// description is the row's description, settled so an emoji a terminal measures differently cannot
// move the columns after it.
func description(r jj.Revision) string {
	if r.Description != "" {
		return glyph.Settle(r.Description)
	}

	if r.Empty {
		return "(empty)"
	}

	return NoDescription
}

// size is what decides whether a revision is worth opening, and no other jj
// front end puts it on the row.
func size(r jj.Revision) string {
	if r.Files == 0 {
		return ""
	}

	return compact(r.Files) + "f +" + compact(r.Added) + " -" + compact(r.Removed)
}

// compact writes a count in at most four cells, the way second-look does: 999, 1.8k, 18k, 180k, 1.8M.
// A full count of a big refactor was cut to "92f +1837 -…" at any width.
func compact(n int) string {
	const (
		thousand = 1000
		million  = thousand * thousand
		tenths   = 10
	)

	switch {
	case n < thousand:
		return strconv.Itoa(n)
	case n < tenths*thousand-thousand/tenths/2:
		return decimal(n, thousand) + "k"
	case n < million-thousand/2:
		return strconv.Itoa((n+thousand/2)/thousand) + "k"
	case n < tenths*million-million/tenths/2:
		return decimal(n, million) + "M"
	}

	return strconv.Itoa((n+million/2)/million) + "M"
}

// decimal is n/unit to one place, rounded, without a trailing ".0".
func decimal(n, unit int) string {
	const tenths = 10

	t := (n*tenths + unit/2) / unit
	if t%tenths == 0 {
		return strconv.Itoa(t / tenths)
	}

	return strconv.Itoa(t/tenths) + "." + strconv.Itoa(t%tenths)
}

// ageUnits abbreviates jj's prose ago() to its first letter, so "3 months ago"
// costs four cells instead of twelve.
var ageUnits = map[string]string{
	"second": "s", "seconds": "s",
	"minute": "m", "minutes": "m",
	"hour": "h", "hours": "h",
	"day": "d", "days": "d",
	"week": "w", "weeks": "w",
	"month": "mo", "months": "mo",
	"year": "y", "years": "y",
}

func compactAge(ago string) string {
	count, rest, found := strings.Cut(ago, " ")
	if !found {
		return ago
	}

	unit, _, _ := strings.Cut(rest, " ")

	short, known := ageUnits[unit]
	if !known {
		return ago
	}

	return count + short
}

// authorHandle keeps the local part, because a full address costs a third of
// the row to say what the handle already says.
func authorHandle(email string) string {
	local, _, found := strings.Cut(email, "@")
	if !found {
		return email
	}

	return local
}

// Fit resolves the row's columns into the width left after the graph gutter.
// A dense log gets FitCompact, so a collapsed column costs no cells to announce
// itself on every row.
func Fit(width, graphWidth int) table.Layout {
	return table.FitCompact(Columns(), width-graphWidth-table.Gutter)
}

// FitRows is Fit for these rows, the bookmark and size columns kept as wide as their widest text.
func FitRows(width, graphWidth int, rows []jj.LogRow) table.Layout {
	return table.FitCompact(columnsFor(BookmarkWidth(rows), SizeWidth(rows)), width-graphWidth-table.Gutter)
}

// SizeWidth is the widest size text among rows, in cells: a size is ASCII, so its length.
func SizeWidth(rows []jj.LogRow) int {
	widest := 0

	for _, row := range rows {
		if row.Revision != nil && row.Revision.Files > 0 {
			widest = max(widest, len(size(*row.Revision)))
		}
	}

	return widest
}

// BookmarkWidth is the widest bookmark text among rows, in display cells.
func BookmarkWidth(rows []jj.LogRow) int {
	widest := 0

	for _, row := range rows {
		if row.Revision == nil || len(row.Revision.Bookmarks) == 0 {
			continue
		}

		widest = max(widest, lipgloss.Width(strings.Join(row.Revision.Bookmarks, " ")))
	}

	return widest
}

// GraphWidth is the widest graph prefix among the revision rows, which every
// revision pads to so the columns start at one offset. Graph-only lines are
// excluded because jj writes prose on some of them ("(elided revisions)"),
// which would drag the whole table right.
func GraphWidth(rows []jj.LogRow) int {
	widest := 0

	for _, row := range rows {
		if row.Revision == nil {
			continue
		}

		if w := lipgloss.Width(strings.TrimRight(row.Revision.Graph, " ")); w > widest {
			widest = w
		}
	}

	return widest
}

// StylesFrom maps the app skin onto the log's roles, so the model and the tests
// render the same log.
func StylesFrom(s ui.Skin) Styles {
	return Styles{
		Graph:     s.Muted,
		Change:    s.ChangeIDPrefix,
		Bookmark:  s.Bookmark,
		Desc:      s.Body,
		Added:     s.Added,
		Removed:   s.Removed,
		Files:     s.Muted,
		Author:    s.Muted,
		Age:       s.Muted,
		Selected:  s.Selection,
		Immutable: s.Immutable,
		Conflict:  s.Conflicted,
		Divergent: s.Divergent,
		Working:   s.WorkingCopy,
		Checked:   s.Accent,
		Header:    s.Heading,
		Match:     s.Band,
		Mark:      s.Accent.Bold(true),
		MarkMuted: s.Muted.Italic(true),
		Unread:    s.Accent,
		Label:     s.Accent.Bold(true).Reverse(true),
		Select:    s.Skin.Selected,
	}
}
