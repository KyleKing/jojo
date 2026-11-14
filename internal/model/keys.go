package model

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.usage.Key(ctxHelp, msg.String())

		return m.handleHelpKey(msg)
	}

	if cmd, handled := m.routeToModal(msg); handled {
		return m, cmd
	}

	switch m.focus {
	case focusRevset:
		return m.handleRevsetKey(msg)
	case focusForge:
		m.usage.Key(ctxForge, msg.String())

		return m.handleForgeKey(msg)
	case focusLog:
	}

	m.usage.Key(ctxLog, msg.String())

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

	if m.log.AtEnd() && m.logFull {
		m.logWindow *= 2

		return m, tea.Batch(cmd, m.loadLog(m.revset))
	}

	return m, cmd
}

// routeToModal hands every key to whichever overlay is open, because a modal
// sharing j and k with the log underneath would move both cursors at once.
func (m *Model) routeToModal(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	ctx := m.openModal()
	if ctx == "" {
		return nil, false
	}

	m.recordModalKey(ctx, msg)

	switch ctx {
	case ctxConfirm:
		return m.confirm.Update(msg), true
	case ctxBookmarks:
		return m.bookmarks.Update(msg), true
	case ctxDetail:
		return m.detail.Update(msg), true
	case ctxDiff:
		return m.diff.Update(msg), true
	case ctxFiles:
		return m.files.Update(msg), true
	case ctxPrompt:
		return m.prompt.Update(msg), true
	case ctxDestination:
		return m.destPicker.Update(msg), true
	case ctxOpLog:
		return m.opLog.Update(msg), true
	case ctxSpaces:
		return m.spaces.Update(msg), true
	case ctxEvolog:
		return m.evolog.Update(msg), true
	case ctxPalette:
		return m.palette.Update(msg), true
	default:
		return m.find.Update(msg), true
	}
}

// openModal names the overlay that takes the next key, "" when none is open.
func (m *Model) openModal() string {
	switch {
	case m.confirm.IsOpen():
		return ctxConfirm
	case m.bookmarks.IsOpen():
		return ctxBookmarks
	case m.detail.IsOpen():
		return ctxDetail
	case m.diff.IsOpen():
		return ctxDiff
	case m.files.IsOpen():
		return ctxFiles
	case m.prompt.IsOpen():
		return ctxPrompt
	case m.destPicker.IsOpen():
		return ctxDestination
	case m.opLog.IsOpen():
		return ctxOpLog
	case m.spaces.IsOpen():
		return ctxSpaces
	case m.evolog.IsOpen():
		return ctxEvolog
	case m.palette.IsOpen():
		return ctxPalette
	case m.find.IsOpen():
		return ctxFind
	}

	return ""
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
