package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLogPagesWhenTheCursorRunsOutOfRows(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())
	m.logWindow = len(m.rows)
	m.logFull = true

	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	got := asModel(t, next)

	if got.logWindow != 2*len(m.rows) {
		t.Errorf("logWindow = %d, want %d after reaching the bottom", got.logWindow, 2*len(m.rows))
	}
}

func TestLogStaysPutWhenThePageIsNotFull(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())

	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	got := asModel(t, next)

	if got.logWindow != logLimit {
		t.Errorf("logWindow = %d, want it unchanged when the log is shorter than one page", got.logWindow)
	}
}
