// Package prompt is a single-line text prompt modal over the log, used for
// jj describe and anything else that needs one string from the user.
package prompt

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"

	"github.com/kyleking/jojo/internal/ui"
)

// SubmittedMsg is sent when Enter confirms the prompt. Value may be empty,
// since an empty jj description is a real state, not a rejected input.
type SubmittedMsg struct {
	Title string
	Value string
}

// CancelledMsg is sent when Esc dismisses the prompt without submitting.
type CancelledMsg struct{}

// Option configures a Model at construction.
type Option func(*Model)

// Model is the prompt's state: a title, the wrapped textinput, and whether
// the modal is currently shown.
type Model struct {
	skin  ui.Skin
	title string
	input textinput.Model
	open  bool
}

// New returns a closed prompt drawn in skin.
func New(skin ui.Skin, opts ...Option) Model {
	m := Model{skin: skin, input: textinput.New()}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Open shows the prompt with title above a field seeded with initial,
// cursor at the end, and gives it keyboard focus.
func (m *Model) Open(title, initial string) tea.Cmd {
	m.title = title
	m.open = true
	m.input.SetValue(initial)
	m.input.CursorEnd()

	return m.input.Focus()
}

// Close hides the prompt and blurs its field.
func (m *Model) Close() {
	m.open = false
	m.input.Blur()
}

// IsOpen reports whether the prompt is currently shown.
func (m Model) IsOpen() bool {
	return m.open
}

// Update handles one message. Key handling only runs while the prompt is open.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.open {
		return nil
	}

	switch keyMsg.String() {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		title, value := m.title, m.input.Value()
		m.Close()

		return func() tea.Msg { return SubmittedMsg{Title: title, Value: value} }
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(keyMsg)

	return cmd
}

// minFieldWidth is the field's floor so a very narrow frame still draws a
// usable input rather than clipping it to nothing.
const minFieldWidth = 1

// cursorCellWidth is the extra column textinput's View reserves for the
// virtual cursor sitting after the last character, which SetWidth's argument
// does not otherwise account for.
const cursorCellWidth = 1

// View renders the title, the field, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)

	input := m.input
	input.SetWidth(max(contentWidth-len([]rune(input.Prompt))-cursorCellWidth, minFieldWidth))

	hint := keyhint.Line(m.skin.KeyHintStyles(), []keyhint.Hint{
		{Key: "enter", What: "submit"},
		{Key: "esc", What: "cancel"},
	})

	body := lipgloss.JoinVertical(lipgloss.Left,
		m.skin.Title.Render(m.title),
		input.View(),
		hint,
	)

	return overlay.Center(body, width, height, styles)
}
