package model

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// A file's history is * on a file: in the file list, or the file under the cursor in the docked or
// full-screen diff. The log becomes the changes that touched it, under every name jj's rename
// detection follows it back through, and the dock follows the cursor showing that file alone. Esc
// puts back the revset, the row, and the dock as they were.
//
// The scope is git's `log -- <path>` widened to what the reader was looking at: the ancestors of @
// and of the revision * was pressed on, plus whatever the log showed, so a side branch on screen
// keeps its changes to the file and the file's whole past back to the root is there:
//
//	files(<names>) & (::(@ | <revision>) | (<the log's revset>))

// historyState is the log narrowed to one file's history, and what Esc returns to.
type historyState struct {
	file   jj.FileHistory
	back   historyReturn
	active bool
}

// historyReturn is the log as it stood before the first *.
type historyReturn struct {
	revset string
	cursor string
	focus  focus
	docked bool
}

// historyMsg is a file's history read in the background, and the revision the cursor lands on.
type historyMsg struct {
	err    error
	origin string
	file   jj.FileHistory
}

// handleHistoryMsg lands a history read.
func (m Model) handleHistoryMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	read, ok := msg.(historyMsg)
	if !ok {
		return m, nil, false
	}

	next, cmd := m.showHistory(read)

	return next, cmd, true
}

// historyOrigin is the file * was pressed on: the file list's row, or the file under the cursor in
// the full-screen diff or the dock, which the log's * reads too while the dock is open.
func (m Model) historyOrigin() (jj.FileOrigin, bool) {
	switch {
	case m.files.IsOpen() && m.openModal() == ctxFiles:
		revision, path, ok := m.files.Selected()

		return jj.FileOrigin{Revision: revision, Path: jj.RenamedTo(path), CwdRelative: true}, ok
	case m.diff.IsOpen() && m.openModal() == ctxDiff:
		path := m.diff.CurrentPath()

		return jj.FileOrigin{Revision: m.diffRevision, Path: path}, path != "" && m.diffRevision != ""
	case m.docked:
		path := m.dock.CurrentPath()

		return jj.FileOrigin{Revision: m.dockShown, Path: path}, path != "" && m.dockShown != ""
	}

	return jj.FileOrigin{}, false
}

// historyGate says why * has no file to show the history of.
func (m Model) historyGate() keys.Availability {
	if _, ok := m.historyOrigin(); ok {
		return keys.Allow()
	}

	if m.diff.IsOpen() && m.openModal() == ctxDiff {
		return keys.Refuse("this diff is not one revision's; * reads a file in a revision's diff")
	}

	return keys.Refuse("needs a file: open the files (l) or dock the diff (p) and put the cursor in one")
}

// historyScope is the revset a history searches: the ancestors of @ and of the revision, and the
// log the reader was on before any history.
func (m Model) historyScope(revision string) string {
	revset := m.revset
	if m.history.active {
		revset = m.history.back.revset
	}

	scope := "::(@ | " + revision + ")"
	if strings.TrimSpace(revset) != "" {
		scope += " | (" + revset + ")"
	}

	return scope
}

// startHistory reads the history of the file under the cursor.
func (m Model) startHistory() (tea.Model, tea.Cmd) {
	origin, ok := m.historyOrigin()
	if !ok {
		return m, m.say(m.historyGate().Why)
	}

	client, ctx, scope := m.client, m.ctx, m.historyScope(origin.Revision)
	read := func() tea.Msg {
		file, err := client.FollowFile(ctx, origin, scope)

		return historyMsg{file: file, origin: origin.Revision, err: err}
	}

	return m, tea.Batch(m.say("reading the history of "+origin.Path+"…"), read)
}

// showHistory turns the log into the file's history, keeping what Esc returns to from the first *
// only, so a * inside a history still returns to where the reader started.
func (m Model) showHistory(msg historyMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return m, nil
		}

		m.fail(msg.err)

		return m, nil
	}

	if !m.history.active {
		m.history.back = historyReturn{revset: m.revset, focus: m.focus, docked: m.docked}
		if rev := m.log.Selected(); rev != nil {
			m.history.back.cursor = rev.ChangeID
		}
	}

	m.history.active, m.history.file, m.message = true, msg.file, ""
	m.files.Close()
	m.diff.Close()
	m.focus, m.docked, m.landOn = focusLog, true, msg.origin
	m.resize()

	return m, m.loadLog(msg.file.Revset)
}

// leaveHistory puts the log back as it was before the first *.
func (m Model) leaveHistory() (Model, tea.Cmd) {
	back := m.history.back
	m.history = historyState{}
	m.landOn, m.focus = back.cursor, back.focus

	if !back.docked {
		m.docked = false
		m.dock.Close()

		if m.focus == focusDock {
			m.focus = focusLog
		}
	}

	m.resize()

	return m, m.loadLog(back.revset)
}

// historyPaths is what the dock narrows a diff to while the log is a file's history: the one fileset
// naming the file under every name it had, none otherwise.
func (m Model) historyPaths() []string {
	if !m.history.active {
		return nil
	}

	return []string{jj.Fileset(m.history.file.Names...)}
}

// historyBanner is the row over the log while it is a file's history: whose, under which older names,
// how many changes, and the way back, which is kept whole at any width.
func (m Model) historyBanner() string {
	names := m.history.file.Names
	text := "★ history of " + names[0]

	if len(names) > 1 {
		text += " (was " + strings.Join(names[1:], ", ") + ")"
	}

	count := len(revisionsOf(m.rows))
	text += " · " + strconv.Itoa(count) + map[bool]string{true: " change", false: " changes"}[count == 1]

	way := " · " + m.keys.Label(keys.ActionBack) + " back to the log"
	text = table.Truncate(text, max(0, m.width-ansi.StringWidth(way))) + way

	return padLine(m.skin.Accent.Render(table.Truncate(text, m.width)), m.width)
}

// historyAction runs * and, in a history, the Esc that leaves it. Esc clears checks first, which is
// the nearer level to back out of.
func (m Model) historyAction(action string) (tea.Model, tea.Cmd, bool) {
	switch {
	case action == help.ActionHistory:
		next, cmd := m.startHistory()

		return next, cmd, true
	case action == keys.ActionBack && m.history.active && len(m.log.Checked()) == 0:
		next, cmd := m.leaveHistory()

		return next, tea.Batch(cmd, m.say("back to the log")), true
	}

	return m, nil, false
}

// footerRevset is the revset the footer names: in a file's history the file, since the revset that
// reads it is a fileset no one typed and the banner above spells out.
func (m Model) footerRevset() string {
	if m.history.active {
		return "★ " + m.history.file.Names[0]
	}

	return m.revset
}
