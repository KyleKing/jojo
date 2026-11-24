package model

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

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

// handleActionKey runs the per-commit verbs. Each executes immediately and offers undo rather than
// opening a confirm gate, because the operation log already covers them.
func (m Model) handleActionKey(key string) (tea.Model, tea.Cmd, bool) {
	rev := m.log.Selected()
	if rev == nil {
		return m, nil, false
	}

	if cmd, handled := m.writeKey(key, *rev); handled {
		return m, cmd, true
	}

	if cmd, handled := m.viewKey(key, *rev); handled {
		return m, cmd, true
	}

	return m.openKey(key, *rev)
}

// writeKey covers the verbs that need no argument, so they run as the key lands.
func (m *Model) writeKey(key string, rev jj.Revision) (tea.Cmd, bool) {
	targets := m.targets()

	switch key {
	case "n":
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.New(ctx, targets) }), true
	case "e":
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Edit(ctx, rev.ChangeID) }), true
	case "a":
		return m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Abandon(ctx, targets) }), true
	case "A":
		return m.act(rev.ChangeID,
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Absorb(ctx, rev.ChangeID) }), true
	case "D":
		return m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Duplicate(ctx, targets) }), true
	case "|":
		return m.parallelize(), true
	}

	return nil, false
}

// viewKey covers the verbs that read rather than write, each opening a panel or a diff.
func (m Model) viewKey(key string, rev jj.Revision) (tea.Cmd, bool) {
	switch key {
	case "E":
		return m.loadEvolog(rev.ChangeID), true
	case "v":
		return m.loadDetail(rev.ChangeID), true
	case "l":
		return m.filesKey(rev), true
	case "c":
		return m.resolveKey(rev), true
	case "P":
		targets := m.targets()

		return m.previewPush(joinIDs(targets), jj.PushOptions{Revisions: targets}), true
	case "enter":
		return m.showDiff(jj.DiffTarget{Revision: rev.ChangeID}), true
	}

	return nil, false
}

// openKey covers the verbs that need an argument, a terminal, or a view of their own.
func (m Model) openKey(key string, rev jj.Revision) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "d":
		return m, m.loadDescribe(rev), true
	case "alt+e":
		return m, m.describeInEditor(rev.ChangeID, ""), true
	case "s":
		cmd := m.openSquash(m.targets())

		return m, cmd, true
	case "r":
		cmd := m.openRebase(m.targets(), jj.RebaseScopeRevisions)

		return m, cmd, true
	case "R":
		cmd := m.openRebase(m.targets(), jj.RebaseScopeSource)

		return m, cmd, true
	case "S":
		return m, m.exec("split"+undoHint, m.hunkArgs("split", "-i", "-r", rev.ChangeID)...), true
	case "i":
		return m, m.exec("squashed interactively"+undoHint, m.hunkArgs("squash", "-i", "-r", rev.ChangeID)...), true
	case "I":
		cmd := m.openMoveHunks(rev)

		return m, cmd, true
	case "alt+d":
		cmd := m.openDiffRange()

		return m, cmd, true
	case "X":
		return m, m.exec("arranged, press u to undo", "arrange"), true
	case "B":
		next, cmd := m.openBisect()

		return next, cmd, true
	}

	return m, nil, false
}

func joinIDs(ids []string) string {
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
