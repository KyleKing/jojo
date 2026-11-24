package model

import (
	"context"
	"errors"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/jj"
)

// pickVerb is the write the in-log picker is choosing a destination for.
type pickVerb int

const (
	pickRebase pickVerb = iota
	pickSquash
	pickDuplicate
	pickNew
)

var pickVerbNames = map[pickVerb]string{
	pickRebase: "rebase", pickSquash: "squash", pickDuplicate: "duplicate", pickNew: "new",
}

// pickDebounce is how long the destination must stay put before the preview runs, so holding j
// costs one preview at the row it stops on rather than one per row it passes.
const pickDebounce = 150 * time.Millisecond

// pickState is the in-log picker: the log as it was when the verb was pressed, which the cursor
// walks, and the command the flags build against whatever the cursor is on. The preview is that
// command run in a sandbox (jj.Sandbox), never in the repository, so moving, toggling, and Esc leave
// nothing behind; Enter runs the same arguments for real, once.
type pickState struct {
	sandbox    *jj.Sandbox
	sandboxErr error
	previewErr error
	dest       string
	shownFor   string
	sources    []string
	base       []jj.LogRow
	preview    []jj.LogRow
	view       logview.Model
	session    int
	seq        int
	verb       pickVerb
	scope      jj.RebaseScope
	mode       jj.RebaseDestinationMode
	active     bool
	inPlace    bool
	skip       bool
	keep       bool
	combine    bool
	noEdit     bool
	unchanged  bool
	before     bool
}

type (
	sandboxOpenedMsg struct {
		err     error
		sandbox *jj.Sandbox
		session int
	}
	pickTickMsg struct{ session, seq int }
	previewMsg  struct {
		err     error
		command string
		preview jj.Preview
		session int
	}
)

// args is the jj invocation the picker stands for. A preview cannot hand jj a terminal, so the
// combined-message squash previews with the destination's message, which draws the same graph.
func (p *pickState) args(forPreview bool) []string {
	dest := []string{p.dest}

	switch p.verb {
	case pickSquash:
		return jj.SquashIntoArgs(jj.SquashOptions{
			Sources: p.sources, Into: p.dest, KeepEmptied: p.keep, Combine: p.combine && !forPreview,
		})
	case pickDuplicate:
		if p.inPlace {
			dest = nil
		}

		return jj.DuplicateArgs(p.sources, jj.PlaceOptions{Destination: dest, Mode: p.mode})
	case pickNew:
		parents := slices.Clone(p.sources)
		if !slices.Contains(parents, p.dest) {
			parents = append(parents, p.dest)
		}

		return jj.NewArgs(jj.PlaceOptions{Destination: parents, Mode: p.mode}, p.noEdit)
	case pickRebase:
	}

	return jj.RebaseArgs(jj.RebaseOptions{
		Revisions: p.sources, Destination: dest, Mode: p.mode, Scope: p.scope, SkipEmptied: p.skip,
	})
}

// command is what the footer shows and what a preview is keyed by.
func (p *pickState) command() string { return jj.CommandLine(p.args(false)) }

// openPicker puts the log into picking mode over sources. Nothing is written: the sandbox the
// preview runs in is a copy (see jj.Sandbox), opened behind the first frame.
func (m *Model) openPicker(verb pickVerb, sources []string) tea.Cmd {
	m.pickSession++
	m.pick = pickState{
		active: true, verb: verb, sources: sources, base: m.rows, session: m.pickSession,
		view: logview.New(logview.StylesFrom(m.skin)),
	}
	m.pick.dest = m.initialDest()
	m.pick.inPlace = verb == pickDuplicate
	m.pick.view.SetRows(m.rows)
	m.pick.view.Focus(m.pick.dest)
	m.resize()
	m.showPicked()

	client, ctx, session := m.client, m.ctx, m.pickSession

	return func() tea.Msg {
		sandbox, err := client.OpenSandbox(ctx)

		return sandboxOpenedMsg{sandbox: sandbox, err: err, session: session}
	}
}

