package listview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A list jumps the way jojo's log does: ' labels the rows on screen and typing a label lands there,
// and g, G, and a label each leave the row they jumped from on a jump list that ctrl+o walks back
// and ctrl+i forward.

// jumpListCap is how many jumps the list keeps, as vim's does.
const jumpListCap = 100

// labelWidth is the columns a label takes at the start of its row: two letters and a space.
const labelWidth = 3

// noteJump records that the cursor is about to jump away from where it is, which drops whatever a
// walk back had left to go forward to.
func (m *Model[T]) noteJump() {
	if n := len(m.jumps.back); n > 0 && m.jumps.back[n-1] == m.cursor {
		return
	}

	m.jumps.back = append(m.jumps.back, m.cursor)
	if len(m.jumps.back) > jumpListCap {
		m.jumps.back = m.jumps.back[len(m.jumps.back)-jumpListCap:]
	}

	m.jumps.forward = nil
}

// JumpBack walks the jump list back (ctrl+o), reporting whether the cursor moved.
func (m *Model[T]) JumpBack() bool { return m.walk(&m.jumps.back, &m.jumps.forward) }

// JumpForward walks it forward again (ctrl+i), reporting whether the cursor moved.
func (m *Model[T]) JumpForward() bool { return m.walk(&m.jumps.forward, &m.jumps.back) }

func (m *Model[T]) walk(from, to *[]int) bool {
	for len(*from) > 0 {
		at := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]

		if at == m.cursor || at >= len(m.items) {
			continue
		}

		*to = append(*to, m.cursor)
		m.Jump(at)

		return true
	}

	return false
}

// labelKeys are the letters labels are made of, home row first.
const labelKeys = "asdfghjklqwertyuiopzxcvbnm"

// Labels names n rows: single letters while they last, then pairs, so every label in one jump is the
// same length and no label is the start of another.
func Labels(n int) []string {
	keys := strings.Split(labelKeys, "")
	if n <= len(keys) {
		return keys[:n]
	}

	out := make([]string, 0, n)

	for _, a := range keys {
		for _, b := range keys {
			if len(out) == n {
				return out
			}

			out = append(out, a+b)
		}
	}

	return out
}

// StartLabels labels every row the last frame showed, reporting false when there was none.
func (m *Model[T]) StartLabels() bool {
	start, end := 0, 0
	if m.drawn != nil {
		start, end = m.drawn[0], min(m.drawn[1], len(m.items))
	}

	if end <= start {
		return false
	}

	m.labels, m.typed = map[string]int{}, ""

	for i, label := range Labels(end - start) {
		m.labels[label] = start + i
	}

	return true
}

// Labeling reports whether a label jump is waiting for its label.
func (m Model[T]) Labeling() bool { return m.labels != nil }

// LabelKey takes a key while the labels are up. A label's letters land the cursor there and answer
// true; a key no label starts with, or esc, puts them away, answering what was typed ("" for esc).
func (m *Model[T]) LabelKey(key string) (bool, string) {
	typed := m.typed + key

	if key == "esc" || len(key) != 1 {
		m.labels, m.typed = nil, ""

		return false, ""
	}

	if at, ok := m.labels[typed]; ok {
		m.labels, m.typed = nil, ""
		m.noteJump()
		m.Jump(at)

		return true, ""
	}

	for label := range m.labels {
		if strings.HasPrefix(label, typed) {
			m.typed = typed

			return false, ""
		}
	}

	m.labels, m.typed = nil, ""

	return false, typed
}

// labelRows writes each label over the start of its row, so the row keeps its width.
func (m Model[T]) labelRows(rows []string, start, end int) {
	if m.labels == nil {
		return
	}

	for label, at := range m.labels {
		if at < start || at >= end || at-start >= len(rows) {
			continue
		}

		row := rows[at-start]
		tag := m.skin.Accent.Render(label) + strings.Repeat(" ", max(0, labelWidth-len(label)))
		rows[at-start] = tag + ansi.TruncateLeft(row, labelWidth, "")
	}
}
