// Package readmark names the hunks of a change by what they say, so read marks can follow a hunk
// through a rebase and drop it once its text changes, and sums them into the strip an outline
// chapter carries.
//
// The ids are second-look's (internal/seen): sha256 over the file path, a NUL, and every line of the
// hunk as its kind byte and text, line numbers left out. Marks stored by the shared aragonite/seen
// package, once jojo reads it, will then name the same hunks this package does.
package readmark

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strings"

	"github.com/kyleking/aragonite/diff"
)

// Hunk is one hunk of a diff: the file it belongs to, its 1-based position in that file, and its id.
type Hunk struct {
	Path string
	ID   string
	Hunk int
}

// Hunks is every hunk of a git-format diff in patch order. A file that carries no hunk, such as a
// rename or a binary payload, has nothing to read and is absent.
func Hunks(gitDiff string) []Hunk {
	if gitDiff == "" {
		return nil
	}

	d := diff.Parse([]byte(gitDiff))

	var out []Hunk

	h := sha256.New()

	for i := range d.Files {
		path := filePath(&d.Files[i])
		last := 0

		begin(h, path)

		for _, l := range d.Files[i].Lines {
			if l.Hunk != last {
				if last != 0 {
					out = append(out, Hunk{Path: path, Hunk: last, ID: hex.EncodeToString(h.Sum(nil))})
					begin(h, path)
				}

				last = l.Hunk
			}

			h.Write([]byte{l.Kind})
			h.Write([]byte(l.Text))
			h.Write([]byte{'\n'})
		}

		if last != 0 {
			out = append(out, Hunk{Path: path, Hunk: last, ID: hex.EncodeToString(h.Sum(nil))})
		}
	}

	return out
}

func begin(h hash.Hash, path string) {
	h.Reset()
	h.Write([]byte(path))
	h.Write([]byte{0})
}

func filePath(f *diff.File) string {
	if f.NewPath != "" {
		return f.NewPath
	}

	return f.OldPath
}

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
