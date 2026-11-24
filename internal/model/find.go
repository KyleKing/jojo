package model

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/search"
)

// findRows is the height the search bar takes out of the log while it is open.
const findRows = 1

// handleFindMsg covers the search bar, which acts on the log rather than jj.
func (m Model) handleFindMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case search.ChangedMsg:
		m.find.SetMatches(m.log.SetSearch(msg.Pattern))
		cmd := m.followDock(m.log.Selected())

		return m, cmd, true
	case search.SteppedMsg:
		m.find.SetMatches(m.log.StepMatch(msg.Delta))
		cmd := m.followDock(m.log.Selected())

		return m, cmd, true
	case search.AcceptedMsg:
		m.resize()

		return m, nil, true
	case search.CancelledMsg:
		m.log.SetSearch("")
		m.resize()

		return m, nil, true
	}

	return m, nil, false
}
