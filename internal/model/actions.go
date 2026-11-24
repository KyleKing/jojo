package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// targets is the one rule every verb that takes several revisions follows: the checked revisions when
// any are checked, else the one under the cursor. The footer counts the checks while they stand, and
// a successful write or Esc clears them, so a check cannot quietly redirect a later action.
func (m Model) targets() []string {
	if checked := m.log.Checked(); len(checked) > 0 {
		return checked
	}

	if rev := m.log.Selected(); rev != nil {
		return []string{rev.ChangeID}
	}

	return nil
}

// revisionAction runs the verbs on the revision under the cursor (or the checked ones). Each
// executes immediately and offers undo rather than opening a confirm gate, because the operation log
// already covers them; the verbs that need a destination open the in-log picker.
func (m Model) revisionAction(action string) (tea.Model, tea.Cmd) {
	rev := m.log.Selected()
	if rev == nil {
		return m, nil
	}

	if cmd, handled := m.writeAction(action, *rev); handled {
		return m, cmd
	}

	if cmd, handled := m.viewAction(action, *rev); handled {
		return m, cmd
	}

	return m.openAction(action, *rev)
}

// writeAction covers the verbs that need no argument, so they run as the key lands.
func (m *Model) writeAction(action string, rev jj.Revision) (tea.Cmd, bool) {
	targets := m.targets()

	switch action {
	case help.ActionNew:
		return m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.New(ctx, targets) }), true
	case help.ActionEdit:
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Edit(ctx, rev.ChangeID) }), true
	case help.ActionAbandon:
		return m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Abandon(ctx, targets) }), true
	case help.ActionAbsorb:
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Absorb(ctx, rev.ChangeID) }), true
	case help.ActionParallelize:
		return m.parallelize(), true
	case help.ActionCheck:
		m.log.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})

		return nil, true
	}

	return nil, false
}

// viewAction covers the verbs that read rather than write, each opening a panel or a diff.
func (m Model) viewAction(action string, rev jj.Revision) (tea.Cmd, bool) {
	switch action {
	case keys.ActionInspect:
		return m.loadDetail(rev.ChangeID), true
	case keys.ActionYank:
		return tea.Batch(tea.SetClipboard(rev.ChangeID), m.say("yanked "+rev.ChangeID)), true
	case help.ActionFiles:
		return m.filesKey(rev), true
	case help.ActionResolve:
		return m.resolveKey(rev), true
	case help.ActionPush:
		targets := m.targets()

		return m.previewPush(joinIDs(targets), jj.PushOptions{Revisions: targets}), true
	case help.ActionDiff:
		return m.showDiff(jj.DiffTarget{Revision: rev.ChangeID}), true
	case help.ActionCompare:
		return m.openCompare(rev.ChangeID), true
	}

	return nil, false
}

// openAction covers the verbs that need an argument, a terminal, or a view of their own.
func (m Model) openAction(action string, rev jj.Revision) (tea.Model, tea.Cmd) {
	switch action {
	case keys.ActionDescribe:
		return m, m.loadDescribe(rev)
	case keys.ActionDescribeEditor:
		return m, m.describeInEditor(rev.ChangeID, "")
	case help.ActionCommit:
		return m.openCommitPrompt(rev)
	case help.ActionDiffedit:
		return m, m.exec("edited "+rev.ChangeID+undoHint, m.hunkArgs("diffedit", "-r", rev.ChangeID)...)
	case help.ActionSplit:
		return m, m.exec("split"+undoHint, m.hunkArgs("split", "-i", "-r", rev.ChangeID)...)
	case help.ActionBisect:
		return m.openBisect()
	case help.ActionBookmarkCreate:
		target := rev.ChangeID

		return m.askBookmarkName("new bookmark on "+target, func(m Model, name string) (Model, tea.Cmd) {
			return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
				return m.client.BookmarkSet(ctx, name, target)
			})
		}), nil
	}

	cmd := m.openVerb(action, rev)

	return m, cmd
}

// openVerb puts the log into picking mode for a verb that needs a destination. New takes the checked
// revisions as extra parents and nothing else, so with none checked it is a destination alone, and
// restore acts on the one revision under the cursor.
func (m *Model) openVerb(action string, rev jj.Revision) tea.Cmd {
	targets := m.targets()

	switch action {
	case help.ActionSquash:
		return m.openPicker(pickSquash, targets)
	case help.ActionDuplicate:
		return m.openPicker(pickDuplicate, targets)
	case help.ActionNewPlaced:
		return m.openPicker(pickNew, m.log.Checked())
	case help.ActionRestore:
		return m.openPicker(pickRestore, []string{rev.ChangeID})
	case help.ActionRebase:
		return m.openPicker(pickRebase, targets)
	case help.ActionRebaseSubtree:
		return m.openPicker(pickRebase, targets, "s")
	}

	return nil
}

