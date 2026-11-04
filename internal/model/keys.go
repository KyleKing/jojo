package model

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		return m.handleHelpKey(msg)
	}

	if cmd, handled := m.routeToModal(msg); handled {
		return m, cmd
	}

	switch m.focus {
	case focusRevset:
		return m.handleRevsetKey(msg)
	case focusForge:
		return m.handleForgeKey(msg)
	case focusLog:
	}

	key := m.keys.Resolve(msg.String())
	if next, cmd, handled := m.handleRootKey(key); handled {
		return next, cmd
	}

	if next, cmd, handled := m.panelKey(key); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleActionKey(key); handled {
		return next, cmd
	}

	cmd := m.log.Update(msg)

	return m, cmd
}

// routeToModal hands every key to whichever overlay is open, because a modal
// sharing j and k with the log underneath would move both cursors at once.
func (m *Model) routeToModal(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case m.confirm.IsOpen():
		return m.confirm.Update(msg), true
	case m.bookmarks.IsOpen():
		return m.bookmarks.Update(msg), true
	case m.detail.IsOpen():
		return m.detail.Update(msg), true
	case m.diff.IsOpen():
		return m.diff.Update(msg), true
	case m.files.IsOpen():
		return m.files.Update(msg), true
	case m.prompt.IsOpen():
		return m.prompt.Update(msg), true
	case m.destPicker.IsOpen():
		return m.destPicker.Update(msg), true
	case m.opLog.IsOpen():
		return m.opLog.Update(msg), true
	case m.spaces.IsOpen():
		return m.spaces.Update(msg), true
	case m.evolog.IsOpen():
		return m.evolog.Update(msg), true
	case m.palette.IsOpen():
		return m.palette.Update(msg), true
	case m.find.IsOpen():
		return m.find.Update(msg), true
	}

	return nil, false
}

// handleRootKey covers the keys that belong to the application rather than to
// the revision under the cursor.
func (m Model) handleRootKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "L":
		m.focus = focusRevset

		cmd := m.revsetBox.Focus()

		return m, cmd, true
	case keyTab:
		next, cmd := m.cycleFocus()

		return next, cmd, true
	case "p":
		next, cmd := m.toggleForge()

		return next, cmd, true
	case "q", "ctrl+c":
		m.quitting = true

		return m, tea.Quit, true
	case "?":
		m.showHelp = true
		m.help = m.help.ScrollToStart()

		return m, nil, true
	case "u":
		return m, m.undo(), true
	case "ctrl+r":
		return m, m.redo(), true
	case "ctrl+l":
		return m, m.loadLog(m.revset), true
	case "/":
		cmd := m.find.Open()
		m.resize()

		return m, cmd, true
	case ":":
		cmd := m.palette.Open(m.paletteCommands())

		return m, cmd, true
	}

	return m, nil, false
}

// panelKey covers the keys that open a panel of their own or fire a whole-repository action,
// which read and reload rather than act on the revision under the cursor.
func (m Model) panelKey(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "o":
		return m, m.loadOpLog(), true
	case "w":
		return m, m.loadWorkspaces(), true
	case "b":
		return m, m.loadBookmarks(), true
	case "f":
		return m, m.fetch(), true
	case "x":
		return m, m.previewDivergent(), true
	}

	return m, nil, false
}

// cycleFocus walks Tab through the panes that are open, so a focused pane owns
// its own keys and a mnemonic cannot mean two things at once.
func (m Model) cycleFocus() (tea.Model, tea.Cmd) {
	switch {
	case m.focus == focusLog:
		m.focus = focusRevset

		cmd := m.revsetBox.Focus()

		return m, cmd
	case m.focus == focusRevset && m.showForge:
		m.revsetBox.Blur()
		m.focus = focusForge

		return m, nil
	default:
		m.revsetBox.Blur()
		m.focus = focusLog

		return m, nil
	}
}

func (m Model) handleForgeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		m.focus = focusLog

		return m, nil
	case keyTab:
		return m.cycleFocus()
	}

	cmd := m.forge.Update(msg)

	return m, cmd
}

func (m Model) handleRevsetKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == keyTab {
		return m.cycleFocus()
	}

	if msg.String() == keyEsc {
		m.revsetBox.Blur()
		m.focus = focusLog

		return m, nil
	}

	cmd := m.revsetBox.Update(msg)
	m.resize()

	return m, cmd
}

func (m Model) handleHelpKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, "q", "?":
		m.showHelp = false
	case "up", "k":
		m.help = m.help.ScrollBy(-1)
	case "down", "j":
		m.help = m.help.ScrollBy(1)
	case "g", "home":
		m.help = m.help.ScrollToStart()
	case "G", "end":
		m.help = m.help.ScrollToEnd()
	}

	return m, nil
}

func (m Model) toggleForge() (tea.Model, tea.Cmd) {
	m.showForge = !m.showForge
	if !m.showForge && m.focus == focusForge {
		m.focus = focusLog
	}

	m.resize()

	cmd := m.forge.SetFocused(m.showForge)

	return m, cmd
}
