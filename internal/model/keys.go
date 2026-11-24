package model

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.usage.Key(ctxHelp, msg.String())

		return m.handleHelpKey(msg)
	}

	if cmd, handled := m.routeToOutputOrModal(msg); handled {
		return m, cmd
	}

	key, next, cmd, handled := m.routeToMode(msg)
	if handled {
		return next, cmd
	}

	switch m.focus {
	case focusRevset:
		return m.handleRevsetKey(msg)
	case focusForge:
		m.usage.Key(ctxForge, msg.String())

		return m.handleForgeKey(msg)
	case focusDock:
		m.usage.Key(ctxDock, msg.String())

		return m.handleDockKey(msg)
	case focusLog:
	}

	m.usage.Key(ctxLog, msg.String())

	if next, cmd, handled := m.handleRootKey(key); handled {
		return next, cmd
	}

	if next, cmd, handled := m.panelKey(key); handled {
		return next, cmd
	}

	if next, cmd, handled := m.handleActionKey(key); handled {
		return next, cmd
	}

	cmd = m.log.Update(msg)

	if m.log.AtEnd() && m.logFull {
		m.logWindow *= 2

		return m, tea.Batch(cmd, m.loadLog(m.revset))
	}

	return m, cmd
}

// routeToOutputOrModal opens the last failure on "!" anywhere a key is not text, and otherwise hands
// the key to whichever overlay is open.
func (m *Model) routeToOutputOrModal(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if msg.String() == "!" && m.lastOutput != "" && !m.typing() {
		m.usage.Key(ctxLog, "!")

		return m.showOutput(), true
	}

	return m.routeToModal(msg)
}

// routeToMode gives the key to the picker or to time travel while either is active, holds a chord
// prefix, and otherwise returns the key resolved for the log.
func (m *Model) routeToMode(msg tea.KeyPressMsg) (string, tea.Model, tea.Cmd, bool) {
	if m.pick.active {
		next, cmd := m.handlePickKey(msg)

		return "", next, cmd, true
	}

	key, held := m.chordKey(msg.String())
	if held {
		return "", *m, nil, true
	}

	if m.travel.active {
		next, cmd := m.handleTravelKey(key, msg)

		return "", next, cmd, true
	}

	if m.outline.active && m.focus == focusLog {
		next, cmd := m.handleOutlineKey(key)

		return "", next, cmd, true
	}

	return key, m, nil, false
}

// opStep is how far back [o and ]o move: older is positive.
func opStep(key string) int {
	if key == "]o" {
		return -1
	}

	return 1
}

// chordPrefixes start the two-key bindings: [o and ]o step through the op log, [r and ]r through
// the versions of the change in the dock, and ]] and [[ through the outline's chapters.
var chordPrefixes = map[string]bool{"[": true, "]": true}

// chordKey resolves a key against a held chord prefix, and holds a new one. A prefix followed by a
// key that completes nothing resolves to that pair, which no handler knows, so it does nothing.
func (m *Model) chordKey(pressed string) (string, bool) {
	if m.chord != "" {
		pair := m.chord + pressed
		m.chord = ""

		return m.keys.Resolve(pair), false
	}

	if chordPrefixes[pressed] && m.focus == focusLog {
		m.chord = pressed

		return "", true
	}

	return m.keys.Resolve(pressed), false
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
	case ctxOutput:
		return m.output.Update(msg), true
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
	}

	return m.routeToListModal(ctx, msg), true
}

// routeToListModal is routeToModal for the list overlays and the search bar.
func (m *Model) routeToListModal(ctx string, msg tea.KeyPressMsg) tea.Cmd {
	switch ctx {
	case ctxSpaces:
		return m.spaces.Update(msg)
	case ctxEvolog:
		return m.evolog.Update(msg)
	case ctxRounds:
		return m.roundsBox.Update(msg)
	case ctxPalette:
		return m.palette.Update(msg)
	}

	return m.find.Update(msg)
}

// openModal names the overlay that takes the next key, "" when none is open.
func (m *Model) openModal() string {
	switch {
	case m.output.IsOpen():
		return ctxOutput
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
	case m.roundsBox.IsOpen():
		return ctxRounds
	case m.palette.IsOpen():
		return ctxPalette
	case m.find.IsOpen():
		return ctxFind
	}

	return ""
}

// Typing reports whether the key goes into a text field, where "!" is a character rather than a verb.
func (m *Model) typing() bool {
	if m.focus == focusRevset {
		return true
	}

	switch m.openModal() {
	case ctxPrompt, ctxDestination, ctxPalette, ctxFind, ctxOutput:
		return true
	}

	return false
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
	case dockKey:
		next, cmd := m.toggleDock()

		return next, cmd, true
	case "q", keyQuit:
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
	case keyEsc:
		if len(m.log.Checked()) == 0 {
			return m, nil, false
		}

		m.log.ClearChecked()

		return m, m.say("checks cleared"), true
	case "!":
		cmd := m.showOutput()

		return m, cmd, true
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
	case "[o", "]o":
		cmd := m.stepOp(opStep(key))

		return m, cmd, true
	case "I":
		cmd := m.openOutline()

		return m, cmd, true
	case "H":
		cmd := m.openRoundsBox()

		return m, cmd, true
	case "[r", "]r":
		cmd := m.stepRound(motionDir(key))

		return m, cmd, true
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
	case (m.focus == focusRevset || m.focus == focusForge) && m.docked:
		m.revsetBox.Blur()
		m.focus = focusDock

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
	case "G", keyEnd:
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
