package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/statusbar"
	"github.com/kyleking/jojo/internal/jj"
)

type headOpMsg struct {
	description string
}

// loadHeadOp names the operation u would reverse, refreshed after every write.
func (m Model) loadHeadOp() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		ops, err := client.OpLog(ctx, 1)
		if err != nil || len(ops) == 0 {
			return headOpMsg{}
		}

		return headOpMsg{description: ops[0].Description}
	}
}

// helpHint is the route to the legend, which every footer ends on.
func (m Model) helpHint() keyhint.Hint {
	h, ok := m.keys.Hint(keys.ActionHelp)
	if !ok {
		return keyhint.Hint{Key: "?", What: "help"}
	}

	return h
}

// pinnedHelp is helpHint for a footer that sheds hints by priority, where help must stay.
func (m Model) pinnedHelp() keyhint.Hint {
	h := m.helpHint()
	h.Fresh, h.Pin = 0, true

	return h
}

// failureHint offers the whole of a failure the footer could only show one line of.
func (m Model) failureHint() keyhint.Hint {
	h, ok := m.keys.Hint(keys.ActionMessages)
	if !ok {
		return keyhint.Hint{}
	}

	h.What = "full error"

	return h
}

// logFooter is what the log's footer offers, most used first: keyhint.Fit sheds from the end.
//
//nolint:gochecknoglobals // a fixed table, read only.
var logFooter = []string{
	help.ActionDiff, help.ActionPreview, help.ActionNew, keys.ActionDescribe, help.ActionSquash,
	help.ActionRebase, help.ActionEdit, help.ActionAbandon, keys.ActionUndo, help.ActionFiles,
	help.ActionSplit, help.ActionDuplicate, help.ActionRestore, help.ActionCommit, help.ActionPush,
	help.ActionFetch, help.ActionBookmarks, help.ActionOpLog, keys.ActionPalette,
}

// viewFooter is the docked diff's, while it has the keys.
//
//nolint:gochecknoglobals // a fixed table, read only.
var viewFooter = []string{
	keys.NextAction(keys.ObjHunk), keys.NextAction(keys.ObjFile), keys.ActionAgain,
	keys.NextAction(keys.ObjRound), keys.ActionRounds, keys.ActionViewSideBySide, keys.ActionBack,
}

// footerHints ends on help, because the footer drops hints from the right and keeps the last.
func (m Model) footerHints() []keyhint.Hint {
	var hints []keyhint.Hint

	switch m.screen() {
	case help.Outline:
		hints = m.outlineHints()
	case keys.View:
		hints = m.keys.Hints(viewFooter...)
	default:
		hints = append(m.rowHints(m.log.Selected()), m.keys.Hints(logFooter...)...)
	}

	hints = append(hints, m.helpHint())

	// A failure takes the last place, the one hint the footer never drops, while it is on screen.
	if m.failing {
		hints = append(hints, m.failureHint())
	}

	return hints
}

// rowHints is the one thing worth saying about the revision under the cursor, keyed by the verb
// that answers it.
func (m Model) rowHints(rev *jj.Revision) []keyhint.Hint {
	var action, what string

	switch {
	case rev == nil:
		return nil
	case rev.Conflict:
		action, what = help.ActionResolve, "conflicted, resolve it"
	case rev.Immutable:
		action, what = help.ActionEdit, "immutable, an edit asks first"
	case rev.Empty && !rev.WorkingCopy:
		action, what = help.ActionAbandon, "empty, abandon it"
	case rev.Divergent:
		action, what = help.ActionConverge, "divergent, converge it"
	case len(rev.LocalBookmarks) > 0:
		action, what = help.ActionPush, "bookmarked, push it"
	default:
		return nil
	}

	h, ok := m.keys.Hint(action)
	if !ok {
		return nil
	}

	h.What = what

	return []keyhint.Hint{h}
}

// overlayFooter is the message alone under an open overlay, whose own keys are in its own frame.
func (m Model) overlayFooter() string {
	var hints []keyhint.Hint
	if m.failing {
		hints = []keyhint.Hint{m.failureHint()}
	}

	return m.status.View(m.width, statusbar.Context{Message: m.message, Hints: hints})
}

