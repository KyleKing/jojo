package model

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/jj"
)

// pending names the action a modal is collecting an argument for, so the
// result is dispatched on what was asked rather than on the modal's title.
type pending int

const (
	pendingNone pending = iota
	pendingDescribe
	pendingSquash
	pendingRebaseRevisions
	pendingRebaseSource
)

// openDescribe seeds the prompt with the current description, because an edit
// is far more common than writing one from nothing.
func (m *Model) openDescribe(rev jj.Revision) tea.Cmd {
	m.pending = pendingDescribe
	m.pendingOn = rev.ChangeID

	return m.prompt.Open("describe "+rev.ChangeID, rev.Description)
}

func (m *Model) openSquash(rev jj.Revision) tea.Cmd {
	m.pending = pendingSquash
	m.pendingOn = rev.ChangeID

	return m.destPicker.Open("squash "+rev.ChangeID+" into", m.rows, rev.ChangeID)
}

// openRebase asks for the destination. Scope is already decided by which key
// was pressed, because jj's -r and -s move different things and guessing
// between them silently rewrites the wrong revisions.
func (m *Model) openRebase(rev jj.Revision, scope pending) tea.Cmd {
	m.pending = scope
	m.pendingOn = rev.ChangeID

	what := "rebase " + rev.ChangeID
	if scope == pendingRebaseSource {
		what += " and its descendants"
	}

	return m.destPicker.Open(what+" onto", m.rows, rev.ChangeID)
}

func (m Model) handleSubmitted(msg prompt.SubmittedMsg) (tea.Model, tea.Cmd) {
	source, action := m.pendingOn, m.pending

	m.prompt.Close()
	m.pending, m.pendingOn = pendingNone, ""

	if action != pendingDescribe {
		return m, nil
	}

	return m, m.run(undoable("described "+source), func(ctx context.Context) error {
		return m.client.Describe(ctx, source, msg.Value)
	})
}

func (m Model) handlePicked(msg destpicker.PickedMsg) (tea.Model, tea.Cmd) {
	source, action, dest := m.pendingOn, m.pending, msg.Revision.ChangeID

	m.destPicker.Close()
	m.pending, m.pendingOn = pendingNone, ""

	switch action {
	case pendingSquash:
		return m, m.run(undoable("squashed "+source+" into "+dest), func(ctx context.Context) error {
			return m.client.Squash(ctx, source, dest)
		})
	case pendingRebaseRevisions, pendingRebaseSource:
		return m, m.rebase(source, dest, action)
	case pendingNone, pendingDescribe:
		return m, nil
	}

	return m, nil
}

func (m Model) rebase(source, dest string, action pending) tea.Cmd {
	scope := jj.RebaseScopeRevisions
	if action == pendingRebaseSource {
		scope = jj.RebaseScopeSource
	}

	return m.run(undoable("rebased "+source+" onto "+dest), func(ctx context.Context) error {
		return m.client.Rebase(ctx, jj.RebaseOptions{
			Revisions:   []string{source},
			Destination: jj.RebaseDestination{Revisions: []string{dest}, Mode: jj.RebaseDestOnto},
			Scope:       scope,
		})
	})
}

func (m Model) cancelModals() (tea.Model, tea.Cmd) {
	m.prompt.Close()
	m.destPicker.Close()
	m.pending, m.pendingOn = pendingNone, ""

	return m, nil
}
