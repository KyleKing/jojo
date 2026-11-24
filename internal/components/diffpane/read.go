package diffpane

import (
	"slices"

	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/table"
)

// ReadGlyph leads a hunk label, or a file header, that has been read.
const ReadGlyph = "✓"

// ReadFunc reports whether hunk k (from 0) of the file at path has been read.
type ReadFunc func(path string, k int) bool

// SetRead decides which hunks draw as read: their label rows, and the header of a file whose every
// hunk is read, are drawn muted behind ReadGlyph. Nil draws none, which is the pane's default.
func (m *Model) SetRead(read ReadFunc) { m.read = read }

// HunkAt is where the cursor is in hunk terms: the path of the file it is in and the hunk holding
// it, counted from 0 within that file, or -1 on the file's own rows above its first hunk. Outside any
// file, on jj's prose or with nothing open, the last result is false.
func (m *Model) HunkAt() (string, int, bool) {
	if m.sheet == nil || !m.list.IsOpen() {
		return "", 0, false
	}

	cursor := m.list.Cursor()

	f := m.sheet.partAt(cursor)
	if f < 0 || m.sheet.parts[f].doc == nil || m.sheet.parts[f].path == "" {
		return "", 0, false
	}

	k := -1

	for _, row := range m.sheet.fileHunks(f) {
		if row > cursor {
			break
		}

		k++
	}

	return m.sheet.parts[f].path, k, true
}

// FileHunks is how many hunks the pane draws for path, 0 when it draws no such file.
func (m *Model) FileHunks(path string) int {
	if m.sheet == nil {
		return 0
	}

	f := m.sheet.pathPart(path)
	if f < 0 {
		return 0
	}

	return len(m.sheet.fileHunks(f))
}

// JumpHunk moves the cursor to the label of path's hunk k, or to the file's header for a negative k,
// reporting whether the pane draws it.
func (m *Model) JumpHunk(path string, k int) bool {
	if m.sheet == nil || !m.list.IsOpen() {
		return false
	}

	f := m.sheet.pathPart(path)
	if f < 0 {
		return false
	}

	if k < 0 {
		m.list.Jump(m.sheet.parts[f].at)

		return true
	}

	rows := m.sheet.fileHunks(f)
	if k >= len(rows) {
		return false
	}

	m.list.Jump(rows[k])

	return true
}

// marked is row i cut to width, drawn muted behind ReadGlyph when it is the label of a read hunk or
// the header of a file read whole.
func (m Model) marked(i, width int) string {
	line := m.sheet.row(i)
	if m.read == nil || !m.isRead(i) {
		return table.Truncate(line, width)
	}

	text := ReadGlyph + " " + ansi.Strip(line)

	return m.skin.Muted.Render(table.Truncate(text, width))
}

// isRead reports whether row i is a read hunk's label or a read file's header.
func (m Model) isRead(i int) bool {
	f := m.sheet.partAt(i)
	if f < 0 {
		return false
	}

	p := m.sheet.parts[f]
	if p.doc == nil || p.path == "" {
		return false
	}

	rows := m.sheet.fileHunks(f)
	if i == p.at {
		if len(rows) == 0 {
			return false
		}

		for k := range rows {
			if !m.read(p.path, k) {
				return false
			}
		}

		return true
	}

	k := slices.Index(rows, i)

	return k >= 0 && m.read(p.path, k)
}
