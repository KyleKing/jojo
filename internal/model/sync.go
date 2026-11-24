package model

import (
	"context"
	"errors"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// A sync is a fetch that says what it moved. jj never merges on a fetch, so what a reader needs after
// one is the list of what changed under them: bookmarks that moved, changes that are now someone
// else's (immutable), conflicts that appeared, and whether trunk() moved, which is when the stacks
// built on the old trunk want restacking. That restack is many rebases in one operation, so it waits
// for y.

// syncReport is what one fetch changed.
type syncReport struct {
	err error
	// ran is the fetch's command line, from the trail.
	ran    string
	revset string
	// trunkFrom and trunkTo are trunk()'s change id before and after.
	trunkFrom string
	trunkTo   string
	// rows is the log after the fetch, under the revset it was asked for.
	rows []jj.LogRow
	// moved is each bookmark that changed, "name  old → new" or "name  new" or "name  deleted".
	moved []string
	// immutable and conflicted are change ids in the log that became so on the fetch.
	immutable  []string
	conflicted []string
	// stacks are the roots a restack would move, read once trunk moved.
	stacks []string
}

// trunkMoved reports whether the fetch moved trunk().
func (r syncReport) trunkMoved() bool { return r.trunkFrom != r.trunkTo }

// restackRoot is the first stack a restack would move, "" when trunk did not move under one.
func (r syncReport) restackRoot() string {
	if !r.trunkMoved() || len(r.stacks) == 0 {
		return ""
	}

	return r.stacks[0]
}

// syncedMsg is a sync's report landing.
type syncedMsg struct{ report syncReport }

// sync fetches (every remote, or only the tracked bookmarks) and reads what the fetch changed. The
// reads before and after are trailed like every other call, so gm lists the whole sync.
func (m Model) sync(tracked bool) tea.Cmd {
	client, ctx, revset, limit := m.client, m.ctx, m.revset, m.logWindow

	return func() tea.Msg {
		r := syncReport{revset: revset}

		before, err := client.Bookmarks(ctx)
		if err != nil {
			return syncedMsg{report: syncReport{err: err}}
		}

		r.trunkFrom, _ = client.Trunk(ctx) //nolint:errcheck // an unreadable trunk reads as unmoved

		fetch := client.Fetch
		if tracked {
			fetch = client.FetchTracked
		}

		if r.ran, err = fetch(ctx); err != nil && !isNothingChanged(err) {
			return syncedMsg{report: syncReport{err: err, ran: r.ran}}
		}

		after, err := client.Bookmarks(ctx)
		if err != nil {
			return syncedMsg{report: syncReport{err: err}}
		}

		r.moved = bookmarkMoves(before, after)
		r.trunkTo, _ = client.Trunk(ctx) //nolint:errcheck // as above

		if r.trunkMoved() {
			r.stacks, _ = client.StackRoots(ctx) //nolint:errcheck // no stacks read is no restack offered
		}

		r.rows, err = client.Log(ctx, revset, limit)
		r.err = err

		return syncedMsg{report: r}
	}
}

func isNothingChanged(err error) bool { return errors.Is(err, jj.ErrNothingChanged) }

// bookmarkMoves is every bookmark ref whose target the fetch changed, local and remote alike, in the
// order jj lists them.
func bookmarkMoves(before, after []jj.Bookmark) []string {
	was := map[string]string{}
	for _, b := range before {
		was[b.Ref()] = b.ChangeID
	}

	var out []string

	seen := map[string]bool{}

	for _, b := range after {
		ref := b.Ref()
		seen[ref] = true

		switch old, ok := was[ref]; {
		case !ok:
			out = append(out, ref+"  new at "+b.ChangeID)
		case old != b.ChangeID:
			out = append(out, ref+"  "+old+" → "+b.ChangeID)
		}
	}

	for _, b := range before {
		if !seen[b.Ref()] {
			out = append(out, b.Ref()+"  deleted")
		}
	}

	return out
}

// landSync compares the log before the fetch with the one after, puts the new log up, and opens the
// report: a confirmation offering the restack when trunk moved under a stack, a plain page otherwise.
func (m Model) landSync(r syncReport) (Model, tea.Cmd) {
	if r.err != nil {
		m.fail(r.err)

		return m, m.loadLog(m.revset)
	}

	r.immutable, r.conflicted = becameSo(m.rows, r.rows)
	m.synced = r

	next, cmd := m.applyLog(logLoadedMsg{revset: r.revset, rows: r.rows})
	m = next

	title, lines := syncLines(r)

	if !r.trunkMoved() || len(r.stacks) == 0 {
		return m, tea.Batch(cmd, m.output.Open(title, strings.Join(lines, "\n")))
	}

	lines = append(lines, "", "y restacks "+plural(len(r.stacks), "stack")+" onto the new trunk:",
		"  "+jj.CommandLine(jj.RestackArgs()))

	open := m.confirm.Open(confirm.Request{
		Key: confirmSync, Title: title, Detail: lines,
		Danger: "the restack rewrites every mutable stack in one operation; u undoes it",
	})

	return m, tea.Batch(cmd, open)
}

// becameSo is the change ids in after that are immutable or conflicted and were not in before.
func becameSo(before, after []jj.LogRow) ([]string, []string) {
	type state struct{ immutable, conflict bool }

	was := map[string]state{}

	for _, row := range before {
		if row.Revision != nil {
			was[row.Revision.ChangeID] = state{row.Revision.Immutable, row.Revision.Conflict}
		}
	}

	var immutable, conflicted []string

	for _, row := range after {
		rev := row.Revision
		if rev == nil {
			continue
		}

		old, known := was[rev.ChangeID]
		if rev.Immutable && known && !old.immutable {
			immutable = append(immutable, rev.ChangeID)
		}

		if rev.Conflict && !old.conflict {
			conflicted = append(conflicted, rev.ChangeID)
		}
	}

	return immutable, conflicted
}

// syncLines is the report's title and body.
func syncLines(r syncReport) (string, []string) {
	title := "fetched: nothing moved"
	if n := len(r.moved); n > 0 {
		title = "fetched: " + plural(n, "bookmark") + " moved"
	}

	lines := []string{r.ran}

	section := func(head string, items []string) {
		if len(items) == 0 {
			return
		}

		lines = append(lines, "", head)
		for _, it := range items {
			lines = append(lines, "  "+it)
		}
	}

	section("bookmarks", r.moved)

	if r.trunkMoved() {
		lines = append(lines, "", "trunk() moved: "+r.trunkFrom+" → "+r.trunkTo+"  (gt jumps there)")
	}

	section("now immutable (on a remote's trunk)", r.immutable)
	section("now conflicted", r.conflicted)

	if len(lines) == 1 {
		lines = append(lines, "", "nothing in this repository changed")
	}

	return title, lines
}

// restack runs the rebase the sync report offered.
func (m Model) restack() tea.Cmd {
	return m.act("the stacks", func(ctx context.Context, c *jj.Client) (string, error) { return c.Restack(ctx) })
}

// gotoTrunkMsg is trunk()'s change id, read for gt.
type gotoTrunkMsg struct {
	err error
	id  string
}

// gotoTrunk reads where trunk() is, for gt to move the log's cursor or the picker's destination there.
func (m Model) gotoTrunk() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		id, err := client.Trunk(ctx)

		return gotoTrunkMsg{id: id, err: err}
	}
}

