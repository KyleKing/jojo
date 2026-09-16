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

// targets is the checked revisions, or the one under the cursor. Batch and single share this path,
// so a multi-select cannot silently act on one revision.
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

// openKey covers the verbs that need an argument, a terminal, or a view of their own.
func (m Model) openKey(key string, rev jj.Revision) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "d":
		cmd := m.openDescribe(rev)

		return m, cmd, true
	case "s":
		cmd := m.openSquash(rev)

		return m, cmd, true
	case "r":
		cmd := m.openRebase(rev, pendingRebaseRevisions)

		return m, cmd, true
	case "R":
		cmd := m.openRebase(rev, pendingRebaseSource)

		return m, cmd, true
	case "E":
		return m, m.loadEvolog(rev.ChangeID), true
	case "v":
		return m, m.loadDetail(rev.ChangeID), true
	case "S":
		return m, m.exec("split"+undoHint, "split", "-i", "-r", rev.ChangeID), true
	case "i":
		return m, m.exec("squashed interactively"+undoHint, "squash", "-i", "-r", rev.ChangeID), true
	case "enter":
		return m, m.showDiff(jj.DiffTarget{Revision: rev.ChangeID}), true
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
	revset := m.revset
	// The repaint covers whatever jj printed, so the message has to name the command that failed.
	what := strings.Join(cmd.Args, " ")

	return tea.Sequence(
		tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return messageMsg{text: what + ": " + err.Error()}
			}

			return messageMsg{text: done}
		}),
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
	case err != nil:
		return messageMsg{text: err.Error()}
	}

	return messageMsg{text: ran + hint}
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

// undo records where the repository was first, because jj has no redo: going forward again means
// restoring the operation the undo left.
func (m Model) undo() tea.Cmd {
	client, ctx := m.client, m.ctx

	return tea.Sequence(
		func() tea.Msg {
			ops, err := client.OpLog(ctx, 1)
			if err != nil || len(ops) == 0 {
				return nil
			}

			return redoPointMsg{id: ops[0].ID}
		},
		m.run(", press ctrl+r to redo", client.Undo),
	)
}

func (m Model) redo() tea.Cmd {
	if m.redoOp == "" {
		return m.say("nothing to redo")
	}

	op := m.redoOp

	return m.run("redone"+undoHint, func(ctx context.Context) (string, error) {
		return m.client.OpRestore(ctx, op)
	})
}