// openCommitPrompt asks for the message the working copy is committed with. Commit only acts on @,
// so on any other row it says where to go rather than committing something else.
func (m Model) openCommitPrompt(rev jj.Revision) (tea.Model, tea.Cmd) {
	if !rev.WorkingCopy {
		return m, m.say("c commits @, the working copy: @ jumps there")
	}

	m.onSubmit = func(m Model, text string) (Model, tea.Cmd) {
		if strings.TrimSpace(text) == "" {
			return m, m.say("commit needs a message, nothing ran")
		}

		cmd := m.act("@", func(ctx context.Context, c *jj.Client) (string, error) { return c.Commit(ctx, text) })

		return m, cmd
	}
	m.onEditor = func(m Model, _ string) (Model, tea.Cmd) {
		return m, m.exec("committed"+undoHint, "commit")
	}

	return m, m.prompt.OpenText("commit @", rev.Description)
}

// shell hands the terminal to $SHELL in the repository root and reloads the log on return, since
// whatever ran there may have rewritten it.
func (m Model) shell() tea.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "sh"
	}

	cmd := exec.CommandContext(m.ctx, sh) //nolint:gosec // G204: the user's own shell, by design.
	cmd.Dir = m.repoPath

	return m.execCommand("back from the shell", cmd)
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}

	if len(ids) == 1 {
		return ids[0]
	}

	return ids[0] + " and " + strconv.Itoa(len(ids)-1) + " more"
}

// exec hands the terminal to jj, which is how the interactive verbs reach jj's own diff editor.
func (m Model) exec(done string, args ...string) tea.Cmd {
	//nolint:gosec // G204: the binary is a literal and args are built here, never from user input.
	cmd := exec.CommandContext(m.ctx, "jj", args...)
	cmd.Dir = m.repoPath

	return m.execCommand(done, cmd)
}

func (m Model) execCommand(done string, cmd *exec.Cmd) tea.Cmd {
	revset, client := m.revset, m.client
	// The repaint covers whatever the tool printed, so the message has to name the command itself.
	// An exit status says nothing a reader who just quit the editor does not already know, so the
	// line reports the one fact that matters: nothing was written.
	what := strings.Join(cmd.Args, " ")

	return tea.Sequence(
		func() tea.Msg { client.AcquireQueue(); return nil },
		tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return messageMsg{text: what + " did not finish"}
			}

			return messageMsg{text: done}
		}),
		func() tea.Msg { client.ReleaseQueue(); return nil },
		m.loadLog(revset),
	)
}

func (Model) say(text string) tea.Cmd {
	return func() tea.Msg { return messageMsg{text: text} }
}

// undoHint replaces a confirm gate, because the operation log makes every one of these reversible.
const undoHint = ", press u to undo"

// wrote turns a write's outcome into the status line: the command that ran, or the offer of the
// escape hatch when jj refused it as immutable.
//
//nolint:ireturn // a tea.Msg is the return type the runtime takes; naming a concrete one would lie.
func wrote(ran, hint string, err error) tea.Msg {
	switch {
	case errors.Is(err, jj.ErrNothingChanged):
		return messageMsg{text: ran + ": nothing changed"}
	case errors.Is(err, jj.ErrImmutable):
		return refusedMsg{err: err}
	case errors.Is(err, jj.ErrStaleWorkingCopy):
		return staleMsg{err: err}
	case err != nil:
		return messageMsg{text: err.Error(), output: jj.FullOutput(err)}
	}

	return messageMsg{text: ran + hint, wrote: true}
}

// run executes a jj write then reloads the log.
func (m Model) run(hint string, mutate func(context.Context) (string, error)) tea.Cmd {
	revset, ctx := m.revset, m.ctx

	return tea.Sequence(
		func() tea.Msg {
			ran, err := mutate(ctx)

			return wrote(ran, hint, err)
		},
		m.loadLog(revset),
	)
}

// undo and redo are jj's own, which step through the operation log the way an editor's undo stack
// does and, unlike restoring an operation by id, refuse an operation another workspace performed.
func (m Model) undo() tea.Cmd {
	return m.run(", press ctrl+r to redo", m.client.Undo)
}

func (m Model) redo() tea.Cmd {
	return m.run(undoHint, m.client.Redo)
}
