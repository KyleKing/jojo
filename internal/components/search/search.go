// Package search is the incremental search bar behind the log's "/" key: it
// owns the pattern text and reports every edit and step so the log can move
// its own cursor.
package search

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// ChangedMsg is sent on every edit to the pattern, since the search is
// incremental.
type ChangedMsg struct {
	Pattern string
}

// SteppedMsg is sent when the bar moves between matches: 1 for next, -1 for
// previous.
type SteppedMsg struct {
	Delta int
}

// AcceptedMsg is sent when Enter closes the bar, leaving the cursor where the
// search put it.
type AcceptedMsg struct{}

// CancelledMsg is sent when Esc closes the bar without accepting.
type CancelledMsg struct{}

// Model is the search bar's state: the pattern and the match position the
// parent last reported.
type Model struct {
	skin    ui.Skin
	pattern string
	matches int
	index   int
	open    bool
}

// New returns a closed search bar drawn in skin.
func New(skin ui.Skin) Model {
	return Model{skin: skin}
}

// Open shows the bar with the pattern cleared.
//
//nolint:unparam // matches the Open shape other overlays use; nothing here needs a command yet.
func (m *Model) Open() tea.Cmd {
	m.pattern = ""
	m.matches = 0
	m.index = 0
	m.open = true

	return nil
}

// Close hides the bar.
func (m *Model) Close() {
	m.open = false
}

// IsOpen reports whether the bar is taking keys.
func (m Model) IsOpen() bool {
	return m.open
}

// SetMatches records the match count and the cursor's 1-based position in it,
// as reported by the log after each change.
func (m *Model) SetMatches(count, index int) {
	m.matches = count
	m.index = index
}

// Update handles one message. Key handling only runs while the bar is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	return m.handleKey(key)
}

func (m *Model) handleKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		m.Close()

		return func() tea.Msg { return AcceptedMsg{} }
	case "down", "ctrl+n":
		return func() tea.Msg { return SteppedMsg{Delta: 1} }
	case "up", "ctrl+p":
		return func() tea.Msg { return SteppedMsg{Delta: -1} }
	case "backspace":
		return m.setPattern(trimLastRune(m.pattern))
	}

	if key.Text != "" {
		return m.setPattern(m.pattern + key.Text)
	}

	return nil
}

func (m *Model) setPattern(s string) tea.Cmd {
	m.pattern = s
	pattern := s

	return func() tea.Msg { return ChangedMsg{Pattern: pattern} }
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}

	return string(runes[:len(runes)-1])
}

// minGap is the smallest gap left between the pattern and the match position
// before the position is dropped rather than overlapping the text.
const minGap = 1

// View renders one line: the pattern on the left, and the match position or
// "no match" right-aligned.
func (m Model) View(width int) string {
	left := table.Truncate("/"+m.pattern, width)

	status, style := m.status()
	if status == "" {
		return left
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(status)
	if gap < minGap {
		return left
	}

	return left + strings.Repeat(" ", gap) + style.Render(status)
}

func (m Model) status() (string, lipgloss.Style) {
	switch {
	case m.pattern == "":
		return "", lipgloss.Style{}
	case m.matches == 0:
		return "no match", m.skin.Error
	default:
		return fmt.Sprintf("%d/%d", m.index, m.matches), m.skin.Body
	}
}
