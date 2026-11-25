package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/vimtext"

	"github.com/kyleking/jojo/internal/jj"
)

// A picker's -m is jj's own: the message the command writes, typed on the footer's first line where
// the command line stands, so the graph stays in view while it is written. Enter keeps it, Esc leaves
// the flag as it was, and -m again drops the message. The field is vimtext's, as every prompt's is, so
// esc first goes to normal mode and a second esc leaves.

// toggleMessage is - m: a message already written is dropped, else the footer's prompt opens.
func (m Model) toggleMessage() (Model, tea.Cmd) {
	if m.pick.message != "" {
		m.pick.message = ""
		cmd := m.changed()

		return m, cmd
	}

	m.pick.input = vimtext.New("",
		vimtext.WithOneLine(),
		vimtext.WithID(pickMessageID),
		vimtext.WithVim(!m.noVim),
		vimtext.WithPlaceholder("the message, on one line"),
		vimtext.WithStyles(vimtext.StylesFor(&m.skin.Skin)),
	)

	return m, nil
}

// pickMessageID names the footer's field in the messages it sends.
const pickMessageID = "-m"

// typingMessage reports whether the footer's prompt has the keys.
func (m Model) typingMessage() bool { return m.pick.active && m.pick.input != nil }

// messageKey is a key or a paste while the footer's prompt is open. Every key reaches the field, so
// vim's keys edit the message rather than the picker's flags; enter keeps it and esc from normal mode
// leaves the flag as it was.
func (m Model) messageKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	field := m.pick.input

	cmd := field.Update(msg)
	if !field.Done() {
		return m, cmd
	}

	m.pick.input = nil

	if text, saved := fieldResult(cmd); saved {
		m.pick.message = strings.TrimSpace(text)
		cmd := m.changed()

		return m, cmd
	}

	return m, m.say("message unchanged")
}

// fieldResult is what a field that has just finished answered: its text and true for a save, false
// for a cancel. The field's own message is a value it already holds, so reading it here costs nothing.
func fieldResult(cmd tea.Cmd) (string, bool) {
	if cmd == nil {
		return "", false
	}

	switch msg := cmd().(type) {
	case vimtext.SubmitMsg:
		return msg.Text, true
	case tea.BatchMsg:
		for _, c := range msg {
			if text, ok := fieldResult(c); ok {
				return text, true
			}
		}
	}

	return "", false
}

// messagePrompt is the footer's first line while the prompt is open.
func (m Model) messagePrompt() string {
	head := m.skin.Accent.Render("-m ")

	return padLine(head+m.pick.input.View(max(1, m.width-lipgloss.Width(head))), m.width)
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
