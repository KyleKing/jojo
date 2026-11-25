package model

import (
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/launch"
	"github.com/kyleking/aragonite/tui/palette"

	"github.com/kyleking/jojo/internal/components/help"
)

// screen is where the next key lands, which decides what it means: the shared keymap's contexts plus
// jojo's modes of the log (help.Travel, help.Outline, and one picker screen per verb).
func (m *Model) screen() keys.Context {
	switch {
	case m.diff.IsOpen() && m.openModal() == ctxDiff:
		return keys.View
	case m.files.IsOpen() && m.openModal() == ctxFiles:
		return keys.List
	case m.pick.active:
		return keys.Picker | pickScreens[m.pick.verb]
	case m.travel.active:
		return help.Travel
	case m.submit.active:
		return help.Submit
	case m.focus == focusDock:
		return keys.View
	case m.outline.active && m.focus == focusLog:
		return help.Outline
	}

	return keys.Log
}

// usageContext is the name the usage log files a screen's keys under.
func usageContext(screen keys.Context) string {
	switch {
	case screen&keys.Picker != 0:
		return ctxPick
	case screen == help.Travel:
		return ctxTravel
	case screen == help.Outline:
		return ctxOutline
	case screen == help.Submit:
		return ctxSubmit
	case screen == keys.List:
		return ctxFiles
	case screen == keys.View:
		return ctxDock
	}

	return ctxLog
}

// handleKeyPress is the one door every key comes through. Any key clears a tip the coach is showing
// and restarts its idle clock, which arms the one timer a tip waits on.
func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	now := time.Now()
	m.coach.Press(now)

	if !m.welcomeKey(msg.String(), now) {
		return m, m.coach.WakeAt(now)
	}

	// The log is on its way: a key pressed now means the log it is about to show. ctrl+c is never
	// held, so a jj that never answers can still be left.
	if m.early.Hold(msg) {
		return m, nil
	}

	// A question in the footer takes the next key, whatever it is.
	if m.asking != nil {
		next, cmd := m.answer(msg)

		return next, tea.Batch(cmd, m.coach.WakeAt(now))
	}

	// A prompt is on its way: what is typed now is its text, never a command.
	if m.opening && !m.prompt.IsOpen() {
		m.typeahead = append(slices.Clip(m.typeahead), msg)

		return m, nil
	}

	next, cmd := m.routeKey(msg)

	return next, tea.Batch(cmd, m.coach.WakeAt(now))
}

// landLog applies a log read, and on the first to land replays, in order, the keys held for it
// through Update. It takes them before applyLog, which marks the model ready; later reads hold none.
func (m Model) landLog(msg logLoadedMsg) (tea.Model, tea.Cmd) {
	held := m.early.Release()

	landed, cmd := m.applyLog(msg)

	next, pressed := launch.Replay(landed, held)

	return next, tea.Batch(cmd, pressed)
}

// routeKey sends a key where it lands. The palette, while open, takes every key; overlays with keys
// of their own (the prompt, the panels) take theirs directly; every screen with a keymap feeds the
// matcher, which holds a prefix while it waits and answers an action once a sequence completes. An
// action the gate refuses says why rather than doing nothing.
func (m Model) routeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.pal.IsOpen() {
		return m.paletteKey(msg)
	}

	if m.typingMessage() {
		return m.messageKey(msg)
	}

	if next, cmd, labeling := m.routeLabel(msg); labeling {
		return next, cmd
	}

	if ctx := m.openModal(); ctx != "" && ctx != ctxDiff && ctx != ctxFiles {
		cmd := m.routeToModal(ctx, msg)

		return m, cmd
	}

	switch m.focus {
	case focusRevset:
		return m.handleRevsetKey(msg)
	case focusForge:
		m.usage.Key(ctxForge, msg.String())

		return m.handleForgeKey(msg)
	case focusLog, focusDock:
	}

	screen := m.screen()

	match := m.matcher.Feed(screen, msg.String())
	if match.Pending || match.Canceled {
		return m, nil
	}

	pressed := strings.Join(match.Keys, " ")
	m.usage.Key(usageContext(screen), pressed)

	if match.Action == "" {
		return m.unbound(screen, msg)
	}

	if cmd, refused := m.refuse(screen, match.Action); refused {
		return m, cmd
	}

	m.observe(screen, match.Action, pressed)

	return m.dispatch(screen, match.Action)
}

// unbound is a key no binding names on screen: the log's list still pages on the keys it reads
// itself (home, end, page up and down).
// The past and the outline say why a verb does nothing there rather than doing nothing silently.
func (m Model) unbound(screen keys.Context, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if screen == keys.Log {
		return m.moveLog(msg)
	}

	if screen == help.Travel {
		return m.dispatchTravel("")
	}

	if screen == help.Outline {
		return m.dispatchOutline("")
	}

	return m, nil
}

