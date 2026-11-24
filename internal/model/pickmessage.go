package model

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kyleking/jojo/internal/jj"
)

// A picker's -m is jj's own: the message the command writes, typed on the footer's first line where
// the command line stands, so the graph stays in view while it is written. Enter keeps it, Esc leaves
// the flag as it was, and -m again drops the message.

// toggleMessage is - m: a message already written is dropped, else the footer's prompt opens.
func (m Model) toggleMessage() (Model, tea.Cmd) {
	if m.pick.message != "" {
		m.pick.message = ""
		cmd := m.changed()

		return m, cmd
	}

	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "the message, on one line"
	cmd := input.Focus()
	m.pick.input = &input

	return m, cmd
}

// typingMessage reports whether the footer's prompt has the keys.
func (m Model) typingMessage() bool { return m.pick.active && m.pick.input != nil }

// messageKey is a key while the footer's prompt is open.
func (m Model) messageKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		m.pick.message = strings.TrimSpace(m.pick.input.Value())
		m.pick.input = nil
		cmd := m.changed()

		return m, cmd
	case keyEsc:
		m.pick.input = nil

		return m, m.say("message unchanged")
	}

	input, cmd := m.pick.input.Update(msg)
	m.pick.input = &input

	return m, cmd
}

// messagePrompt is the footer's first line while the prompt is open.
func (m Model) messagePrompt() string {
	head := m.skin.Accent.Render("-m ")
	input := *m.pick.input
	input.SetWidth(max(1, m.width-lipgloss.Width(head)-1))

	return padLine(head+input.View(), m.width)
}

// squashPlaced reports whether a squash makes a new commit (-o, -A, -B) rather than going --into.
func (p *pickState) squashPlaced() bool { return p.verb == pickSquash && !p.flags.On("t") }

// squashArgs is jj squash as the picker's flags spell it: into the destination, or into a new
// commit placed against it, with the message the flags chose.
func (p *pickState) squashArgs(forPreview bool) []string {
	if !p.squashPlaced() {
		args := jj.SquashIntoArgs(jj.SquashOptions{
			Sources: p.sources, Into: p.dest, KeepEmptied: p.flags.On("k"),
			Combine: (!p.flags.On("u") || p.message != "") && !forPreview,
		})

		return withMessage(args, p.message)
	}

	args := []string{"squash"}
	for _, from := range p.sources {
		args = append(args, "--from", from)
	}

	args = append(args, jj.PlaceOptions{Destination: []string{p.dest}, Mode: p.placement()}.Args()...)
	if p.flags.On("k") {
		args = append(args, "--keep-emptied")
	}

	return withMessage(args, p.message)
}

// withMessage is args with -m message when there is one.
func withMessage(args []string, message string) []string {
	if message == "" {
		return args
	}

	return append(slices.Clone(args), "-m", message)
}

// needsTerminal reports whether the picker's command may open jj's editor for a message: a squash
// --into that combines the two descriptions, or a squash of several into a new commit, each with no
// -m to settle it.
func (p *pickState) needsTerminal() bool {
	if p.verb != pickSquash || p.message != "" {
		return false
	}

	if p.squashPlaced() {
		return len(p.sources) > 1
	}

	return !p.flags.On("u")
}
