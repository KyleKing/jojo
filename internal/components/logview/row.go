// Package logview composes and renders the log: one row per revision, scanned
// rather than read.
package logview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
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
	sizeMax       = 14
	authorMin     = 8
	authorMax     = 14
	ageMin        = 4
	ageMax        = 6
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
func Columns() []table.Column {
	return []table.Column{
		{Key: ColChange, Title: "change", Min: changeIDWidth, Max: changeIDWidth},
		{Key: ColBookmarks, Title: "bookmarks", Min: bookmarksMin, Max: bookmarksMax, Priority: prioBookmarks},
		{Key: ColDesc, Title: "description", Min: descMin, Weight: descWeight},
		{Key: ColSize, Title: "size", Min: sizeMin, Max: sizeMax, Priority: prioSize, Align: table.AlignRight},
		{Key: ColAuthor, Title: "author", Min: authorMin, Max: authorMax, Priority: prioAuthor, Trim: table.TrimRight},
		{Key: ColAge, Title: "age", Min: ageMin, Max: ageMax, Priority: prioAge, Align: table.AlignRight},
	}
}

// Cells renders one revision's column text, unstyled and untruncated.
func Cells(r jj.Revision) map[string]string {
	return map[string]string{
		ColChange:    r.ChangeID,
		ColBookmarks: strings.Join(r.Bookmarks, " "),
		ColDesc:      description(r),
		ColSize:      size(r),
		ColAuthor:    authorHandle(r.AuthorEmail),
		ColAge:       compactAge(r.Age),
	}
}

func description(r jj.Revision) string {
	if r.Description != "" {
		return r.Description
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

	return strconv.Itoa(r.Files) + "f +" + strconv.Itoa(r.Added) + " -" + strconv.Itoa(r.Removed)
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
		Selected:  s.Selected,
		Immutable: s.Immutable,
		Conflict:  s.Conflicted,
		Working:   s.WorkingCopy,
		Checked:   s.Accent,
		Header:    s.Heading,
	}
}
