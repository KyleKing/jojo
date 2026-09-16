// Package palette is the command palette behind the ":" key: it lists jojo's
// own actions by name and, on Enter, hands back the same key the root model's
// binding already dispatches on, so the palette can never do something a key
// cannot.
package palette

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/filter"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// Command is one action the palette can run: the key the root model already
// dispatches on, the palette's own searchable name, and a one-line
// description.
type Command struct {
	Key  string
	Name string
	What string
}

// RunMsg is sent when Enter confirms a command, naming the key its binding
// already handles.
type RunMsg struct {
	Key string
}

// CancelledMsg is sent when Esc dismisses the palette without running one.
type CancelledMsg struct{}

// Model is the palette's state: the commands it was opened with, the live
// filter over them, and the cursor into the filtered list.
type Model struct {
	skin     ui.Skin
	filter   string
	commands []Command
	matches  []Command
	cursor   int
	open     bool
}

// New returns a closed palette drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the palette over commands, in the order given, with the filter
// cleared and the cursor on the first entry.
//
//nolint:unparam // matches the Open shape other overlays use; nothing here needs a command yet.
func (m *Model) Open(commands []Command) tea.Cmd {
	m.filter = ""
	m.open = true
	m.commands = commands
	m.recomputeMatches()

	return nil
}

// Close hides the palette.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the palette is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the palette is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	return m.handleKey(key)
}

// handleKey reserves every other key for the filter, unlike destpicker's list,
// since a command's name is the only way to find it and "j" or "k" are
// letters a name can start with.
func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		return m.run()
	case "up", "ctrl+p":
		m.moveCursor(-1)

		return nil
	case "down", "ctrl+n":
		m.moveCursor(1)

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
	cmd, ok := m.selected()
	if !ok {
		return nil
	}

	m.Close()

	return func() tea.Msg { return RunMsg{Key: cmd.Key} }
}

func (m Model) selected() (Command, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return Command{}, false
	}

	return m.matches[m.cursor], true
}

// moveCursor clamps at either end instead of wrapping, since a palette the
// user is scanning top to bottom should stop rather than jump back to the
// other edge.
func (m *Model) moveCursor(delta int) {
	if len(m.matches) == 0 {
		return
	}

	m.cursor = clamp(m.cursor+delta, 0, len(m.matches)-1)
}

func (m *Model) setFilter(s string) {
	m.filter = s
	m.recomputeMatches()
}

func (m *Model) recomputeMatches() {
	pattern := m.filter

	m.matches = filter.Keep(m.commands, func(c Command) bool {
		return matchesFilter(c, pattern)
	})
	m.cursor = 0
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

// scrollCenterDivisor centers the cursor in the visible window rather than
// pinning it to an edge.
const scrollCenterDivisor = 2

// scrollWindow returns the half-open range of matches to render, keeping
// cursor inside it, when body rows are available for total items.
func scrollWindow(cursor, total, body int) (int, int) {
	if body <= 0 || total <= body {
		return 0, total
	}

	start := clamp(cursor-body/scrollCenterDivisor, 0, total-body)

	return start, start + body
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

// footerRows is how much of the frame a clipped list gives up to the
// scroll-position line.
const footerRows = 1

// View renders the title, filter, matching rows, a scroll-position footer
// when clipped, and a key hint line, centered in a width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	lines := []string{
		m.skin.Title.Render("command"),
		m.skin.Body.Render(":" + m.filter),
	}

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "enter", What: "run"},
		{Key: "esc", What: "cancel"},
	})

	body := max(1, contentHeight-len(lines)-1)
	clipped := len(m.matches) > body

	if clipped {
		body = max(1, body-footerRows)
	}

	start, end := scrollWindow(m.cursor, len(m.matches), body)
	keyWidth, nameWidth := columnWidths(m.matches)

	for i := start; i < end; i++ {
		lines = append(lines, m.renderRow(m.matches[i], i == m.cursor, keyWidth, nameWidth, contentWidth))
	}

	if clipped {
		lines = append(lines, m.scrollFooter(start, end, len(m.matches), contentWidth))
	}

	lines = append(lines, hint)

	return overlay.Center(strings.Join(lines, "\n"), width, height, styles)
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

func (m Model) renderRow(c Command, selected bool, keyWidth, nameWidth, width int) string {
	text := table.Truncate(rowText(c, keyWidth, nameWidth), width)

	if selected {
		return m.skin.Selected.Render(text)
	}

	return m.skin.Body.Render(text)
}

func rowText(c Command, keyWidth, nameWidth int) string {
	return table.Pad(c.Key, keyWidth, table.AlignLeft) + "  " +
		table.Pad(c.Name, nameWidth, table.AlignLeft) + "  " + c.What
}

func (m Model) scrollFooter(start, end, total, width int) string {
	text := fmt.Sprintf("%d-%d of %d", start+1, end, total)

	return m.skin.Subtitle.Render(table.Pad(text, width, table.AlignRight))
}
