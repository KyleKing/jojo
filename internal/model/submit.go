package model

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/palette"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/planview"
	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/stack"
)

// submitState is a stack submit: the plan on screen in the outline's place, read in the background
// and carried out only on Enter.
type submitState struct {
	gh *gh.CLI
	// ghMissing is why gh cannot run, nil when it can, looked up once rather than on every frame the
	// gate draws.
	ghMissing error
	// prs is the pull request each change of the last plan read had, by change id, which the outline
	// shows and the coach reads.
	prs map[string]*gh.PR
	// template names a bookmark for a change that has none; draft is the -d flag.
	template string
	revs     []jj.Revision
	view     planview.Model
	seq      int
	active   bool
	draft    bool
	// afterOutline submits once the outline the palette opened for it lands.
	afterOutline bool
}

type (
	submitReadMsg struct {
		err error
		in  stack.Input
		seq int
	}
	submitDoneMsg struct {
		err error
		res stack.Result
		seq int
	}
)

// submitPRsMsg is the stack's pull requests read again after a submit, for the outline alone.
type submitPRsMsg struct{ in stack.Input }

// submitGate refuses a submit of the outline's stack, naming every change that blocks it.
func (m Model) submitGate() keys.Availability {
	if m.submit.ghMissing != nil {
		return keys.Refuse("gh is not on PATH: a submit opens pull requests through it")
	}

	// The whole descriptions are read with the plan; a first line is enough to refuse on.
	changes, _ := stack.Trim(stack.Changes(m.outlineRevs(), nil))
	if why := stack.Refusals(changes); len(why) > 0 {
		return keys.Refuse(strings.Join(why, "; "))
	}

	return keys.Allow()
}

// outlineRevs is the outline's chapters as revisions, parent first.
func (m Model) outlineRevs() []jj.Revision {
	chapters := m.outline.view.Chapters()
	out := make([]jj.Revision, 0, len(chapters))

	for _, c := range chapters {
		out = append(out, *c.Rev)
	}

	return out
}

// startSubmit opens the plan screen over the outline's stack and reads the plan behind it.
func (m *Model) startSubmit() tea.Cmd {
	m.submit.seq++
	m.submit.active, m.submit.afterOutline = true, false
	m.submit.revs = m.outlineRevs()
	m.submit.view.SetLoading()
	m.resize()

	return m.readSubmit()
}

// readSubmit reads what the plan needs: descriptions, trunk, and every bookmark's pull request.
func (m Model) readSubmit() tea.Cmd {
	client, forge, ctx, seq := m.client, m.submit.gh, m.ctx, m.submit.seq
	revs, template, draft := m.submit.revs, m.submit.template, m.submit.draft

	return func() tea.Msg {
		in, err := stack.Read(ctx, revs, client, forge, template, draft)

		return submitReadMsg{in: in, err: err, seq: seq}
	}
}

// refreshPRs reads the stack's pull requests again once a submit has opened them, so the outline
// shows their numbers and states; a failed read leaves what it showed.
func (m Model) refreshPRs() tea.Cmd {
	client, forge, ctx := m.client, m.submit.gh, m.ctx
	revs, template := m.submit.revs, m.submit.template

	return func() tea.Msg {
		in, err := stack.Read(ctx, revs, client, forge, template, false)
		if err != nil {
			return nil
		}

		return submitPRsMsg{in: in}
	}
}

// handleSubmitMsg lands the plan's read and the submit's result.
func (m Model) handleSubmitMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case submitReadMsg:
		if msg.seq != m.submit.seq || !m.submit.active {
			return m, nil, true
		}

		if msg.err != nil {
			m.submit.view.SetErr(msg.err)

			return m, nil, true
		}

		if why := stack.Refusals(msg.in.Changes); len(why) > 0 {
			m.submit.view.SetErr(refusal(why))

			return m, nil, true
		}

		msg.in.Draft = m.submit.draft
		m.notePRs(msg.in)
		m.submit.view.SetPlan(stack.Build(msg.in))
		m.resize()

		return m, nil, true
	case submitDoneMsg:
		if msg.seq != m.submit.seq {
			return m, nil, true
		}

		return m.landSubmit(msg)
	case submitPRsMsg:
		m.notePRs(msg.in)

		return m, nil, true
	}

	return m, nil, false
}

