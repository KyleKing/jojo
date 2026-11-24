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
		cmd := m.followDock(m.log.Selected())

		return m, cmd, true
	case search.SteppedMsg:
		m.find.SetMatches(m.log.StepMatch(msg.Delta))
		cmd := m.followDock(m.log.Selected())

		return m, cmd, true
	case search.AcceptedMsg:
		m.resize()

		return m, nil, true
	case search.CancelledMsg:
		m.log.SetSearch("")
		m.resize()

		return m, nil, true
	case palette.RunMsg:
		m.palette.Close()
		next, cmd := m.runAction(msg.Action)

		return next, cmd, true
	case palette.CancelledMsg:
		m.palette.Close()

		return m, nil, true
	}

	return m, nil, false
}

// paletteCommands is the keymap itself, every action the log runs whether a key is bound to it or
// not, so an entry cannot drift from the binding it names and a palette-only verb (arrange) is
// reachable.
func (m Model) paletteCommands() []palette.Command {
	bindings := m.keys.Runnable()
	commands := make([]palette.Command, 0, len(bindings))

	for _, b := range bindings {
		commands = append(commands, palette.Command{
			Key: b.Label(), Action: b.Action, Name: b.What, What: help.Cmd(b.Action),
		})
	}

	return commands
}
