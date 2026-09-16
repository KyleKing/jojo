package model

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/workspaceview"
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
	pendingWorkspaceAdd
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

	switch action {
	case pendingDescribe:
		return m, m.run(undoable("described "+source), func(ctx context.Context) error {
			return m.client.Describe(ctx, source, msg.Value)
		})
	case pendingWorkspaceAdd:
		return m, m.addWorkspace(msg.Value)
	case pendingNone, pendingSquash, pendingRebaseRevisions, pendingRebaseSource:
		return m, nil
	}

	return m, nil
}

func (m Model) handlePicked(msg destpicker.PickedMsg) (tea.Model, tea.Cmd) {
	source, action, dest := m.pendingOn, m.pending, msg.Revision.ChangeID

	m.destPicker.Close()
	m.pending, m.pendingOn = pendingNone, ""

	switch action {
	case pendingWorkspaceAdd, pendingNone, pendingDescribe:
		return m, nil
	case pendingSquash:
		return m, m.run(undoable("squashed "+source+" into "+dest), func(ctx context.Context) error {
			return m.client.Squash(ctx, source, dest)
		})
	case pendingRebaseRevisions, pendingRebaseSource:
		return m, m.rebase(source, dest, action)
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

// opLogLimit caps the operation log, which grows without bound in an active repository.
const opLogLimit = 200

func (m Model) loadOpLog() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		ops, err := client.OpLog(ctx, opLogLimit)

		return opLogLoadedMsg{ops: ops, err: err}
	}
}

func (m Model) applyOpLog(msg opLogLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = msg.err.Error()

		return m, nil
	}

	cmd := m.opLog.Open(msg.ops)

	return m, cmd
}

func (m Model) restoreOperation(msg oplogview.RestoreMsg) (tea.Model, tea.Cmd) {
	op := msg.Operation.ID

	return m, m.run(undoable("restored "+op), func(ctx context.Context) error {
		return m.client.OpRestore(ctx, op)
	})
}

func (m Model) loadWorkspaces() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		spaces, err := client.Workspaces(ctx)

		return workspacesLoadedMsg{spaces: spaces, err: err}
	}
}

func (m Model) applyWorkspaces(msg workspacesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = msg.err.Error()

		return m, nil
	}

	cmd := m.spaces.Open(msg.spaces)

	return m, cmd
}

func (m Model) openWorkspaceAdd() (tea.Model, tea.Cmd) {
	m.spaces.Close()
	m.pending = pendingWorkspaceAdd

	cmd := m.prompt.Open("new workspace, created beside "+filepath.Base(m.repoPath), "")

	return m, cmd
}

// addWorkspace puts the new working copy beside the repository rather than inside it, because a
// workspace nested under the repository root would be snapshotted into its own commits.
func (m Model) addWorkspace(name string) tea.Cmd {
	if name == "" {
		return m.say("a workspace needs a name")
	}

	path := workspacePath(m.repoPath, name)

	return m.run(undoable("added "+name+" at "+path), func(ctx context.Context) error {
		return m.client.WorkspaceAdd(ctx, name, path)
	})
}

func workspacePath(repoPath, name string) string {
	return filepath.Join(filepath.Dir(repoPath), filepath.Base(repoPath)+"-"+name)
}

// forgetWorkspace stops tracking the workspace and leaves its directory alone, which is what jj does
// and what makes the action reversible.
func (m Model) forgetWorkspace(msg workspaceview.ForgetMsg) (tea.Model, tea.Cmd) {
	name := msg.Name
	m.spaces.Close()

	return m, m.run(undoable("forgot "+name), func(ctx context.Context) error {
		return m.client.WorkspaceForget(ctx, name)
	})
}

func (m Model) cancelModals() Model {
	m.prompt.Close()
	m.destPicker.Close()
	m.opLog.Close()
	m.spaces.Close()
	m.pending, m.pendingOn = pendingNone, ""

	return m
}
