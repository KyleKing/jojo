package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/confirm"
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
