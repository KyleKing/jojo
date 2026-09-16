package model

import (
	"context"
	"errors"
	"os/exec"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
)

// targets is what an action acts on: the checked revisions when any are
// checked, otherwise the one under the cursor. Batch and single are the same
// code path, so a multi-select cannot silently act on one revision.
func (m Model) targets() []string {
	if checked := m.log.Checked(); len(checked) > 0 {
		return checked
	}

	if rev := m.log.Selected(); rev != nil {
		return []string{rev.ChangeID}
	}

	return nil
}

// handleActionKey runs the per-commit verbs. Every one of them is reversible
// through the operation log, so each executes immediately and reports an undo
// hint rather than opening a confirm gate.

func (m Model) handleActionKey(key string) (tea.Model, tea.Cmd, bool) {
	rev := m.log.Selected()
	if rev == nil {
		return m, nil, false
	}

	switch key {
	case "n":
		return m, m.run(undoable("new change on "+rev.ChangeID), func(ctx context.Context) error {
			return m.client.New(ctx, m.targets())
		}), true
	case "e":
		if rev.Immutable {
			return m, m.say(rev.ChangeID + " is immutable"), true
		}

		return m, m.run(undoable("editing "+rev.ChangeID), func(ctx context.Context) error {
			return m.client.Edit(ctx, rev.ChangeID)
		}), true
	case "a":
		return m, m.run(undoable("abandoned "+joinIDs(m.targets())), func(ctx context.Context) error {
			return m.client.Abandon(ctx, m.targets())
		}), true
	case "A":
		return m, m.run(undoable("absorbed "+rev.ChangeID), func(ctx context.Context) error {
			return m.client.Absorb(ctx, rev.ChangeID)
		}), true
	case "D":
		return m, m.run(undoable("duplicated "+joinIDs(m.targets())), func(ctx context.Context) error {
			return m.client.Duplicate(ctx, m.targets())
		}), true
	case "d":
		if rev.Immutable {
			return m, m.say(rev.ChangeID + " is immutable"), true
		}

		cmd := m.openDescribe(*rev)

		return m, cmd, true
	case "s":
		cmd := m.openSquash(*rev)

		return m, cmd, true
	case "r":
		cmd := m.openRebase(*rev, pendingRebaseRevisions)

		return m, cmd, true
	case "R":
		cmd := m.openRebase(*rev, pendingRebaseSource)

		return m, cmd, true
	case "E":
		return m, m.loadEvolog(rev.ChangeID), true
	case "S":
		return m, m.exec(undoable("split"), "split", "-i", "-r", rev.ChangeID), true
	}

	return m, nil, false
}

func joinIDs(ids []string) string {
	if len(ids) == 1 {
		return ids[0]
	}

	return ids[0] + " and " + strconv.Itoa(len(ids)-1) + " more"
}

// exec suspends the TUI and hands the terminal to jj, which is how the
// interactive verbs reach jj's configured diff editor.
func (m Model) exec(done string, args ...string) tea.Cmd {
	//nolint:gosec // G204: the binary is a literal and args are built here, never from user input.
	cmd := exec.CommandContext(m.ctx, "jj", args...)
	cmd.Dir = m.repoPath
	revset := m.revset

	return tea.Sequence(
		tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return messageMsg{text: err.Error()}
			}

			return messageMsg{text: done}
		}),
		m.loadLog(revset),
	)
}

func (Model) say(text string) tea.Cmd {
	return func() tea.Msg { return messageMsg{text: text} }
}

// undoable adds the hint that replaces a confirm gate, because the operation
// log makes every one of these reversible.
func undoable(done string) string {
	return done + ", press u to undo"
}

// run executes a jj write then reloads the log.
func (m Model) run(done string, mutate func(context.Context) error) tea.Cmd {
	revset, ctx := m.revset, m.ctx

	return tea.Sequence(
		func() tea.Msg {
			switch err := mutate(ctx); {
			case errors.Is(err, jj.ErrNothingChanged):
				return messageMsg{text: "nothing changed"}
			case err != nil:
				return messageMsg{text: err.Error()}
			}

			return messageMsg{text: done}
		},
		m.loadLog(revset),
	)
}

// undo records where the repository was before undoing, because jj has no redo
// command: the way forward again is restoring to the operation undo left.
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
		m.run("undone, press ctrl+r to redo", client.Undo),
	)
}

func (m Model) redo() tea.Cmd {
	if m.redoOp == "" {
		return m.say("nothing to redo")
	}

	op := m.redoOp

	return m.run(undoable("redone"), func(ctx context.Context) error {
		return m.client.OpRestore(ctx, op)
	})
}
