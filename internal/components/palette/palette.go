// Package palette is the command palette behind the ":" key: it lists jojo's
// own actions by name and, on Enter, hands back the same key the root model's
// binding already dispatches on, so the palette can never do something a key
// cannot.
package palette

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/listview"
	"github.com/kyleking/jojo/internal/ui"
)

// Command is one action the palette can run: what the root model dispatches on, the key to show for
// it (empty for a command no key runs), the palette's own searchable name, and a one-line
// description.
type Command struct {
	ID   string
	Key  string
	Name string
	What string
}

// RunMsg is sent when Enter confirms a command, naming what the root model dispatches on.
type RunMsg struct {
	ID string
}

// CancelledMsg is sent when Esc dismisses the palette without running one.
type CancelledMsg struct{}

// Model is the palette's state: the commands it was opened with, the live
// filter over them, and the cursor into the filtered list.
type Model struct {
	skin     ui.Skin
	filter   string
	commands []Command
	list     listview.Model[Command]
}

// New returns a closed palette drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin, list: listview.New[Command](skin)}
}

// Open shows the palette over commands, in the order given, with the filter
// cleared and the cursor on the first entry.
//
//nolint:unparam // matches the Open shape other overlays use; nothing here needs a command yet.
func (m *Model) Open(commands []Command) tea.Cmd {
	m.filter = ""
	m.commands = commands
	m.recomputeMatches()

	return nil
}

// Close hides the palette.
func (m *Model) Close() { m.list.Close() }

// IsOpen reports whether the palette is taking keys.
func (m Model) IsOpen() bool { return m.list.IsOpen() }

// Update handles one message. Key handling only runs while the palette is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.list.IsOpen() {
		return nil
	}

	return m.handleKey(key)
}

// handleKey reserves every other key for the filter, unlike destpicker's list,
// since a command's name is the only way to find it and "j" or "k" are
// letters a name can start with.
func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	name := key.String()

	switch name {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		return m.run()
	case "up", "down", "ctrl+n", "ctrl+p", "pgup", "pgdown", "home", "end":
		m.list.MoveKey(name)

		return nil
	case "backspace":
		m.setFilter(trimLastRune(m.filter))

		return nil
	}

	if key.Text != "" {
		m.setFilter(m.filter + key.Text)
	}

	return nil
}

func (m *Model) run() tea.Cmd {
	cmd, ok := m.list.Selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return RunMsg{ID: cmd.ID} }
}

func (m *Model) setFilter(s string) {
	m.filter = s
	m.recomputeMatches()
}

func (m *Model) recomputeMatches() {
	pattern := m.filter

	matches := filter.Keep(m.commands, func(c Command) bool {
		return matchesFilter(c, pattern)
	})

	m.list.Open("command :"+m.filter, matches, 0)
}

func matchesFilter(c Command, pattern string) bool {
	if pattern == "" {
		return true
	}

	return filter.Match(pattern, c.Name) || filter.Match(pattern, c.Key)
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}

	return string(runes[:len(runes)-1])
}

// View renders the title with the live filter, matching rows, a
// scroll-position footer when clipped, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	return m.list.View(width, height, m.row, []keyhint.Hint{
		{Key: "enter", What: "run"},
		{Key: "esc", What: "cancel"},
	})
}

func (m Model) row(c Command, selected bool) listview.Row {
	keyWidth, nameWidth := columnWidths(m.list.Items())

	style := m.skin.Body
	if selected {
		style = m.skin.Selected
	}

	return listview.Row{Text: rowText(c, keyWidth, nameWidth), Style: style}
}

// columnWidths measures the key and name columns across every match, not just
// the ones on screen, so the columns hold still as the window scrolls.
func columnWidths(commands []Command) (int, int) {
	keyWidth, nameWidth := 0, 0

	for _, c := range commands {
		keyWidth = max(keyWidth, lipgloss.Width(c.Key))
		nameWidth = max(nameWidth, lipgloss.Width(c.Name))
	}

	return keyWidth, nameWidth
}

func rowText(c Command, keyWidth, nameWidth int) string {
	return table.Pad(c.Key, keyWidth, table.AlignLeft) + "  " +
		table.Pad(c.Name, nameWidth, table.AlignLeft) + "  " + c.What
}
