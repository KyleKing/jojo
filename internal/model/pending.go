package model

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/evologview"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/components/workspaceview"
	"github.com/kyleking/jojo/internal/jj"
)

// The modals collect one argument for an action chosen earlier, so the action travels with them as a
// closure. A state enum would have to be read the same way in three places to stay right.
type (
	submitted func(Model, string) (Model, tea.Cmd)
	picked    func(Model, destpicker.PickedMsg) (Model, tea.Cmd)
)

// openDescribe seeds the prompt with the current description, because an edit is far more common
// than writing one from nothing.
func (m *Model) openDescribe(rev jj.Revision) tea.Cmd {
	id := rev.ChangeID
	m.onSubmit = func(m Model, text string) (Model, tea.Cmd) {
		cmd := m.act(id, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Describe(ctx, id, text)
		})

		return m, cmd
	}

	return m.prompt.Open("describe "+id, rev.Description)
}

func (m *Model) openSquash(rev jj.Revision) tea.Cmd {
	id := rev.ChangeID
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		cmd := m.act(id, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Squash(ctx, id, dest.Revision.ChangeID)
		})

		return m, cmd
	}

	return m.destPicker.Open("squash "+id+" into", m.rows, id)
}

// openRebase asks for the destination. Scope is decided by the key that was pressed, because jj's -r
// and -s move different things and guessing between them rewrites the wrong revisions.
func (m *Model) openRebase(rev jj.Revision, scope jj.RebaseScope) tea.Cmd {
	id := rev.ChangeID
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		cmd := m.act(id, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Rebase(ctx, jj.RebaseOptions{
				Revisions:   []string{id},
				Destination: []string{dest.Revision.ChangeID},
				Mode:        rebaseMode(dest.Placement),
				Scope:       scope,
			})
		})

		return m, cmd
	}

	what := "rebase " + id
	if scope == jj.RebaseScopeSource {
		what += " and its descendants"
	}

	return m.destPicker.Open(what+" onto", m.rows, id, destpicker.WithPlacement())
}

func (m Model) handleSubmitted(msg prompt.SubmittedMsg) (tea.Model, tea.Cmd) {
	run := m.onSubmit
	m.prompt.Close()
	m.onSubmit = nil

	if run == nil {
		return m, nil
	}

	return run(m, msg.Value)
}

func (m Model) handlePicked(msg destpicker.PickedMsg) (tea.Model, tea.Cmd) {
	run := m.onPick
	m.destPicker.Close()
	m.onPick = nil

	if run == nil {
		return m, nil
	}

	return run(m, msg)
}

var rebaseModes = map[destpicker.Placement]jj.RebaseDestinationMode{
	destpicker.PlaceAfter:  jj.RebaseDestInsertAfter,
	destpicker.PlaceBefore: jj.RebaseDestInsertBefore,
}

func rebaseMode(where destpicker.Placement) jj.RebaseDestinationMode {
	return rebaseModes[where]
}

func (m Model) restoreOperation(msg oplogview.RestoreMsg) (tea.Model, tea.Cmd) {
	op := msg.Operation.ID

	return m, m.run(undoHint, func(ctx context.Context) (string, error) {
		return m.client.OpRestore(ctx, op)
	})
}

// restoreVersion puts an earlier version's content back into the change it belongs to, which leaves
// the change's description and its place in the graph alone.
func (m Model) restoreVersion(msg evologview.RestoreMsg) (tea.Model, tea.Cmd) {
	from, into := msg.CommitID, m.evolog.ChangeID()
	m.evolog.Close()

	return m, m.run(undoHint, func(ctx context.Context) (string, error) {
		return m.client.Restore(ctx, from, into)
	})
}

func (m Model) openWorkspaceAdd() (tea.Model, tea.Cmd) {
	m.spaces.Close()
	m.onSubmit = func(m Model, name string) (Model, tea.Cmd) { return m, m.addWorkspace(name) }

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

	return m.run(undoHint, func(ctx context.Context) (string, error) {
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

	return m, m.run(undoHint, func(ctx context.Context) (string, error) {
		return m.client.WorkspaceForget(ctx, name)
	})
}

func (m Model) cancelModals() Model {
	m.prompt.Close()
	m.destPicker.Close()
	m.opLog.Close()
	m.spaces.Close()
	m.evolog.Close()
	m.bookmarks.Close()
	m.detail.Close()
	m.files.Close()
	m.diff.Close()
	m.onSubmit, m.onPick = nil, nil

	return m
}