// dispatch runs action on screen.
func (m Model) dispatch(screen keys.Context, action string) (tea.Model, tea.Cmd) {
	switch {
	case screen&keys.Picker != 0:
		return m.dispatchPick(action)
	case screen == help.Travel:
		return m.dispatchTravel(action)
	case screen == help.Outline:
		return m.dispatchOutline(action)
	case screen == help.Submit:
		return m.dispatchSubmit(action)
	case screen == keys.View:
		return m.dispatchView(action)
	case screen == keys.List:
		return m.dispatchList(action)
	}

	return m.dispatchLog(action)
}

// dispatchLog runs an action pressed in the log.
func (m Model) dispatchLog(action string) (tea.Model, tea.Cmd) {
	walking := m.jumps.walking
	m.jumps.walking = false

	if next, cmd, handled := m.historyAction(action); handled {
		return next, cmd
	}

	if next, cmd, handled := m.jumpAction(action, walking); handled {
		return next, cmd
	}

	if key, ok := navKey(action); ok {
		if jumpKeys[action] {
			m.noteJump(m.log.Selected())
		}

		return m.moveLog(key)
	}

	if next, cmd, handled := m.globalAction(action); handled {
		return next, cmd
	}

	if next, cmd, handled := m.motionAction(action); handled {
		return next, cmd
	}

	if next, cmd, handled := m.panelAction(action); handled {
		return next, cmd
	}

	return m.revisionAction(action)
}

// jumpKeys are the movement keys that count as a jump, which the jump list remembers the row of.
//
//nolint:gochecknoglobals // a fixed table, read only.
var jumpKeys = map[string]bool{keys.ActionTop: true, keys.ActionBottom: true, help.ActionWorkingCopy: true}

// jumpAction runs the label jump and the jump list. Tab walks forward too right after ctrl+o, which is
// how vim reads one key two terminals send differently.
func (m Model) jumpAction(action string, walking bool) (tea.Model, tea.Cmd, bool) {
	switch {
	case action == keys.ActionLabelJump:
		next, cmd := m.startLabels()

		return next, cmd, true
	case action == keys.ActionJumpBack:
		next, cmd := m.walkJumps(true)

		return next, cmd, true
	case action == keys.ActionJumpForward, action == keys.ActionSwitchPane && walking:
		next, cmd := m.walkJumps(false)

		return next, cmd, true
	}

	return m, nil, false
}

// moveLog moves the log's cursor, reading further back once it reaches the last row loaded.
func (m Model) moveLog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	cmd := m.log.Update(key)

	if m.log.AtEnd() && m.logFull {
		m.logWindow *= 2

		return m, tea.Batch(cmd, m.loadLog(m.revset))
	}

	return m, cmd
}

// navKeys are the keys the lists read for themselves, by the action that moves them, so a rebound
// movement key reaches a list that only knows its own.
var navKeys = map[string]tea.KeyPressMsg{
	keys.ActionLineDown:     {Code: tea.KeyDown},
	keys.ActionLineUp:       {Code: tea.KeyUp},
	keys.ActionHalfPageDown: {Code: tea.KeyPgDown},
	keys.ActionHalfPageUp:   {Code: tea.KeyPgUp},
	keys.ActionTop:          {Code: tea.KeyHome},
	keys.ActionBottom:       {Code: tea.KeyEnd},
	help.ActionWorkingCopy:  {Code: '@', Text: "@"},
}

// navKey is the key a list reads for a movement action.
func navKey(action string) (tea.KeyPressMsg, bool) {
	key, ok := navKeys[action]

	return key, ok
}

// globalAction covers the keys that belong to the application rather than to the revision under the
// cursor, which every screen that offers them shares.
func (m Model) globalAction(action string) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionQuit, keys.ActionCancel:
		m.quitting = true

		return m, tea.Quit, true
	case keys.ActionHelp:
		m.openPalette(palette.Browse)

		return m, nil, true
	case keys.ActionPalette:
		m.openPalette(palette.Run)

		return m, nil, true
	case keys.ActionSearch:
		cmd := m.find.Open()
		m.resize()

		return m, cmd, true
	case keys.ActionMessages:
		m.showOutput()

		return m, nil, true
	case keys.ActionSwitchPane:
		next, cmd := m.cycleFocus()

		return next, cmd, true
	case help.ActionPreview:
		next, cmd := m.toggleDock()

		return next, cmd, true
	}

	return m, nil, false
}

// panelAction covers the keys that open a panel of their own or act on the whole repository, which
// read and reload rather than act on the revision under the cursor.
func (m Model) panelAction(action string) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionUndo:
		return m, m.undo(), true
	case keys.ActionRedo:
		return m, m.redo(), true
	case keys.ActionReload:
		return m, m.loadLog(m.revset), true
	case keys.ActionBack:
		if len(m.log.Checked()) == 0 {
			return m, nil, true
		}

		m.log.ClearChecked()

		return m, m.say("checks cleared"), true
	case keys.ActionShell:
		return m, m.shell(), true
	case help.ActionRevset:
		m.focus = focusRevset

		return m, m.revsetBox.Focus(), true
	case help.ActionForge:
		next, cmd := m.toggleForge()

		return next, cmd, true
	}

	return m.repoAction(action)
}