// initialDest is where the cursor starts: below what moves for rebase and squash, where the parent
// usually is, and on the revision under the cursor for duplicate and new.
func (m *Model) initialDest() string {
	cursor := ""
	if rev := m.log.Selected(); rev != nil {
		cursor = rev.ChangeID
	}

	switch m.pick.verb {
	case pickNew:
		if len(m.pick.sources) > 0 && !slices.Contains(m.pick.sources, cursor) {
			return m.pick.sources[0]
		}

		return cursor
	case pickDuplicate:
		return cursor
	case pickRebase, pickSquash:
	}

	ids := revisionIDs(m.rows)
	last := -1

	for i, id := range ids {
		if slices.Contains(m.pick.sources, id) {
			last = i
		}
	}

	for _, i := range append(seq(last+1, len(ids)), seqDown(last-1)...) {
		if !slices.Contains(m.pick.sources, ids[i]) {
			return ids[i]
		}
	}

	return cursor
}

func seq(from, to int) []int {
	out := []int{}
	for i := from; i < to; i++ {
		out = append(out, i)
	}

	return out
}

func seqDown(from int) []int {
	out := []int{}
	for i := from; i >= 0; i-- {
		out = append(out, i)
	}

	return out
}

func revisionIDs(rows []jj.LogRow) []string {
	ids := make([]string, 0, len(rows))

	for _, row := range rows {
		if row.Revision != nil {
			ids = append(ids, row.Revision.ChangeID)
		}
	}

	return ids
}

// closePicker leaves picking mode. The sandbox goes in a command, since Close waits out a preview
// that is still running.
func (m *Model) closePicker() tea.Cmd {
	sandbox := m.pick.sandbox
	m.pick = pickState{}
	m.resize()

	if sandbox == nil {
		return nil
	}

	return closeSandbox(sandbox)
}

// handleModeMsg covers the background work of the picker and of time travel.
func (m Model) handleModeMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if next, cmd, handled := m.handlePickMsg(msg); handled {
		return next, cmd, true
	}

	return m.handleTravelMsg(msg)
}

// handlePickMsg covers the picker's own background work: the sandbox arriving, the debounce
// expiring, and a preview landing. Anything for a session that has since closed is dropped, and a
// sandbox that arrives for one is closed rather than leaked.
func (m Model) handlePickMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case sandboxOpenedMsg:
		if !m.pick.active || msg.session != m.pick.session {
			return m, closeSandbox(msg.sandbox), true
		}

		m.pick.sandbox, m.pick.sandboxErr = msg.sandbox, msg.err
		cmd := m.runPreview()

		return m, cmd, true
	case pickTickMsg:
		if !m.pick.active || msg.session != m.pick.session || msg.seq != m.pick.seq {
			return m, nil, true
		}

		cmd := m.runPreview()

		return m, cmd, true
	case previewMsg:
		if m.pick.active && msg.session == m.pick.session && msg.command == m.pick.command() {
			m.pick.shownFor, m.pick.previewErr, m.pick.unchanged = msg.command, msg.err, msg.preview.Unchanged
			if msg.err == nil {
				m.pick.preview = msg.preview.Rows
				fillPreviewStats(m.pick.base, m.pick.preview)
			}

			m.showPicked()
		}

		return m, nil, true
	}

	return m, nil, false
}

// fillPreviewStats gives each previewed revision the size its change had in the log. A rebase moves
// a change's own diff along with it, so the size is the one it will have unless the move conflicts,
// and a row without one would read as though the preview had emptied it.
func fillPreviewStats(base, preview []jj.LogRow) {
	sizes := map[string]*jj.Revision{}

	for _, row := range base {
		if row.Revision != nil {
			sizes[row.Revision.ChangeID] = row.Revision
		}
	}

	for _, row := range preview {
		if was, ok := sizes[rowID(row)]; ok {
			row.Revision.Added, row.Revision.Removed, row.Revision.Files = was.Added, was.Removed, was.Files
		}
	}
}

func rowID(row jj.LogRow) string {
	if row.Revision == nil {
		return ""
	}

	return row.Revision.ChangeID
}

func closeSandbox(sandbox *jj.Sandbox) tea.Cmd {
	if sandbox == nil {
		return nil
	}

	return func() tea.Msg {
		// A sandbox that will not delete is swept on a later start, so only a link that changed is
		// worth the footer: a preview may have written into the repository, and jj op log says.
		if err := sandbox.Close(); errors.Is(err, jj.ErrLinkRewritten) {
			return messageMsg{text: err.Error() + "; check jj op log", output: err.Error()}
		}

		return nil
	}
}

