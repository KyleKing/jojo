package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// OnBackground lays an already-styled line on face's background, across every cell. Each styled
// part of the line ends in a reset, and wrapping the whole line in face.Render leaves the face in
// force only up to the first of those, so the face's opening sequence is put back after each one.
// It is how the row under the cursor stays highlighted past its first colored cell.
func OnBackground(line string, face lipgloss.Style) string {
	const probe = "\x00"

	open, _, ok := strings.Cut(face.Render(probe), probe)
	if !ok || open == "" {
		return line
	}

	for _, reset := range []string{"\x1b[0m", "\x1b[m"} {
		line = strings.ReplaceAll(line, reset, reset+open)
	}

	return open + line + "\x1b[m"
}
