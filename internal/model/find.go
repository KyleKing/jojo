package model

import (
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

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
		next, cmd := m.dispatch(msg.ID)

		return next, cmd, true
	case palette.CancelledMsg:
		m.palette.Close()

		return m, nil, true
	}

	return m, nil, false
}

// revsetPrefix marks a palette entry that names a revset rather than a key. No key contains a
// space, so the two families cannot collide.
const revsetPrefix = "revset "

// dispatch runs what the palette picked through the same handlers the keypress itself reaches, so
// the palette can never do something a key cannot.
func (m Model) dispatch(id string) (tea.Model, tea.Cmd) {
	if expr, ok := strings.CutPrefix(id, revsetPrefix); ok {
		// The box is the one place the active revset is editable, so it has to show what just ran.
		m.revsetBox.SetValue(expr)

		return m, m.loadLog(expr)
	}

	if next, cmd, handled := m.handleRootKey(id); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleActionKey(id); handled {
		return next, cmd
	}

	return m, m.say(id + " does nothing here")
}

// paletteCommands is the keymap itself, filtered to what a single key runs, so an entry cannot drift
// from the binding it names, followed by the revsets the user saved.
func (m Model) paletteCommands() []palette.Command {
	bindings := m.keys.Runnable()
	commands := make([]palette.Command, 0, len(bindings)+len(m.saved))

	for _, b := range bindings {
		commands = append(commands, palette.Command{ID: b.Run, Key: b.Run, Name: b.What, What: b.Cmd})
	}

	for _, name := range slices.Sorted(maps.Keys(m.saved)) {
		expr := m.saved[name]
		commands = append(commands,
			palette.Command{ID: revsetPrefix + expr, Name: "revset: " + name, What: expr})
	}

	return commands
}