// landTrunk moves to trunk() once it is read: the picker's destination while one is open, else the
// log's cursor.
func (m Model) landTrunk(msg gotoTrunkMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.fail(msg.err)

		return m, nil
	}

	if m.pick.active {
		if !slices.Contains(revisionIDs(m.pick.base), msg.id) {
			return m, m.say("trunk() is " + msg.id + ", which this revset does not show; L edits it")
		}

		if m.pick.dest == msg.id {
			return m, nil
		}

		m.pick.dest = msg.id
		m.pick.inPlace = false
		cmd := m.changed()

		return m, cmd
	}

	return m.jumpTo(msg.id, "trunk()")
}

// jumpTo puts the log's cursor on id, or says why it cannot when the revset does not show it.
func (m Model) jumpTo(id, what string) (Model, tea.Cmd) {
	if !m.log.Focus(id) {
		return m, m.say(what + " is " + id + ", which this revset does not show; L edits it")
	}

	return m, m.followSelected()
}

// followSelected is what a cursor move outside the log's own keys owes the panes that follow it.
func (m Model) followSelected() tea.Cmd {
	rev := m.log.Selected()

	return tea.Batch(m.followBookmark(rev), m.followDock(m.dockTarget()), m.loadStats())
}

// syncMsg answers a sync's report, gt's read, and the sync confirmation's answer.
func (m Model) syncMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case syncedMsg:
		next, cmd := m.landSync(msg.report)

		return next, cmd, true
	case gotoTrunkMsg:
		next, cmd := m.landTrunk(msg)

		return next, cmd, true
	}

	return m, nil, false
}

// syncAction runs F and gt from the log, and the palette's tracked-only sync.
func (m Model) syncAction(action string) (tea.Model, tea.Cmd, bool) {
	switch action {
	case help.ActionSync:
		return m, tea.Batch(m.say("fetching every remote…"), m.sync(false)), true
	case help.ActionSyncTracked:
		return m, tea.Batch(m.say("fetching the tracked bookmarks…"), m.sync(true)), true
	case help.ActionTrunk:
		return m, m.gotoTrunk(), true
	}

	return m, nil, false
}
