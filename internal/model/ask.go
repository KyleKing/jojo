package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/ui"
)

// ask is a one-line question in the footer that a write with something to lose waits on. The y key
// answers it; any other key declines it and is swallowed rather than replayed, so a reflex keystroke can
// neither confirm the write nor run as a command of its own once it is declined.
type ask struct {
	// run is the write itself.
	run func(Model) (Model, tea.Cmd)
	// question names what the write would do, and to what.
	question string
	// verb is what y does, for its hint.
	verb string
	// declined is what the footer says when any other key answers.
	declined string
	// undoable says the write lands in the operation log, so the footer can say u takes it back.
	undoable bool
}

// keyYes is the one key that answers an ask.
const keyYes = "y"

// openAsk puts a question in the footer, which takes the next key.
func (m *Model) openAsk(a ask) {
	m.asking = &a
	m.message, m.failing = "", false
}

// answer takes the key an open ask waits on: y runs the write, and anything else keeps things as
// they are.
func (m Model) answer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	a := *m.asking
	m.asking = nil

	if msg.String() != keyYes {
		return m, m.say(a.declined)
	}

	return a.run(m)
}

// askFooter is the footer while an ask is open: the question, cut to leave room for its keys, then
// the keys themselves, which are never dropped.
func (m Model) askFooter() string {
	hints := []keyhint.Hint{{Key: keyYes, What: m.asking.verb}, {Key: "n", What: "keep"}}
	if m.asking.undoable {
		hints = append(hints, keyhint.Hint{Key: "u", What: "undoes it"})
	}

	keys := keyhint.Line(m.skin.FooterHintStyles(), hints)
	if lipgloss.Width(keys) > m.width {
		keys = ui.FitHints(m.skin.FooterHintStyles(), hints[:2], m.width)
	}

	room := m.width - lipgloss.Width(keys) - len(askGap)
	line := keys

	if room > 0 {
		line = m.skin.Accent.Render(table.Truncate(m.asking.question, room)) + askGap + keys
	}

	return padLine(line, m.width)
}

const askGap = "  "

// joinNames lists names for a sentence: "a", "a and b", "a, b and c".
func joinNames(names []string) string {
	if len(names) < 2 { //nolint:mnd // one name needs no joining
		return strings.Join(names, "")
	}

	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
