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

// openStale offers the first rung of the recovery ladder: update-stale before op restore, which
// before re-cloning. jj's own error text is the detail because it names the operation the checkout
// fell behind.
func (m *Model) openStale(err error) tea.Cmd {
	return m.confirm.Open(confirm.Request{
		Key:    confirmStale,
		Title:  "the working copy is stale, run jj workspace update-stale?",
		Detail: []string{err.Error()},
	})
}

// divergentPreviewMsg carries the rows an abandon would remove, which is what the confirmation
// lists rather than asking the user to trust a count.
type divergentPreviewMsg struct {
	err   error
	sides []string
}

// previewDivergent lists the set first, because each abandoned side's live twin on the trunk is
// what stays and the user should see that before agreeing.
func (m Model) previewDivergent() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		rows, err := client.Log(ctx, jj.DivergentRevset, 0)
		if err != nil {
			return divergentPreviewMsg{err: err}
		}

		var sides []string
		for _, row := range rows {
			if rev := row.Revision; rev != nil {
				sides = append(sides, rev.ChangeID+" "+rev.Description)
			}
		}

		return divergentPreviewMsg{sides: sides}
	}
}

func (m Model) offerDivergent(msg divergentPreviewMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, jj.ErrStaleWorkingCopy):
		return m, m.openStale(msg.err)
	case msg.err != nil:
		m.message = msg.err.Error()

		return m, nil
	case len(msg.sides) == 0:
		return m, m.say("no divergent revisions")
	}

	cmd := m.confirm.Open(confirm.Request{
		Key:    confirmDivergent,
		Title:  "abandon the divergent sides?",
		Detail: msg.sides,
	})

	return m, cmd
}
