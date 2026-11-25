// Package prompt is the text prompt docked under the log, used for jj describe and anything else
// that needs a string from the user. Its field is aragonite's vimtext, so the family's one text-entry
// contract holds in every prompt: enter saves, ctrl+j breaks the line where the value can hold one,
// and esc backs out one level (insert to normal mode, then normal mode cancels). The footer says so,
// and names the mode the field is in.
package prompt

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"
	"github.com/kyleking/aragonite/tui/vimtext"

	"github.com/kyleking/jojo/internal/ui"
)

// SubmittedMsg is sent when Enter confirms the prompt. Value may be empty,
// since an empty jj description is a real state, not a rejected input.
type SubmittedMsg struct {
	Title string
	Value string
}

// CancelledMsg is sent when esc from normal mode (from insert mode with the vim layer off) dismisses
// the prompt without submitting.
type CancelledMsg struct{}

// Option configures a Model at construction.
type Option func(*Model)

// WithVim turns the field's vim layer on or off, which config.toml's vim sets. It is on by default:
// the field still opens in insert mode, so someone who does not use vim just types. Off, esc cancels
// at once.
func WithVim(on bool) Option { return func(m *Model) { m.vim = on } }

// Model is the prompt's state: a title, the field, and whether it is shown. The field is held by
// pointer and is the vimtext.Field itself, so every copy of the root model edits the one field.
type Model struct {
	skin  ui.Skin
	field *vimtext.Field
	// reg is the unnamed register, shared by every prompt this model opens, so a word yanked in one
	// description can be put in the next.
	reg   *vimtext.Register
	title string
	// screen is the terminal's height, which bounds how tall a multi-line field may grow.
	screen int
	open   bool
	vim    bool
}

// New returns a closed prompt drawn in skin.
func New(skin ui.Skin, opts ...Option) Model {
	m := Model{skin: skin, reg: &vimtext.Register{}, vim: true}
	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// SetHeight is the terminal's height, so a multi-line field opened in it grows no taller than its
// share of the screen and the log it describes stays in view.
func (m *Model) SetHeight(height int) { m.screen = height }

// Open shows a one-line prompt with title above a field seeded with initial, cursor at the end.
//
//nolint:unparam // the panels' Open signature; nothing here needs a command yet.
func (m *Model) Open(title, initial string) tea.Cmd {
	m.show(title, initial, vimtext.WithOneLine())

	return nil
}

// OpenName is Open for a value with rules, such as a bookmark name: enter on a value check refuses
// says why in the footer and keeps the prompt open.
//
//nolint:unparam // the panels' Open signature; nothing here needs a command yet.
func (m *Model) OpenName(title, initial string, check func(string) error) tea.Cmd {
	m.show(title, initial, vimtext.WithOneLine(), vimtext.WithValidator(check))

	return nil
}

// OpenText shows a multi-line field seeded with initial, whose line breaks survive the edit: enter
// submits, ctrl+j breaks the line, and ctrl+e hands the draft to $EDITOR and takes it back. It is the
// prompt for a description, whose body and trailers a single line would drop. The cursor starts at
// the end, so what is typed adds to the text rather than landing in front of it.
//
//nolint:unparam // the panels' Open signature; nothing here needs a command yet.
func (m *Model) OpenText(title, initial string) tea.Cmd {
	most := m.maxRows()
	m.show(title, initial, vimtext.WithHeight(min(minTextRows, most), most), vimtext.WithEditor("jojo-*.txt"))

	return nil
}

func (m *Model) show(title, initial string, opts ...vimtext.Option) {
	m.title, m.open = title, true
	m.field = vimtext.New(initial, append([]vimtext.Option{
		vimtext.WithID(title),
		vimtext.WithVim(m.vim),
		vimtext.WithRegister(m.reg),
		vimtext.WithStyles(vimtext.StylesFor(&m.skin.Skin)),
	}, opts...)...)
}

// maxRows is the most text rows a multi-line field takes: its cap, and half the screen less the rule.
func (m *Model) maxRows() int {
	if m.screen <= 0 {
		return maxTextRows
	}

	return max(1, min(maxTextRows, m.screen/shareOfScreen-ruleRows))
}

// Close hides the prompt.
func (m *Model) Close() { m.open = false }

// Value is what the field holds.
func (m Model) Value() string {
	if m.field == nil {
		return ""
	}

	return m.field.Value()
}

// Mode is the vim mode the field is in.
func (m Model) Mode() vimtext.Mode {
	if m.field == nil {
		return vimtext.Insert
	}

	return m.field.Mode()
}

// IsOpen reports whether the prompt is currently shown.
func (m Model) IsOpen() bool {
	return m.open
}

// Update hands the field what it takes while the prompt is open: keys, a paste, and the text back
// from $EDITOR. A key that saves or cancels the field closes the prompt and answers SubmittedMsg or
// CancelledMsg. Every key reaches the field, so vim's u, a, x, and the rest edit the text and never
// act on the log; the field leaves ctrl+c alone, and the caller quits on it before the prompt sees it.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if !m.open || m.field == nil {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
	case vimtext.EditedMsg:
		if msg.ID != m.field.ID() {
			return nil
		}
	default:
		return nil
	}

	cmd := m.field.Update(msg)
	if !m.field.Done() {
		return cmd
	}

	title := m.title
	m.Close()

	return finish(title, cmd)
}

