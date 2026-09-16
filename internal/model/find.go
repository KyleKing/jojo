package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/palette"
	"github.com/kyleking/jojo/internal/components/search"
)

// findRows is the height the search bar takes out of the log while it is open.
const findRows = 1

// handleFindMsg covers the search bar and the command palette, which act on the log rather than jj.
func (m Model) handleFindMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case search.ChangedMsg:
		m.find.SetMatches(m.log.SetSearch(msg.Pattern))

		return m, nil, true
	case search.SteppedMsg:
		m.find.SetMatches(m.log.StepMatch(msg.Delta))

		return m, nil, true
	case search.AcceptedMsg:
		m.resize()

		return m, nil, true
	case search.CancelledMsg:
		m.log.SetSearch("")
		m.resize()

		return m, nil, true
	case palette.RunMsg:
		next, cmd := m.dispatch(msg.Key)

		return next, cmd, true
	case palette.CancelledMsg:
		m.palette.Close()

		return m, nil, true
	}

	return m, nil, false
}

// dispatch runs a key the palette picked through the same handlers the keypress itself reaches, so
// the palette can never do something a key cannot.
func (m Model) dispatch(key string) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleRootKey(key); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleActionKey(key); handled {
		return next, cmd
	}

	return m, m.say(key + " does nothing here")
}

// paletteCommands is the keymap itself, filtered to what a single key runs, so an entry cannot drift
// from the binding it names.
func paletteCommands() []palette.Command {
	bindings := help.Runnable()
	commands := make([]palette.Command, 0, len(bindings))

	for _, b := range bindings {
		commands = append(commands, palette.Command{Key: b.Run, Name: b.What, What: b.Cmd})
	}

	return commands
}
