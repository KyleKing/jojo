package model

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/jj"
)

// openDiffRange starts the two-step pick for a diff between any two revisions, which is the one
// comparison jj's own log view cannot show.
func (m *Model) openDiffRange() tea.Cmd {
	m.onPick = func(m Model, from destpicker.PickedMsg) (Model, tea.Cmd) {
		cmd := m.pickDiffTo(from.Revision.ChangeID)

		return m, cmd
	}

	return m.destPicker.Open("diff from", m.rows, "", destpicker.AllowImmutable())
}

func (m *Model) pickDiffTo(from string) tea.Cmd {
	m.onPick = func(m Model, to destpicker.PickedMsg) (Model, tea.Cmd) {
		return m, m.showDiff(jj.DiffTarget{From: from, To: to.Revision.ChangeID})
	}

	return m.destPicker.Open("diff "+from+" to", m.rows, from, destpicker.AllowImmutable())
}

// diffLoadedMsg carries a diff read in the background, along with what to call it.
type diffLoadedMsg struct {
	err   error
	title string
	text  string
}

// showDiff reads the diff and opens it in a pane. An operation is the exception: `jj op show`
// prints its own header above the diff, which no diff parser reads, so it keeps the pager.
func (m Model) showDiff(target jj.DiffTarget) tea.Cmd {
	if target.Operation != "" {
		cmd, err := m.client.DiffCommand(m.ctx, target)
		if err != nil {
			return m.say(err.Error())
		}

		return m.execCommand("", cmd)
	}

	client, ctx, title := m.client, m.ctx, diffTitle(target)

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, target)

		return diffLoadedMsg{title: title, text: text, err: err}
	}
}

func diffTitle(t jj.DiffTarget) string {
	name := t.Revision
	if name == "" {
		name = t.From + " to " + t.To
	}

	if len(t.Paths) == 1 {
		return "diff " + t.Paths[0] + " in " + name
	}

	return "diff " + name
}

func (m Model) openDiff(msg diffLoadedMsg) tea.Model {
	switch {
	case msg.err != nil:
		m.message = msg.err.Error()
	case msg.text == "":
		m.message = msg.title + ": nothing changed"
	default:
		m.diff.Open(msg.title, msg.text, m.width)
	}

	return m
}

func (m Model) showOperation(msg oplogview.ShowMsg) (tea.Model, tea.Cmd) {
	return m, m.showDiff(jj.DiffTarget{Operation: msg.Operation.ID})
}

func (m Model) revertOperation(msg oplogview.RevertMsg) (tea.Model, tea.Cmd) {
	op := msg.Operation.ID

	return m, m.run(undoHint, func(ctx context.Context) (string, error) {
		return m.client.OpRevert(ctx, op)
	})
}

// bisectRange is the revset `jj bisect run` searches, built from exactly two checked revisions so
// the known-good and known-bad ends are stated rather than guessed. The log lists newest first, so
// the second checked revision is the older end and has to lead the range.
func bisectRange(checked []string) (string, bool) {
	const ends = 2
	if len(checked) != ends {
		return "", false
	}

	return checked[1] + "::" + checked[0], true
}

func (m Model) openBisect() (tea.Model, tea.Cmd) {
	rangeSet, ok := bisectRange(m.log.Checked())
	if !ok {
		return m, m.say("check the known-good and the known-bad revision with space first")
	}

	m.onSubmit = func(m Model, command string) (Model, tea.Cmd) {
		return m, m.bisect(rangeSet, command)
	}

	cmd := m.prompt.Open("test command for "+rangeSet, "")

	return m, cmd
}

// bisect runs jj's own scripted bisection rather than reimplementing the search, and hands over the
// terminal because the command it runs is the user's and may print anything.
func (m Model) bisect(rangeSet, command string) tea.Cmd {
	if strings.TrimSpace(command) == "" {
		return m.say("bisect needs a command that fails on a bad revision")
	}

	return m.exec("bisect finished", "bisect", "run", "--range", rangeSet, "--", "sh", "-c", command)
}

func (m Model) parallelize() tea.Cmd {
	targets := m.targets()
	if len(targets) < 2 { //nolint:mnd // one revision has nothing to be parallel to.
		return m.say("check two or more revisions with space first")
	}

	return m.act(joinIDs(targets),
		func(ctx context.Context, c *jj.Client) (string, error) { return c.Parallelize(ctx, targets) })
}