// finish answers the field's last message as the prompt's: its SubmitMsg as SubmittedMsg, its
// CancelMsg as CancelledMsg, and anything else as it was.
func finish(title string, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		switch msg := cmd().(type) {
		case vimtext.SubmitMsg:
			return SubmittedMsg{Title: title, Value: msg.Text}
		case vimtext.CancelMsg:
			return CancelledMsg{}
		case tea.BatchMsg:
			out := make(tea.BatchMsg, 0, len(msg))
			for _, c := range msg {
				out = append(out, finish(title, c))
			}

			return out
		default:
			return msg
		}
	}
}

// ruleRows is the row the docked prompt's rule and title take above the field.
const ruleRows = 1

// minTextRows keeps room to see a line break land in a one-line description.
const minTextRows = 3

// maxTextRows bounds the multi-line field, since a description is read a screen at a time anyway.
const maxTextRows = 12

// shareOfScreen is the most of the screen the docked prompt takes, a part in this many, so the log
// it describes stays in view above it.
const shareOfScreen = 2

// Rows is how many rows the docked prompt takes in a screen width by height: its rule, and the
// field, which grows with the lines typed into it up to its cap and half the screen. Closed, it takes
// none.
func (m Model) Rows(width, height int) int {
	if !m.open || m.field == nil {
		return 0
	}

	return ruleRows + m.fieldRows(width, height)
}

func (m Model) fieldRows(width, height int) int {
	return max(1, min(m.field.Height(fieldWidth(width)), height/shareOfScreen-ruleRows))
}

// fieldIndent sets the field in from the edge, under the rule's title.
const fieldIndent = " "

// fieldWidth is the cells the field gets in a panel width wide.
func fieldWidth(width int) int { return max(1, width-len(fieldIndent)) }

// Hints are the field's keys in the mode it is in, for a footer that draws them among its own.
func (m Model) Hints() []keyhint.Hint {
	if m.field == nil {
		return nil
	}

	return m.field.Hints()
}

// Footer is the row under the docked prompt, exactly width cells: the field's mode badge, then its
// keys, a refusal from a check, or the : line being typed.
func (m Model) Footer(width int) string {
	if m.field == nil || width < 1 {
		return strings.Repeat(" ", max(0, width))
	}

	return m.field.Footer(width)
}

// Docked renders the prompt as a panel width wide, Rows(width, height) tall, for the bottom of a
// screen height tall: a rule naming what it is for, then the field. The log stays in view above it,
// so what is being described is on screen while it is written.
func (m Model) Docked(width, height int) string {
	if width < 1 || !m.open || m.field == nil {
		return ""
	}

	rows := m.fieldRows(width, height)
	title := table.Truncate(m.title, max(0, width-len("─  ─")))
	rule := m.skin.Muted.Render("─ ") + m.skin.Title.Render(title) + " "
	rule += m.skin.Muted.Render(strings.Repeat("─", max(0, width-lipgloss.Width(rule))))

	lines := []string{rule}

	for _, line := range cursorWindow(strings.Split(m.field.View(fieldWidth(width)), "\n"), rows) {
		lines = append(lines, pad(fieldIndent+line, width))
	}

	for len(lines) < ruleRows+rows {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines[:ruleRows+rows], "\n")
}

// reverse is the attribute the field draws its cursor cell in.
const reverse = "\x1b[7m"

// cursorWindow is rows of the field's lines, the window holding the cursor. The field is as tall as
// the screen allowed when it opened, so it draws more rows than fit only after the screen shrank
// under it, and then the rows kept are the ones the cursor is on rather than the top ones.
func cursorWindow(lines []string, rows int) []string {
	if len(lines) <= rows {
		return lines
	}

	start := 0

	for i, line := range lines {
		if strings.Contains(line, reverse) {
			start = max(0, i-rows+1)

			break
		}
	}

	return lines[start : start+rows]
}

// View renders the docked prompt at the bottom of a width by height frame, its footer on the row
// under it, for a caller that has no footer of its own to put it in.
func (m Model) View(width, height int) string {
	if width < minViewWidth || height < m.Rows(width, height)+1 {
		return ui.TooSmall(width, height)
	}

	panel := m.Docked(width, height)
	used := m.Rows(width, height) + 1

	lines := make([]string, 0, height)
	for range max(0, height-used) {
		lines = append(lines, strings.Repeat(" ", width))
	}

	lines = append(lines, strings.Split(panel, "\n")...)
	lines = append(lines, m.Footer(width))

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
