// Package textview is a read-only, scrollable overlay of plain text, used for the full output of a
// jj command that failed. The footer only has room for one line of it, and the line that explains a
// failure is not always the one jj printed first.
package textview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/ui"
)

// CancelledMsg is sent when the overlay is dismissed.
type CancelledMsg struct{}

// Model is the text being shown, one list item per line, so it scrolls like every other overlay.
type Model struct {
	skin ui.Skin
	list listview.Model[string]
}

// New returns a closed overlay drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[string](skin)}
}

// Open shows text under title, scrolled to its top.
//
//nolint:unparam // matches the other panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(title, text string) tea.Cmd {
	m.list.Open(title, strings.Split(strings.TrimRight(text, "\n"), "\n"), 0)

	return nil
}

// Close hides the overlay.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the overlay is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update scrolls on the shared movement keys and closes on esc, q, or the "!" that opened it.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	switch key.String() {
	case "esc", "q", "!", "enter":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	}

	m.list.MoveKey(key.String())

	return nil
}

// View renders the visible lines, the cursor line marked so a long output shows where it is.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "j/k", What: "scroll"},
		{Key: "esc", What: "close"},
	})
}

func (m Model) row(line string, selected bool) listview.Row {
	style := m.skin.Body
	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: line, Style: style}
}