type refusalError string

func (r refusalError) Error() string { return string(r) }

func refusal(why []string) error { return refusalError("cannot submit: " + strings.Join(why, "; ")) }

// notePRs files each change's pull request and shows it in the outline's forge column.
func (m *Model) notePRs(in stack.Input) {
	for _, c := range in.Changes {
		name := c.Bookmark
		if name == "" {
			name = stack.BookmarkName(in.Template, c)
		}

		pr := in.PRs[name]
		if pr == nil {
			continue
		}

		m.submit.prs[c.ID] = pr
	}

	m.showPRs()
}

// showPRs puts what the last plan read knew about each chapter's pull request into the outline.
func (m *Model) showPRs() {
	chapters := m.outline.view.Chapters()
	for i := range chapters {
		if pr := m.submit.prs[chapters[i].Rev.ChangeID]; pr != nil {
			chapters[i].Forge = pr.Summary()
		}
	}

	m.outline.view.SetChapters(chapters)
}

// runSubmit carries the plan out in the background.
func (m *Model) runSubmit() tea.Cmd {
	plan := m.submit.view.Plan()

	switch {
	case plan == nil || m.submit.view.Running():
		return nil
	case !plan.Writes():
		return m.say("nothing to submit: every pull request is up to date")
	}

	m.submit.view.SetRunning(true)
	client, forge, ctx, seq, p := m.client, m.submit.gh, m.ctx, m.submit.seq, *plan

	return func() tea.Msg {
		res, err := stack.Run(ctx, p, client.Apply, forge)

		return submitDoneMsg{res: res, err: err, seq: seq}
	}
}

// landSubmit closes the plan and reads the stack again, so the outline shows the pull requests the
// submit left; a failure keeps the plan up with what failed, and gm has every call.
func (m Model) landSubmit(msg submitDoneMsg) (tea.Model, tea.Cmd, bool) {
	if msg.err != nil {
		m.submit.view.SetErr(msg.err)
		m.fail(msg.err)

		return m, tea.Batch(m.loadLog(m.revset), m.readOutline(m.outline.revset)), true
	}

	m.submit.active = false
	m.submit.seq++
	m.resize()

	numbers := make([]string, 0, len(msg.res.Numbers))
	steps := m.submit.view.Plan().Open()

	for k := range steps {
		s := &steps[k]
		if n := msg.res.Numbers[s.Bookmark]; n > 0 {
			numbers = append(numbers, "#"+strconv.Itoa(n))
		}
	}

	text := "submitted " + strings.Join(numbers, " ") + " (" + strconv.Itoa(msg.res.Ran) + " calls; gm lists them)"
	cmd := tea.Batch(m.say(text), m.loadLog(m.revset), m.readOutline(m.outline.revset), m.refreshPRs())

	return m, cmd, true
}

// dispatchSubmit owns every key while the plan is up.
func (m Model) dispatchSubmit(action string) (tea.Model, tea.Cmd) {
	if key, ok := navKey(action); ok {
		m.scrollPlan(key.Code)

		return m, nil
	}

	switch action {
	case help.ActionSubmitRun:
		cmd := m.runSubmit()

		return m, cmd
	case help.ActionSubmitDraft:
		if m.submit.view.Running() {
			return m, nil
		}

		m.submit.draft = !m.submit.draft
		if p := m.submit.view.Plan(); p != nil {
			m.submit.view.SetPlan(stack.Build(stack.Input{
				Changes: changesOf(p), Skipped: p.Skipped, PRs: prsOf(p), Trunk: p.Trunk,
				Template: m.submit.template, Draft: m.submit.draft,
			}))
		}

		return m, nil
	case keys.ActionBack:
		if m.submit.view.Running() {
			return m, m.say("the submit is running; gm shows how far it got")
		}

		m.submit.active = false
		m.submit.seq++
		m.resize()

		return m, m.say("submit canceled, nothing ran")
	case keys.ActionHelp:
		m.openPalette(palette.Browse)

		return m, nil
	case keys.ActionMessages:
		m.showOutput()

		return m, nil
	case keys.ActionQuit, keys.ActionCancel:
		m.quitting = true

		return m, tea.Quit
	}

	return m, nil
}

