package model

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/seen"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/jj"
)

// openCompare picks the other side of a diff ending at rev, which is the one comparison jj's own
// log view cannot show.
func (m *Model) openCompare(rev string) tea.Cmd {
	m.onPick = func(m Model, from destpicker.PickedMsg) (Model, tea.Cmd) {
		return m, m.showDiff(jj.DiffTarget{From: from.Revision.ChangeID, To: rev})
	}

	return m.destPicker.Open("diff "+rev+" against", m.rows, rev, destpicker.AllowImmutable())
}

// diffLoadedMsg carries a diff read in the background, along with what to call it. A change's whole
// diff also carries its id, the commit it was read at, and its hunks, which the read marks key on.
type diffLoadedMsg struct {
	err    error
	title  string
	text   string
	change string
	commit string
	// revision is the one revision the diff is of, "" for a comparison or an operation.
	revision string
	hunks    []seen.Ref
}

// showDiff reads the diff and opens it in a pane. An operation's diff opens there too, its header and
// changed commits drawn as prose between the patches: handed to jj's pager instead, a pager that
// quits on a short page (less -F) flashed it and returned before it could be read.
func (m Model) showDiff(target jj.DiffTarget) tea.Cmd { return m.loadDiff(target, "") }

// showRevisionDiff is a revision's whole diff full screen, its hunks named so space, ]u, [u, and U
// keep marks there as they do in the dock.
func (m Model) showRevisionDiff(rev jj.Revision) tea.Cmd {
	return m.loadDiff(jj.DiffTarget{Revision: rev.ChangeID}, rev.CommitID)
}

// loadDiff reads target's diff for the full-screen pane. One revision's whole diff, with no paths
// and no other side, is a change the read marks apply to, read at commit when the caller knows it.
func (m Model) loadDiff(target jj.DiffTarget, commit string) tea.Cmd {
	client, ctx, title := m.client, m.ctx, diffTitle(target)
	whole := target.Revision != "" && target.From == "" && target.To == "" && target.Operation == "" &&
		len(target.Paths) == 0

	return func() tea.Msg {
		text, err := client.GitDiff(ctx, target)

		msg := diffLoadedMsg{title: title, text: text, err: err, revision: target.Revision}
		if whole && err == nil {
			msg.change, msg.commit, msg.hunks = target.Revision, commit, seen.Parse([]byte(text))
		}

		return msg
	}
}

func diffTitle(t jj.DiffTarget) string {
	name := t.Revision

	switch {
	case t.Operation != "":
		return "operation " + t.Operation
	case name == "":
		name = t.From + " to " + t.To
	}

	if len(t.Paths) == 1 {
		return "diff " + t.Paths[0] + " in " + name
	}

	return "diff " + name
}

func (m Model) openDiff(msg diffLoadedMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, jj.ErrStaleWorkingCopy):
		cmd := m.openStale(msg.err)

		return m, cmd
	case msg.err != nil:
		m.fail(msg.err)
	case msg.text == "":
		m.message = msg.title + ": nothing changed"
	case msg.change != "":
		m.diffRevision = msg.revision
		m.diff.SetHints(diffHints(m.keys, !m.marksOff()))
		m.noteRead(msg.change, msg.commit, msg.hunks)
		m.showChange(m.fullReads(), msg.change, msg.title, msg.text, "")
	default:
		m.diffRevision = msg.revision
		m.diff.SetHints(diffHints(m.keys, false))
		leaveChange(m.fullReads())
		m.marks.full.title = msg.title
		m.diff.Open(msg.title, msg.text, m.width)
	}

	return m, nil
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
