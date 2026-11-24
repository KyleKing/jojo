package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// The log's ] and [ objects: conflicted revisions (]x), bookmarked ones (]b), and the graph's own
// edges (]] to a child, [[ to the first parent). ] steps down the list for a row object, as ]u does,
// and ]] steps up the screen, since a child is drawn above its parent.

// rowMotion is a ] or [ object the log finds among the rows it already holds.
type rowMotion struct {
	test func(*jj.Revision) bool
	noun string
}

// rowMotions are the row objects by their next action.
//
//nolint:gochecknoglobals // a fixed table, read only.
var rowMotions = map[string]rowMotion{
	keys.NextAction(keys.ObjConflict): {
		noun: "conflicted revision", test: func(r *jj.Revision) bool { return r.Conflict },
	},
	help.ActionBookmarkNext: {
		noun: "bookmarked revision",
		test: func(r *jj.Revision) bool { return len(r.LocalBookmarks) > 0 || len(r.RemoteBookmarks) > 0 },
	},
}

// rowMotionOf is the row object action steps through and its direction, false for any other action.
func rowMotionOf(action string) (rowMotion, int, bool) {
	switch action {
	case keys.NextAction(keys.ObjConflict), help.ActionBookmarkNext:
		return rowMotions[action], 1, true
	case keys.PrevAction(keys.ObjConflict):
		return rowMotions[keys.NextAction(keys.ObjConflict)], -1, true
	case help.ActionBookmarkPrev:
		return rowMotions[help.ActionBookmarkNext], -1, true
	}

	return rowMotion{}, 0, false
}

// nextRow is the change id of the first revision in rows past from, going dir, that passes test.
func nextRow(rows []jj.LogRow, from string, dir int, test func(*jj.Revision) bool) (string, bool) {
	ids := revisionIDs(rows)
	byID := map[string]*jj.Revision{}

	for _, row := range rows {
		if row.Revision != nil {
			byID[row.Revision.ChangeID] = row.Revision
		}
	}

	at := slices.Index(ids, from)
	if at < 0 && dir < 0 {
		at = len(ids)
	}

	for i := at + dir; i >= 0 && i < len(ids); i += dir {
		if test(byID[ids[i]]) {
			return ids[i], true
		}
	}

	return "", false
}

// stepRows moves the log's cursor, or the picker's destination, to the next row object.
func (m Model) stepRows(motion rowMotion, dir int) (Model, tea.Cmd) {
	rows, from := m.rows, ""
	if m.pick.active {
		rows, from = m.pick.base, m.pick.dest
	} else if rev := m.log.Selected(); rev != nil {
		from = rev.ChangeID
	}

	id, ok := nextRow(rows, from, dir, motion.test)
	if !ok {
		where := map[int]string{1: "below", -1: "above"}[dir]

		return m, m.say("no " + motion.noun + " " + where + " in this revset")
	}

	return m.landMotion(id)
}

// landMotion puts the picker's destination or the log's cursor on id.
func (m Model) landMotion(id string) (Model, tea.Cmd) {
	if m.pick.active {
		if id == m.pick.dest {
			return m, nil
		}

		m.pick.dest, m.pick.inPlace = id, false
		cmd := m.changed()

		return m, cmd
	}

	return m.jumpTo(id, id)
}

// edgeMsg is the revision across a graph edge from the cursor, read for ]] and [[.
type edgeMsg struct {
	err  error
	from string
	ids  []string
	dir  int
}

// stepEdge reads the revisions across the graph edge from the cursor (or the picker's destination):
// its children for ]], its first parent for [[.
func (m Model) stepEdge(dir int) tea.Cmd {
	from := m.pick.dest
	if !m.pick.active {
		rev := m.log.Selected()
		if rev == nil {
			return nil
		}

		from = rev.ChangeID
	}

	client, ctx := m.client, m.ctx
	revset := "first_parent(" + from + ")"

	if dir > 0 {
		revset = "children(" + from + ")"
	}

	return func() tea.Msg {
		ids, err := client.ChangeIDs(ctx, revset)

		return edgeMsg{from: from, ids: ids, dir: dir, err: err}
	}
}

// landEdge moves across the edge once it is read. Of several children the nearest one drawn above
// is the one taken, which is the branch the eye follows up from the row.
func (m Model) landEdge(msg edgeMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.fail(msg.err)

		return m, nil
	}

	rows := m.rows
	if m.pick.active {
		rows = m.pick.base
	}

	shown := revisionIDs(rows)
	at := slices.Index(shown, msg.from)
	best, bestAt := "", -1

	for _, id := range msg.ids {
		i := slices.IndexFunc(shown, func(s string) bool { return sameChange(s, id) })
		if i < 0 {
			continue
		}

		// Above the row and nearest it wins; a child drawn below (a merge's other side) only when
		// nothing is above.
		above := i < at
		if best == "" || (above && (bestAt >= at || i > bestAt)) {
			best, bestAt = shown[i], i
		}
	}

	if best == "" {
		what := map[int]string{1: "a child", -1: "a parent"}[msg.dir]
		if len(msg.ids) > 0 {
			return m, m.say(msg.from + " has " + what + " this revset does not show; L edits it")
		}

		return m, m.say(msg.from + " has no " + strings.TrimPrefix(what, "a ") + " to step to")
	}

	return m.landMotion(best)
}

// motionAction runs the log's and the picker's ] and [ objects, false for any other action.
func (m Model) motionAction(action string) (Model, tea.Cmd, bool) {
	if motion, dir, ok := rowMotionOf(action); ok {
		next, cmd := m.stepRows(motion, dir)

		return next, cmd, true
	}

	switch action {
	case keys.NextAction(keys.ObjChapter):
		return m, m.stepEdge(1), true
	case keys.PrevAction(keys.ObjChapter):
		return m, m.stepEdge(-1), true
	}

	return m, nil, false
}

// edgeMsgs answers ]] and [['s read.
func (m Model) edgeMsgs(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if e, ok := msg.(edgeMsg); ok {
		next, cmd := m.landEdge(e)

		return next, cmd, true
	}

	return m, nil, false
}