// scrollPlan moves the plan the way a list's key would move its cursor.
func (m *Model) scrollPlan(code rune) {
	half := max(1, m.height/2) //nolint:mnd // half a page

	switch code {
	case tea.KeyDown:
		m.submit.view.Scroll(1)
	case tea.KeyUp:
		m.submit.view.Scroll(-1)
	case tea.KeyPgDown:
		m.submit.view.Scroll(half)
	case tea.KeyPgUp:
		m.submit.view.Scroll(-half)
	case tea.KeyHome:
		m.submit.view.ScrollTo(0)
	case tea.KeyEnd:
		m.submit.view.ScrollTo(-1)
	}
}

// changesOf and prsOf rebuild a plan's input from the plan, so -d replans without asking the forge
// again.
func changesOf(p *stack.Plan) []stack.Change {
	out := make([]stack.Change, 0, len(p.Steps))
	for k := range p.Steps {
		s := &p.Steps[k]
		c := s.Change
		if s.NewBookmark {
			c.Bookmark = ""
		}

		out = append(out, c)
	}

	return out
}

func prsOf(p *stack.Plan) map[string]*gh.PR {
	out := map[string]*gh.PR{}

	for k := range p.Steps {
		s := &p.Steps[k]
		if s.PR != nil {
			out[s.Bookmark] = s.PR
		}
	}

	return out
}

// submitFrame is the plan in the log's place, over a footer of the command it is and its flags.
func (m Model) submitFrame() string {
	return m.submit.view.View() + "\n" + m.submitFooter()
}

// submitFooter is the picker's footer for the plan: what Enter runs and how far it is, then the flags.
func (m Model) submitFooter() string {
	return m.submitStatus() + "\n" + m.submitChips()
}

func (m Model) submitStatus() string {
	summary, state, face := "jojo submit", "· reading the forge…", m.skin.Muted

	if p := m.submit.view.Plan(); p != nil {
		summary = planSummary(*p)
		state = "· plan"

		if m.submit.view.Running() {
			state = "· submitting…"
		}
	}

	if err := m.submit.view.Err(); err != nil {
		state, face = "· "+oneLineErr(err), m.skin.Error
	}

	room := max(0, m.width-lipgloss.Width(state)-1)
	line := m.skin.Accent.Render(table.Truncate(summary, room))

	return padLine(line+" "+face.Render(table.Truncate(state, max(0, m.width-lipgloss.Width(line)-1))), m.width)
}

// planSummary is the plan in one line: how many of each command it runs and what they do.
func planSummary(p stack.Plan) string {
	count := map[stack.Action]int{}
	for k := range p.Steps {
		s := &p.Steps[k]
		count[s.Action]++
	}

	tools := map[string]int{}
	for _, c := range p.Commands {
		tools[c.Tool()]++
	}

	parts := []string{
		strconv.Itoa(tools["jj"]) + " jj + " + strconv.Itoa(tools["gh"]) + " gh calls",
	}

	for _, a := range []stack.Action{stack.Create, stack.Update, stack.Retarget, stack.Merged} {
		if count[a] > 0 {
			parts = append(parts, strconv.Itoa(count[a])+" "+a.String())
		}
	}

	return strings.Join(parts, " · ")
}

func (m Model) submitChips() string {
	var hints []keyhint.Hint

	if h, ok := m.keys.Hint(help.ActionSubmitDraft); ok {
		h.What, h.Off, h.Priority = "draft", !m.submit.draft, prioToggle
		hints = append(hints, h)
	}

	for _, pinned := range []struct{ action, what string }{
		{help.ActionSubmitRun, "submit"}, {keys.ActionBack, "cancel"},
	} {
		if h, ok := m.keys.Hint(pinned.action); ok {
			h.What, h.Pin = pinned.what, true
			hints = append(hints, h)
		}
	}

	return padLine(keyhint.Fit(m.skin.FooterHintStyles(), hints, m.width), m.width)
}
