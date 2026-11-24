package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/logview"
)

// A label jump (') puts a one- or two-letter label in the gutter of every revision on screen, the
// way flash.nvim and hop do, and typing a label lands the cursor there: two or three keys to any row
// in view, however far down. In a picker it moves the destination. Esc, or a key no label starts
// with, puts the labels away.

// labelKeys are the letters labels are made of, home row first.
const labelKeys = "asdfghjklqwertyuiopzxcvbnm"

// labeling is a label jump waiting for its label: each label's change id, and what has been typed.
type labeling struct {
	targets map[string]string
	typed   string
}

// labelsFor names n rows: single letters while they last, then pairs, so every label in one jump is
// the same length and no label is the start of another.
func labelsFor(n int) []string {
	keys := strings.Split(labelKeys, "")
	if n <= len(keys) {
		return keys[:n]
	}

	out := make([]string, 0, n)

	for _, a := range keys {
		for _, b := range keys {
			if len(out) == n {
				return out
			}

			out = append(out, a+b)
		}
	}

	return out
}

// labelView is the log the jump labels: the picker's while one is open.
func (m *Model) labelView() *logview.Model {
	if m.pick.active {
		return &m.pick.view
	}

	return &m.log
}

// startLabels labels every revision on screen.
func (m Model) startLabels() (Model, tea.Cmd) {
	view := m.labelView()
	ids := view.Visible()

	if len(ids) == 0 {
		return m, m.say("nothing on screen to jump to")
	}

	m.labels = labeling{targets: map[string]string{}}
	shown := map[string]string{}

	for i, label := range labelsFor(len(ids)) {
		m.labels.targets[label] = ids[i]
		shown[ids[i]] = label
	}

	view.SetLabels(shown)

	return m, nil
}

// labelsShown reports whether a label jump is waiting for its label.
func (m Model) labelsShown() bool { return m.labels.targets != nil }

// stopLabels puts the labels away.
func (m *Model) stopLabels() {
	m.labels = labeling{}
	m.log.SetLabels(nil)
	m.pick.view.SetLabels(nil)
}

// labelKey takes a key while the labels are up: a label's letters jump, Esc or anything else puts
// them away.
func (m Model) labelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	typed := m.labels.typed + key

	if key == keyEsc || len(key) != 1 {
		m.stopLabels()

		return m, nil
	}

	if id, ok := m.labels.targets[typed]; ok {
		m.stopLabels()

		if m.pick.active {
			if id == m.pick.dest {
				return m, nil
			}

			m.pick.dest, m.pick.inPlace = id, false
			m.showPicked()
			cmd := m.changed()

			return m, cmd
		}

		return m.jumpTo(id, id)
	}

	for label := range m.labels.targets {
		if strings.HasPrefix(label, typed) {
			m.labels.typed = typed

			return m, nil
		}
	}

	m.stopLabels()

	return m, m.say("no label " + typed)
}
