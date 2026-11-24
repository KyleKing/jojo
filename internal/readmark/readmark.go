// Package readmark draws how much of a change has been read: the strip an outline chapter carries
// and the unread count a log row and a chapter show. The marks themselves are aragonite/seen's, which
// names a hunk by what it says so a mark follows it through a rebase and drops once its text changes.
package readmark

import (
	"strconv"
	"strings"
)

// Strip is how much of a chapter has been read. Total is zero until the chapter's diff has been read
// once, which draws as a strip that says nothing rather than one that says "all unread".
type Strip struct {
	Read  int
	Total int
}

// Known reports whether the strip has hunks to count.
func (s Strip) Known() bool { return s.Total > 0 }

// stripCells is how wide the strip draws.
const stripCells = 4

// Width is the cells String draws in.
const Width = stripCells

// String draws the strip as filled and hollow cells in proportion, rounding a partly read chapter
// away from both ends so "some" never reads as "none" or "all", and dots while it is unknown.
func (s Strip) String() string {
	if !s.Known() {
		return strings.Repeat("·", stripCells)
	}

	filled := s.Read * stripCells / s.Total

	switch {
	case s.Read > 0 && filled == 0:
		filled = 1
	case s.Read < s.Total && filled == stripCells:
		filled = stripCells - 1
	}

	return strings.Repeat("▰", filled) + strings.Repeat("▱", stripCells-filled)
}

// Unread is how many hunks are left to read, 0 while the strip is unknown.
func (s Strip) Unread() int { return max(0, s.Total-s.Read) }

// UnreadGlyph leads an unread count, a hollow circle for what is still open.
const UnreadGlyph = "◌"

// Label is the unread count as a row draws it, "◌3", or "" when nothing is left or nothing is known.
func (s Strip) Label() string {
	if s.Unread() == 0 {
		return ""
	}

	return UnreadGlyph + strconv.Itoa(s.Unread())
}