// fail puts err in the footer and keeps all of it for !.
func (m *Model) fail(err error) {
	m.message, m.lastOutput, m.failing = err.Error(), jj.FullOutput(err), true
}

// showOutput opens the last failure in full.
func (m *Model) showOutput() tea.Cmd {
	if m.lastOutput == "" {
		return m.say("no failed command to show")
	}

	return m.output.Open("last failure", m.lastOutput)
}

func (m Model) footer() string {
	return m.status.View(m.width, statusbar.Context{
		Bookmarks: m.nearbyBookmarks(),
		Revset:    m.revset,
		Message:   m.message,
		Checked:   len(m.log.Checked()),
		Undo:      m.lastOp,
		Hints:     m.footerHints(),
	})
}

// nearbyBookmarks reports the bookmarks in the log rather than a single current branch, because jj
// has no current branch to report.
func (m Model) nearbyBookmarks() []statusbar.Bookmark {
	var out []statusbar.Bookmark

	for _, row := range m.rows {
		if row.Revision == nil {
			continue
		}

		for _, name := range row.Revision.Bookmarks {
			out = append(out, statusbar.Bookmark{Name: name, AtWorkingCopy: row.Revision.WorkingCopy})
		}
	}

	return out
}

// inspectorMinHeight is the shortest terminal that spends a row on the inspector, since below it the
// row is worth more to the log.
const inspectorMinHeight = 12

// inspectorRows is the rows the inspector takes: one, unless the terminal is too short to spare it.
func (m Model) inspectorRows() int {
	if m.height < inspectorMinHeight {
		return 0
	}

	return 1
}

// overflow is what the row the cursor is on had no room for, from whichever list is drawn.
// While the outline is open it ends on the revset the stack was read with, which its title leaves
// out for length.
func (m Model) overflow() []string {
	if m.outline.active {
		return append(slices.Clone(m.outline.view.Overflow()), "stack "+m.outline.revset)
	}

	return m.log.Overflow()
}

// inspectorGap separates the inspector's facts.
const inspectorGap = " · "

// inspector is the line over the footer that says what the row under the cursor had no room for at
// this width, then its commit id, so narrowing the terminal hides nothing a reader cannot get back by
// moving onto the row. It is one row or none, never taller, so it costs the log one line.
func (m Model) inspector(rev *jj.Revision, overflow []string) []string {
	if m.inspectorRows() == 0 {
		return nil
	}

	if rev == nil {
		return []string{strings.Repeat(" ", m.width)}
	}

	// The commit sits at the right edge and the facts are cut to the room left of it, so a long
	// description can never push the commit off the line.
	commit := ""
	if rev.CommitID != "" {
		commit = "commit " + rev.CommitID
	}

	room := m.width - 1 - lipgloss.Width(commit) - len(inspectorGap)
	facts := make([]string, 0, len(overflow))

	for _, fact := range overflow {
		facts = append(facts, oneLineText(fact))
	}

	left := table.Truncate(strings.Join(facts, inspectorGap), max(0, room))
	pad := max(1, m.width-1-lipgloss.Width(left)-lipgloss.Width(commit))
	line := " " + m.skin.Body.Render(left) + strings.Repeat(" ", pad) + m.skin.Muted.Render(commit)

	return []string{padLine(line, m.width)}
}

// oneLineText keeps a fact on the inspector's single row.
func oneLineText(text string) string { return strings.Join(strings.Fields(text), " ") }

// outlineHints are the footer's while the outline is open: its own keys, most used first.
func (m Model) outlineHints() []keyhint.Hint {
	return m.keys.Hints(keys.NextAction(keys.ObjChapter), keys.PrevAction(keys.ObjChapter), help.ActionChapterRead,
		keys.NextAction(keys.ObjHunk), keys.NextAction(keys.ObjRound), keys.ActionRounds, keys.ActionBack)
}