// repoAction covers the panels and the reads of the repository as a whole.
func (m Model) repoAction(action string) (tea.Model, tea.Cmd, bool) {
	switch action {
	case help.ActionOpLog:
		return m, m.loadOpLog(), true
	case help.ActionWorkspaces:
		return m, m.loadWorkspaces(), true
	case help.ActionBookmarks:
		return m, m.loadBookmarks(), true
	case help.ActionFetch:
		return m, m.fetch(), true
	case help.ActionSync, help.ActionSyncTracked, help.ActionTrunk:
		return m.syncAction(action)
	case help.ActionConverge:
		return m, m.previewDivergent(), true
	case help.ActionArrange:
		return m, m.exec("arranged, press u to undo", "arrange"), true
	case help.ActionOpOlder, help.ActionOpNewer:
		cmd := m.stepOp(opStep(action))

		return m, cmd, true
	case keys.ActionOutline:
		cmd := m.openOutline()

		return m, cmd, true
	case keys.NextAction(keys.ObjUnread), keys.PrevAction(keys.ObjUnread):
		next, cmd := m.stepUnreadRev(keys.Log, actionDir(action))

		return next, cmd, true
	case keys.ActionOnlyUnread:
		cmd := m.toggleOnlyUnread()

		return m, cmd, true
	case help.ActionSubmitStack:
		m.submit.afterOutline = true
		cmd := m.openOutline()

		return m, cmd, true
	case keys.ActionRounds:
		cmd := m.openRoundsBox()

		return m, cmd, true
	case keys.NextAction(keys.ObjRound), keys.PrevAction(keys.ObjRound):
		cmd := m.stepRound(actionDir(action))

		return m, cmd, true
	}

	return m, nil, false
}

// opStep is how far an op motion moves: older is positive.
func opStep(action string) int {
	if action == help.ActionOpNewer {
		return -1
	}

	return 1
}

// actionDir is 1 for a forward motion and -1 for a backward one.
func actionDir(action string) int {
	if strings.HasSuffix(action, "-prev") {
		return -1
	}

	return 1
}

// cycleFocus walks Tab through the panes that are open, so a focused pane owns its own keys and a
// mnemonic cannot mean two things at once. The revset box is left out: L focuses it, and a stray Tab
// used to drop the next keys into it.
func (m Model) cycleFocus() (tea.Model, tea.Cmd) {
	m.revsetBox.Blur()

	switch {
	case (m.focus == focusLog || m.focus == focusRevset) && m.showForge:
		m.focus = focusForge
	case (m.focus == focusLog || m.focus == focusRevset || m.focus == focusForge) && m.docked:
		m.focus = focusDock
	default:
		m.focus = focusLog
	}

	return m, nil
}

// routeToModal hands every key to whichever overlay is open, because a modal sharing j and k with
// the log underneath would move both cursors at once.
func (m *Model) routeToModal(ctx string, msg tea.KeyPressMsg) tea.Cmd {
	m.recordModalKey(ctx, msg)

	switch ctx {
	case ctxOutput:
		return m.output.Update(msg)
	case ctxTrail:
		return m.trailView.Update(msg)
	case ctxConfirm:
		return m.confirm.Update(msg)
	case ctxBookmarks:
		return m.bookmarks.Update(msg)
	case ctxDetail:
		return m.detail.Update(msg)
	case ctxPrompt:
		return m.prompt.Update(msg)
	case ctxDestination:
		return m.destPicker.Update(msg)
	case ctxOpLog:
		return m.opLog.Update(msg)
	case ctxSpaces:
		return m.spaces.Update(msg)
	case ctxRounds:
		return m.roundsBox.Update(msg)
	}

	return m.find.Update(msg)
}

// openModal names the overlay that takes the next key, "" when none is open.
func (m *Model) openModal() string {
	switch {
	case m.output.IsOpen():
		return ctxOutput
	case m.trailView.IsOpen():
		return ctxTrail
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
	case m.roundsBox.IsOpen():
		return ctxRounds
	case m.find.IsOpen():
		return ctxFind
	}

	return ""
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

// handleRevsetKey types into the revset box, where Tab completes. Enter runs the revset and, like
// Esc, hands the keys back to the log.
func (m Model) handleRevsetKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == keyEsc {
		m.revsetBox.Blur()
		m.focus = focusLog

		return m, nil
	}

	cmd := m.revsetBox.Update(msg)
	if msg.String() == keyEnter {
		m.revsetBox.Blur()
		m.focus = focusLog
	}

	m.resize()

	return m, cmd
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