// runPreview runs the current command in the sandbox. It copies what it needs first, since the
// closure runs off the update loop.
func (m Model) runPreview() tea.Cmd {
	sandbox := m.pick.sandbox
	if sandbox == nil {
		return nil
	}

	args, command, session := m.pick.args(true), m.pick.command(), m.pick.session
	revset, limit, ctx := m.revset, m.logWindow, m.ctx

	return func() tea.Msg {
		preview, err := sandbox.Preview(ctx, args, revset, limit)

		return previewMsg{preview: preview, err: err, command: command, session: session}
	}
}

// changed is every edit to the picker's command: the debounce restarts, and the view shows the
// newest preview it has with the new destination marked until the right one lands.
func (m *Model) changed() tea.Cmd {
	m.pick.seq++
	m.showPicked()

	session, seq := m.pick.session, m.pick.seq

	return tea.Tick(pickDebounce, func(time.Time) tea.Msg { return pickTickMsg{session: session, seq: seq} })
}

// showPicked draws whichever log the picker is showing: the preview once one has landed and Tab has
// not asked for the log as it is, else the log as it is.
func (m *Model) showPicked() {
	after := !m.pick.before && m.pick.preview != nil
	rows := m.pick.base

	if after {
		rows = m.pick.preview
	}

	m.pick.view.SetRows(rows)
	m.pick.view.SetMarks(pickMarks(&m.pick, after))

	if !m.pick.view.Focus(m.pick.dest) && len(m.pick.sources) > 0 {
		m.pick.view.Focus(m.pick.sources[0])
	}
}

// destWords is the chip on the destination, naming the flag that places what moves against it.
var destWords = map[jj.RebaseDestinationMode]string{
	jj.RebaseDestOnto:         "◀ onto",
	jj.RebaseDestInsertAfter:  "◀ after",
	jj.RebaseDestInsertBefore: "◀ before",
}

// sourceWords is the chip on what the command is about, before it runs.
var sourceWords = map[pickVerb]string{
	pickRebase: "moving", pickSquash: "squashing", pickDuplicate: "copying", pickNew: "parent",
}

// pickMarks labels the rows the command is about. Before it runs that is what moves and where to;
// in the preview it is also what the command rewrote on the way and what it created, found by
// comparing each row's commit with the one the log had for that change.
func pickMarks(p *pickState, after bool) map[string]logview.Mark {
	marks := map[string]logview.Mark{}
	sourceWord := sourceWords[p.verb]

	if after {
		markPreview(p, marks)
	} else {
		for _, id := range p.sources {
			marks[id] = logview.Mark{Text: sourceWord}
		}
	}

	switch {
	case p.verb == pickSquash:
		marks[p.dest] = logview.Mark{Text: "◀ into"}
	case p.verb == pickDuplicate && p.inPlace:
	default:
		marks[p.dest] = logview.Mark{Text: destWords[p.mode]}
	}

	return marks
}

func markPreview(p *pickState, marks map[string]logview.Mark) {
	was := map[string]string{}

	for _, row := range p.base {
		if row.Revision != nil {
			was[row.Revision.ChangeID] = row.Revision.CommitID
		}
	}

	for _, row := range p.preview {
		rev := row.Revision
		if rev == nil {
			continue
		}

		commit, known := was[rev.ChangeID]

		switch {
		case !known:
			marks[rev.ChangeID] = logview.Mark{Text: "new"}
		case slices.Contains(p.sources, rev.ChangeID) && p.verb == pickRebase:
			marks[rev.ChangeID] = logview.Mark{Text: "moved"}
		case commit != rev.CommitID:
			marks[rev.ChangeID] = logview.Mark{Text: "rewritten", Muted: true}
		}
	}
}

// applyPick runs the command the picker built, as one operation, and leaves picking mode.
func (m Model) applyPick() (Model, tea.Cmd) {
	pick := m.pick
	args, what := pick.args(false), joinIDs(pick.sources)
	closing := m.closePicker()

	if pick.verb == pickSquash && pick.combine {
		return m, tea.Batch(closing, m.exec("squashed "+what+undoHint, args...))
	}

	if what == "" {
		what = pick.dest
	}

	act := m.act(what, func(ctx context.Context, c *jj.Client) (string, error) { return c.Apply(ctx, args) })

	return m, tea.Batch(closing, act)
}
