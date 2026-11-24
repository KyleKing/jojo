package model

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
)

// staleMsg reports a jj call refused because the checkout is older than the operation log. It is a
// distinct message rather than a status line because the fix is one keypress, not dead-end prose.
type staleMsg struct {
	err error
}

// openStale offers the first rung of the recovery ladder — update-stale before op restore, before
// re-cloning — and jj's own error text is the detail because it names the operation the checkout
// fell behind.
func (m *Model) openStale(err error) tea.Cmd {
	return m.confirm.Open(confirm.Request{
		Key:    confirmStale,
		Title:  "the working copy is stale, run jj workspace update-stale?",
		Detail: []string{err.Error()},
	})
}

// divergentPreviewMsg carries the divergent rows, which is what the confirmation lists rather than
// asking the user to trust a count, and the change under the cursor when it is one of them.
type divergentPreviewMsg struct {
	err   error
	focus string
	sides []string
}

// previewDivergent lists the set first, because each abandoned side's live twin on the trunk is
// what stays and the user should see that before agreeing.
func (m Model) previewDivergent() tea.Cmd {
	client, ctx, focus := m.client, m.ctx, ""
	if rev := m.log.Selected(); rev != nil && rev.Divergent {
		focus = rev.ChangeID
	}

	return func() tea.Msg {
		rows, err := client.Log(ctx, jj.ConvergeRevset, 0)
		if err != nil {
			return divergentPreviewMsg{err: err}
		}

		var sides []string
		for _, row := range rows {
			if rev := row.Revision; rev != nil {
				sides = append(sides, rev.ChangeID+" "+rev.Description)
			}
		}

		return divergentPreviewMsg{sides: sides, focus: focus}
	}
}

func (m Model) offerDivergent(msg divergentPreviewMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, jj.ErrStaleWorkingCopy):
		cmd := m.openStale(msg.err)

		return m, cmd
	case msg.err != nil:
		m.fail(msg.err)

		return m, nil
	case len(msg.sides) == 0:
		return m, m.say("no divergent revisions")
	}

	m.converging = msg.focus

	what := "the divergent changes"
	if msg.focus != "" {
		what = msg.focus
	}

	cmd := m.confirm.Open(confirm.Request{
		Key:   confirmDivergent,
		Title: "converge " + what + "?",
		Detail: append(msg.sides, "",
			"y runs jj converge, which replaces each change's sides with one revision and asks",
			"in the terminal when it cannot choose a description or parents itself.",
			"a abandons the sides the trunk does not keep instead."),
		AltKey:  "a",
		AltWhat: "abandon stale sides",
	})

	return m, cmd
}

// converge hands the terminal to jj converge, scoped to the change under the cursor when it is
// divergent, since jj asks which change to work on when there are several and which description to
// keep when the sides disagree.
func (m Model) converge(change string) tea.Cmd {
	return m.exec("converged"+undoHint, convergeArgs(change)...)
}

func convergeArgs(change string) []string {
	if change == "" {
		return []string{"converge"}
	}

	return []string{"converge", "-r", "change_id(" + change + ")"}
}
