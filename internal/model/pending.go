package model

import (
	"context"
	"path/filepath"
	"strings"

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

// loadDescribe reads the whole description before the prompt opens, because the log row carries only
// its first line and seeding the prompt with that dropped the body and trailers on Enter.
func (m Model) loadDescribe(rev jj.Revision) tea.Cmd {
	client, id := m.client, rev.ChangeID

	return load(m.ctx,
		func(ctx context.Context) (jj.Detail, error) { return client.RevisionDetail(ctx, id) },
		openDescribe)
}

// openDescribe seeds the prompt with the current description, because an edit is far more common
// than writing one from nothing. Submitting it unchanged writes nothing, so Enter on a glance is not
// an operation to undo.
func openDescribe(m *Model, detail jj.Detail) tea.Cmd {
	id, before := detail.ChangeID, detail.Description
	m.onSubmit = func(m Model, text string) (Model, tea.Cmd) {
		if sameDescription(text, before) {
			return m, m.say("description of " + id + " unchanged")
		}

		cmd := m.act(id, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Describe(ctx, id, text)
		})

		return m, cmd
	}
	m.onEditor = func(m Model, text string) (Model, tea.Cmd) { return m, m.describeInEditor(id, text) }

	return m.prompt.OpenText("describe "+id, before)
}

// sameDescription ignores the trailing newline jj adds to every description it stores.
func sameDescription(a, b string) bool {
	return strings.TrimRight(a, "\n") == strings.TrimRight(b, "\n")
}

// describeInEditor hands the terminal to $EDITOR through jj, seeded with text when the prompt had
// one, which is the way to write a description longer than a prompt is comfortable for.
func (m Model) describeInEditor(id, text string) tea.Cmd {
	args := []string{"describe", "-r", id}
	if text != "" {
		args = append(args, "--editor", "-m", text)
	}

	return m.exec("described "+id+undoHint, args...)
}

func (m Model) handleEditorRequested(msg prompt.EditorRequestedMsg) (tea.Model, tea.Cmd) {
	run := m.onEditor
	m.prompt.Close()
	m.onSubmit, m.onEditor = nil, nil

	if run == nil {
		return m, nil
	}

	return run(m, msg.Value)
}

// openSquash asks where the targets go. Several checked revisions squash into one destination in one
// operation, which jj takes as repeated --from.
func (m *Model) openSquash(ids []string) tea.Cmd {
	what := joinIDs(ids)
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		cmd := m.act(what, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.SquashAll(ctx, ids, dest.Revision.ChangeID)
		})

		return m, cmd
	}

	return m.destPicker.Open("squash "+what+" into", m.rows, ids[0], destpicker.Excluding(ids...))
}

// openMoveHunks asks for the destination, then hands the terminal to jj's configured diff editor,
// which is where the hunks and lines actually get picked. Nothing here reimplements that selection.
func (m *Model) openMoveHunks(rev jj.Revision) tea.Cmd {
	id := rev.ChangeID
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		into := dest.Revision.ChangeID
		return m, m.exec("moved part of "+id+" into "+into+undoHint, m.squashHunkArgs(id, into)...)
	}

	return m.destPicker.Open("move part of "+id+" into", m.rows, id)
}

// openMoveFile asks for the destination and moves one path, which needs no editor at all. It is the
// shortest path from "this file belongs in that commit" to it being there.
func (m *Model) openMoveFile(from, path string) tea.Cmd {
	m.files.Close()
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		into := dest.Revision.ChangeID
		cmd := m.act(path, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Squash(ctx, from, into, path)
		})

		return m, cmd
	}

	return m.destPicker.Open("move "+path+" into", m.rows, from)
}

// openRebase asks for the destination. Scope is decided by the key that was pressed, because jj's -r
// and -s move different things and guessing between them rewrites the wrong revisions.
func (m *Model) openRebase(ids []string, scope jj.RebaseScope) tea.Cmd {
	id := joinIDs(ids)
	m.onPick = func(m Model, dest destpicker.PickedMsg) (Model, tea.Cmd) {
		cmd := m.act(id, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.Rebase(ctx, jj.RebaseOptions{
				Revisions:   ids,
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

	// Only what moves has to be mutable: landing on trunk() or the root rewrites nothing there.
	return m.destPicker.Open(what+" onto", m.rows, ids[0], destpicker.WithPlacement(),
		destpicker.AllowImmutableDestination(), destpicker.Excluding(ids...))
}

func (m Model) handleSubmitted(msg prompt.SubmittedMsg) (tea.Model, tea.Cmd) {
	run := m.onSubmit
	m.prompt.Close()
	m.onSubmit, m.onEditor = nil, nil

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
	m.onSubmit, m.onPick, m.onEditor = nil, nil, nil

	return m
}
