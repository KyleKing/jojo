// Package prompt is a single-line text prompt modal over the log, used for
// jj describe and anything else that needs one string from the user.
package prompt

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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

// EditorRequestedMsg is sent when alt+e asks for $EDITOR instead, carrying what has been typed so far
// so the editor opens on it rather than on what the prompt started with.
type EditorRequestedMsg struct {
	Title string
	Value string
}

// Option configures a Model at construction.
type Option func(*Model)

// Model is the prompt's state: a title, the wrapped textinput, and whether
// the modal is currently shown.
type Model struct {
	skin ui.Skin
	// input and text are held by pointer, since the root model is copied on every message and the
	// two fields are most of this pane; each edit swaps in a fresh copy (editInput, editText), so a
	// copy of the prompt never sees another's edits.
	input *textinput.Model
	// text is the multi-line field OpenText uses, for a value whose line breaks are part of it.
	text      *textarea.Model
	title     string
	open      bool
	multiline bool
	// editor offers alt+e, which only a caller that can hand the terminal to $EDITOR can act on.
	editor bool
}

// New returns a closed prompt drawn in skin.
func New(skin ui.Skin, opts ...Option) Model {
	text := textarea.New()
	text.ShowLineNumbers = false
	text.Prompt = ""

	input := textinput.New()
	m := Model{skin: skin, input: &input, text: &text}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// Open shows the prompt with title above a field seeded with initial,
// cursor at the end, and gives it keyboard focus.
func (m *Model) Open(title, initial string) tea.Cmd {
	m.title = title
	m.open, m.multiline, m.editor = true, false, false
	m.editInput().SetValue(initial)
	m.editInput().CursorEnd()

	return m.editInput().Focus()
}

// OpenText shows a multi-line field seeded with initial, whose line breaks survive the edit: Enter
// submits, alt+enter or ctrl+j breaks the line, and alt+e asks for $EDITOR. It is the prompt for a
// description, whose body and trailers a single line would drop.
func (m *Model) OpenText(title, initial string) tea.Cmd {
	m.title = title
	m.open, m.multiline, m.editor = true, true, true
	m.editText().SetHeight(maxTextRows)
	m.editText().SetValue(initial)
	m.editText().MoveToBegin()

	return m.editText().Focus()
}

// Close hides the prompt and blurs its field.
func (m *Model) Close() {
	m.open = false
	m.editInput().Blur()
	m.editText().Blur()
}

// Value is what the field holds.
func (m Model) Value() string {
	if m.multiline {
		return m.text.Value()
	}

	return m.input.Value()
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

	title, value := m.title, m.Value()

	switch keyMsg.String() {
	case "esc":
		m.Close()

		return func() tea.Msg { return CancelledMsg{} }
	case "enter":
		m.Close()

		return func() tea.Msg { return SubmittedMsg{Title: title, Value: value} }
	case "alt+e":
		if m.editor {
			m.Close()

			return func() tea.Msg { return EditorRequestedMsg{Title: title, Value: value} }
		}
	case "alt+enter", "ctrl+j":
		if m.multiline {
			m.editText().InsertString("\n")

			return nil
		}
	}

	var cmd tea.Cmd

	if m.multiline {
		var text textarea.Model
		text, cmd = m.text.Update(keyMsg)
		m.text = &text
	} else {
		var input textinput.Model
		input, cmd = m.input.Update(keyMsg)
		m.input = &input
	}

	return cmd
}

// minFieldWidth is the field's floor so a very narrow frame still draws a
// usable input rather than clipping it to nothing.
const minFieldWidth = 1

// chromeRows is what the title and the hint line take from a multi-line field.
const chromeRows = 2

// minTextRows keeps room to see a line break land in a one-line description.
const minTextRows = 3

// maxTextRows bounds the multi-line field, since a description is read a screen at a time anyway.
const maxTextRows = 12

func (m Model) field(width, height int) string {
	if !m.multiline {
		input := *m.input
		input.SetWidth(max(width-len([]rune(input.Prompt))-cursorCellWidth, minFieldWidth))

		return input.View()
	}

	text := *m.text
	text.SetWidth(max(width, minFieldWidth))
	text.SetHeight(max(1, min(height-chromeRows, maxTextRows, max(minTextRows, text.LineCount()+1))))

	return text.View()
}

func (m Model) hints() []keyhint.Hint {
	hints := []keyhint.Hint{{Key: "enter", What: "submit"}}
	if m.multiline {
		hints = append(hints, keyhint.Hint{Key: "alt+enter", What: "new line"})
	}

	if m.editor {
		hints = append(hints, keyhint.Hint{Key: "alt+e", What: "$EDITOR"})
	}

	return append(hints, keyhint.Hint{Key: "esc", What: "cancel"})
}

// cursorCellWidth is the extra column textinput's View reserves for the
// virtual cursor sitting after the last character, which SetWidth's argument
// does not otherwise account for.
const cursorCellWidth = 1

// View renders the title, the field, and a key hint line, centered in a
// width by height frame.
func (m Model) View(width, height int) string {
	styles := m.skin.OverlayStyles()
	contentWidth := overlay.ContentWidth(width, styles)
	contentHeight := overlay.ContentHeight(height, styles)

	if contentWidth < 1 || contentHeight < 1 {
		return ui.TooSmall(width, height)
	}

	hint := ui.FitHints(m.skin.KeyHintStyles(), m.hints(), contentWidth)

	body := strings.Join([]string{
		m.skin.Title.Render(m.title),
		m.field(contentWidth, contentHeight),
		hint,
	}, "\n")

	return overlay.Center(body, width, height, styles)
}

// editInput replaces the single-line field with a copy of its own and returns it to edit.
func (m *Model) editInput() *textinput.Model {
	input := *m.input
	m.input = &input

	return m.input
}

// editText replaces the multi-line field with a copy of its own and returns it to edit.
func (m *Model) editText() *textarea.Model {
	text := *m.text
	m.text = &text

	return m.text
}
