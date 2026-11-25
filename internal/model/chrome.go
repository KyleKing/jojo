package model

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/icons"
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

// loadHeadOp names the operation u would reverse, refreshed after every write: "" when u would
// refuse it (a snapshot of edits on disk, a push), so the footer never offers what u will not do.
// After an undo it is the operation the next undo steps back to, not the undo itself.
func (m Model) loadHeadOp() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		op, ok, err := client.HeadUndo(ctx)
		if err != nil || !ok {
			return headOpMsg{}
		}

		return headOpMsg{description: op.Description}
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

// failureHint offers the whole of a failure the footer could only show one line of: gm opens the
// trail on it.
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
	help.ActionFetch, help.ActionBookmarks, help.ActionOpLog, keys.ActionReload, keys.ActionPalette,
}

// viewFooter is the docked diff's, while it has the keys.
//
//nolint:gochecknoglobals // a fixed table, read only.
var viewFooter = []string{
	keys.NextAction(keys.ObjHunk), keys.ActionRead, keys.NextAction(keys.ObjUnread), keys.NextAction(keys.ObjFile),
	keys.ActionOnlyUnread, keys.ActionAgain, keys.NextAction(keys.ObjRound), keys.ActionRounds,
	keys.ActionViewSideBySide, keys.ActionBack,
}

// footerHints ends on help, because the footer drops hints from the right and keeps the last. A key
// the gate refuses here is drawn flat rather than dropped, and the key the coach's next step names is
// brightened, so the footer itself points at it.
func (m *Model) footerHints() []keyhint.Hint {
	var hints []keyhint.Hint

	switch m.screen() {
	case help.Outline:
		hints = m.outlineHints()
	case keys.View:
		hints = tellOnlyUnread(m.keys, m.keys.HintsAt(m.gate, viewFooter...))
	default:
		hints = m.keys.HintsAt(m.gate, logFooter...)
		if next, ok := m.nextStep(); ok {
			hints = coach.Mark(hints, next)
		}
	}

	hints = append(hints, m.helpHint())

	// A failure takes the last place, the one hint the footer never drops, while it is on screen.
	if m.failing {
		hints = append(hints, m.failureHint())
	}

	return hints
}

// overlayFooter is the message alone under an open overlay, whose own keys are in its own frame.
func (m Model) overlayFooter() string {
	var hints []keyhint.Hint
	if m.failing {
		hints = []keyhint.Hint{m.failureHint()}
	}

	return m.status.View(m.width, statusbar.Context{Message: m.message, Hints: hints})
}

// promptFooter is the footer under the docked prompt: its keys, after any message.
func (m Model) promptFooter() string {
	if m.message == "" {
		return m.prompt.Footer(m.width)
	}

	return m.status.View(m.width, statusbar.Context{Message: m.message, Hints: m.prompt.Hints()})
}

// withPrompt docks an open prompt over the bottom of a frame that did not make room for it (a
// picker, an overlay), replacing those rows with it and the footer that names its keys. The log's
// own frame docks it itself.
func (m Model) withPrompt(frame string) string {
	if !m.prompt.IsOpen() {
		return frame
	}

	lines := strings.Split(frame, "\n")
	keep := max(0, len(lines)-m.prompt.Rows(m.width, m.height)-footerRows)

	return strings.Join(append(lines[:keep], m.prompt.Docked(m.width, m.height), m.promptFooter()), "\n")
}

// fail puts err in the footer; the trail keeps all of what jj said for gm.
func (m *Model) fail(err error) {
	m.message, m.failing = err.Error(), true
}

// showOutput opens the trail, on the newest failure while one is on screen.
func (m *Model) showOutput() { m.trailView.Open(m.trail, m.failing) }

func (m Model) footer() string {
	if m.asking != nil {
		return m.askFooter()
	}

	return m.status.View(m.width, statusbar.Context{
		Bookmarks: m.nearbyBookmarks(),
		Revset:    m.footerRevset(),
		Message:   m.message,
		Checked:   len(m.log.Checked()),
		Undo:      m.lastOp,
		Hints:     m.footerHints(),
		Tip:       m.tipLine(),
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

	// What the row cut comes first, since nothing else on screen carries it; with nothing cut the
	// line says the coach's next step, unless the band says it instead.
	left := m.skin.Body.Render(table.Truncate(strings.Join(facts, inspectorGap), max(0, room)))
	if next, ok := m.nextStep(); ok && len(facts) == 0 && !m.band {
		left = coach.Line(m.skin.Skin, next, max(0, room))
	}

	pad := max(1, m.width-1-lipgloss.Width(left)-lipgloss.Width(commit))
	line := " " + left + strings.Repeat(" ", pad) + m.skin.Muted.Render(commit)

	return []string{padLine(line, m.width)}
}

// oneLineText keeps a fact on the inspector's single row.
func oneLineText(text string) string { return strings.Join(strings.Fields(text), " ") }

// outlineHints are the footer's while the outline is open: its own keys, most used first.
func (m *Model) outlineHints() []keyhint.Hint {
	return m.keys.HintsAt(m.gate, keys.NextAction(keys.ObjChapter), keys.PrevAction(keys.ObjChapter),
		help.ActionChapterRead, keys.ActionRead, keys.NextAction(keys.ObjUnread), help.ActionSubmit,
		keys.NextAction(keys.ObjHunk), keys.NextAction(keys.ObjRound), keys.ActionRounds, keys.ActionBack)
}

// iconsLabel heads the legend's last row, as config.toml's key spells it.
const iconsLabel = "icons"

// iconsLine is the row the legend keeps under itself: which icon set the file list draws, why, and
// how to change it, in the longest wording of the change that fits the width. Where none fits, the
// set and its reason keep their place and the change is cut, so the row never wraps.
func (m Model) iconsLine() string {
	if m.icons == nil {
		return ""
	}

	says := m.icons.String()
	lead := iconsLabel + "  " + says + " · "
	ways := iconsHow(*m.icons)

	how := ways[len(ways)-1]
	for _, way := range ways {
		if lipgloss.Width(lead+way) <= m.width {
			how = way

			break
		}
	}

	room := m.width - lipgloss.Width(iconsLabel+"  ")
	says = table.Truncate(says, room)
	how = table.Truncate(how, room-lipgloss.Width(says+" · "))

	line := m.skin.Key.Render(iconsLabel) + "  " + m.skin.Body.Render(says)
	if how != "" {
		line += m.skin.Muted.Render(" · " + how)
	}

	return line
}

// iconsHow are the ways to change the icon set a resolution names, longest first: it suggests the
// other set worth trying (plain where Nerd icons draw, nerd elsewhere), and where ARAGONITE_ICONS
// decided, that config.toml cannot win until the variable is changed or unset.
func iconsHow(r icons.Resolution) []string {
	want := "nerd"
	if r.Mode == icons.Nerd {
		want = "plain"
	}

	env := icons.EnvOverride + "=" + want
	if r.Source == icons.FromEnv {
		return []string{
			"set " + env + ", or unset it for config.toml's icons",
			"set " + env + " or unset it",
			env,
		}
	}

	set := iconsLabel + ` = "` + want + `"`

	return []string{
		"set " + set + " in ~/.config/jojo/config.toml or " + env,
		"set " + set + " or " + env,
		set + " or " + env,
		env,
	}
}
