// Package prompt is the text prompt docked under the log, used for jj describe and anything else
// that needs a string from the user. Every prompt keeps one contract: enter submits, ctrl+j (or
// alt+enter) breaks the line where the value can hold one, esc cancels, and the footer says so.
package prompt

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

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
// submits, ctrl+j or alt+enter breaks the line, and alt+e asks for $EDITOR. It is the prompt for a
// description, whose body and trailers a single line would drop. The cursor starts at the end, so
// what is typed adds to the text rather than landing in front of it.
func (m *Model) OpenText(title, initial string) tea.Cmd {
	m.title = title
	m.open, m.multiline, m.editor = true, true, true
	m.editText().SetHeight(maxTextRows)
	m.editText().SetValue(initial)
	m.editText().MoveToEnd()

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

// ruleRows is the row the docked prompt's rule and title take above the field.
const ruleRows = 1

// minTextRows keeps room to see a line break land in a one-line description.
const minTextRows = 3

// maxTextRows bounds the multi-line field, since a description is read a screen at a time anyway.
const maxTextRows = 12

// shareOfScreen is the most of the screen the docked prompt takes, a part in this many, so the log
// it describes stays in view above it.
const shareOfScreen = 2

// Rows is how many rows the docked prompt takes in a screen height tall: its rule, and the field,
// which grows with the lines typed into it up to its cap and half the screen. Closed, it takes none.
func (m Model) Rows(height int) int {
	if !m.open {
		return 0
	}

	return ruleRows + m.fieldRows(height)
}

func (m Model) fieldRows(height int) int {
	if !m.multiline {
		return 1
	}

	rows := min(maxTextRows, max(minTextRows, m.text.LineCount()+1))

	return max(1, min(rows, height/shareOfScreen-ruleRows))
}

func (m Model) field(width, rows int) string {
	if !m.multiline {
		input := *m.input
		input.SetWidth(max(width-lipgloss.Width(input.Prompt)-cursorCellWidth, minFieldWidth))
		// SetCursor scrolls the field to the new width, so a long value shows the end the cursor is at.
		input.SetCursor(input.Position())

		return table.Truncate(input.View(), width)
	}

	text := *m.text
	text.SetWidth(max(width, minFieldWidth))
	text.SetHeight(rows)

	return text.View()
}

// Hints are the prompt's keys, for the footer under it: the contract every prompt keeps.
func (m Model) Hints() []keyhint.Hint {
	hints := []keyhint.Hint{{Key: "enter", What: "submit"}}
	if m.multiline {
		hints = append(hints, keyhint.Hint{Key: "ctrl+j", What: "new line"})
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

// fieldIndent sets the field in from the edge, under the rule's title.
const fieldIndent = " "

// Docked renders the prompt as a panel width wide, Rows(height) tall, for the bottom of a screen
// height tall: a rule naming what it is for, then the field. The log stays in view above it, so what
// is being described is on screen while it is written.
func (m Model) Docked(width, height int) string {
	if width < 1 || !m.open {
		return ""
	}

	rows := m.fieldRows(height)
	title := table.Truncate(m.title, max(0, width-len("─  ─")))
	rule := m.skin.Muted.Render("─ ") + m.skin.Title.Render(title) + " "
	rule += m.skin.Muted.Render(strings.Repeat("─", max(0, width-lipgloss.Width(rule))))

	lines := []string{rule}

	for _, line := range strings.Split(m.field(max(1, width-len(fieldIndent)), rows), "\n") {
		lines = append(lines, pad(fieldIndent+line, width))
	}

	for len(lines) < ruleRows+rows {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines[:ruleRows+rows], "\n")
}

// View renders the docked prompt at the bottom of a width by height frame, its keys on the row under
// it, for a caller that has no footer of its own to put them in.
func (m Model) View(width, height int) string {
	if width < minViewWidth || height < m.Rows(height)+1 {
		return ui.TooSmall(width, height)
	}

	panel := m.Docked(width, height)
	hint := pad(ui.FitHints(m.skin.FooterHintStyles(), m.Hints(), width), width)
	used := m.Rows(height) + 1

	lines := make([]string, 0, height)
	for range max(0, height-used) {
		lines = append(lines, strings.Repeat(" ", width))
	}

	lines = append(lines, strings.Split(panel, "\n")...)
	lines = append(lines, hint)

	return strings.Join(lines[max(0, len(lines)-height):], "\n")
}

// minViewWidth is the narrowest frame View draws a prompt in: the rule's corner and a cell of field.
const minViewWidth = 4

// pad fills line to width cells, never cutting a styled string.
func pad(line string, width int) string {
	if gap := width - lipgloss.Width(line); gap > 0 {
		return line + strings.Repeat(" ", gap)
	}

	return line
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
