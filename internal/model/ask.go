package model

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/jj"
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

// abandon drops the targets at once when none of them changes anything, and otherwise asks first,
// naming what goes with them: an abandon of work is one key from gone, and u is a way back only for
// whoever knows to press it.
func (m *Model) abandon(targets []string) tea.Cmd {
	run := func(m Model) (Model, tea.Cmd) {
		cmd := m.act(joinIDs(targets),
			func(ctx context.Context, c *jj.Client) (string, error) { return c.Abandon(ctx, targets) })

		return m, cmd
	}

	held := m.contentOf(targets)
	if held.losing == 0 {
		_, cmd := run(*m)

		return cmd
	}

	m.openAsk(ask{
		question: "abandon " + abandonNames(targets) + " and " + filesLabel(len(targets), held) + "?",
		verb:     "abandon",
		declined: "kept " + abandonNames(targets),
		undoable: true,
		run:      run,
	})

	return nil
}

// content is what an abandon would take with it: how many targets change something, and the files
// they change, which counted says are all known.
type content struct {
	losing, files int
	counted       bool
}

// contentOf counts the targets that change something, and the files they change. A target the log
// has not read, or whose stats have not landed, counts as changing something and leaves the count
// unknown, so nothing claims a file count it does not have.
func (m Model) contentOf(targets []string) content {
	c := content{counted: true}

	for _, id := range targets {
		rev := m.revisionByChange(id)

		switch {
		case rev == nil, !rev.Empty && rev.Files == 0:
			c.losing++
			c.counted = false
		case rev.Empty:
		default:
			c.losing++
			c.files += rev.Files
		}
	}

	return c
}

// revisionByChange is the log's revision with change id prefix id, nil when the log holds none.
func (m Model) revisionByChange(id string) *jj.Revision {
	for _, row := range m.rows {
		if row.Revision != nil && sameChange(row.Revision.ChangeID, id) {
			return row.Revision
		}
	}

	return nil
}

// abandonListed is how many targets an abandon question names before it counts the rest.
const abandonListed = 3

// abandonNames names the targets of an abandon for a sentence, counting past the first few.
func abandonNames(targets []string) string {
	if len(targets) <= abandonListed {
		return joinNames(targets)
	}

	return strings.Join(targets[:abandonListed-1], ", ") + " and " + strconv.Itoa(len(targets)-abandonListed+1) +
		" more"
}

// filesLabel says what an abandon of n changes takes with it: their changed files, counted when the
// count is known.
func filesLabel(n int, held content) string {
	its := "its"
	if n > 1 {
		its = "their"
	}

	switch {
	case !held.counted:
		return its + " changes"
	case held.files == 1:
		return its + " 1 changed file"
	}

	return its + " " + strconv.Itoa(held.files) + " changed files"
}
