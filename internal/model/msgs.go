package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/evologview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/workspaceview"
)

// handleOverlayMsg covers what the modals report back: an argument collected, an action confirmed,
// or a dismissal. Every overlay in the app funnels through one of these.
func (m Model) handleOverlayMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case prompt.SubmittedMsg:
		next, cmd := m.handleSubmitted(msg)

		return next, cmd, true
	case destpicker.PickedMsg:
		next, cmd := m.handlePicked(msg)

		return next, cmd, true
	case refusedMsg:
		next, cmd := m.offerForce(msg)

		return next, cmd, true
	case confirm.ConfirmedMsg:
		next, cmd := m.handleConfirmed(msg)

		return next, cmd, true
	case confirm.CancelledMsg:
		return m.cancelConfirm(), nil, true
	case prompt.CancelledMsg, destpicker.CancelledMsg, oplogview.CancelledMsg, workspaceview.CancelledMsg,
		evologview.CancelledMsg, bookmarkview.CancelledMsg:
		return m.cancelModals(), nil, true
	}

	return m.handlePanelMsg(msg)
}

// handlePanelMsg covers the scoped views, each of which loads its own list and then reports the one
// action the user picked from it.
func (m Model) handlePanelMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case opLogLoadedMsg:
		next, cmd := m.applyOpLog(msg)

		return next, cmd, true
	case oplogview.RestoreMsg:
		next, cmd := m.restoreOperation(msg)

		return next, cmd, true
	case workspacesLoadedMsg:
		next, cmd := m.applyWorkspaces(msg)

		return next, cmd, true
	case workspaceview.AddRequestedMsg:
		next, cmd := m.openWorkspaceAdd()

		return next, cmd, true
	case workspaceview.ForgetMsg:
		next, cmd := m.forgetWorkspace(msg)

		return next, cmd, true
	case evologLoadedMsg:
		next, cmd := m.applyEvolog(msg)

		return next, cmd, true
	case evologview.ShowMsg:
		return m, m.exec("", "diff", "-r", msg.CommitID), true
	case evologview.InterdiffMsg:
		return m, m.exec("", "diff", "--from", msg.From, "--to", msg.To), true
	case evologview.RestoreMsg:
		next, cmd := m.restoreVersion(msg)

		return next, cmd, true
	}

	return m, nil, false
}
