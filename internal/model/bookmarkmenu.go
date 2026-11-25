package model

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// The b menu acts on the revision under the cursor, one chord per verb, where the bookmark panel (b b)
// acts on a row of the list. A verb that needs a name takes it from the revision when it carries one
// bookmark and asks which when it carries several, so the common case is two keys.

// bookmarksForMsg is the bookmark list a b-menu verb read to find its candidates.
type bookmarksForMsg struct {
	err    error
	marks  []jj.Bookmark
	action string
	rev    jj.Revision
}

// conflictSuffix is how a log row draws a bookmark that moved both ways.
const conflictSuffix = "??"

// bookmarkMenu runs a b-menu verb on rev, false for an action that is not one.
func (m Model) bookmarkMenu(action string, rev jj.Revision) (Model, tea.Cmd, bool) {
	switch action {
	case help.ActionBookmarkAdvance:
		m.noteWrite(action)

		return m, m.readAdvances(rev), true
	case help.ActionBookmarkResolve:
		marks := make([]jj.Bookmark, 0, len(rev.Bookmarks))
		for _, name := range conflictedOn(rev) {
			marks = append(marks, jj.Bookmark{Name: name, Conflict: true})
		}

		next, cmd := m.chooseBookmark(action, rev, marks)

		return next, cmd, true
	case help.ActionBookmarkSet, help.ActionBookmarkRename, help.ActionBookmarkDelete,
		help.ActionBookmarkForget, help.ActionBookmarkTrack:
		client, ctx := m.client, m.ctx

		return m, func() tea.Msg {
			marks, err := client.Bookmarks(ctx)

			return bookmarksForMsg{action: action, rev: rev, marks: marks, err: err}
		}, true
	}

	return m, nil, false
}

// advancesMsg is what a tug to rev would move, read before anything moves.
type advancesMsg struct {
	err      error
	advances []jj.Advance
	rev      jj.Revision
}

// readAdvances asks jj which bookmarks a tug to rev would move: those on its nearest bookmarked
// ancestors, never the row the log happens to draw under it.
func (m Model) readAdvances(rev jj.Revision) tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		advances, err := client.Advances(ctx, rev.ChangeID)

		return advancesMsg{rev: rev, advances: advances, err: err}
	}
}

// landAdvances asks before a tug moves anything, naming each bookmark, where it is, and where it
// goes. A trunk bookmark is named as the trunk's, because tugging it onto a change publishes that
// change as trunk on the next push.
func (m Model) landAdvances(msg advancesMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.fail(msg.err)

		return m, nil
	}

	to := shortID(&msg.rev)
	if len(msg.advances) == 0 {
		return m, m.say("no bookmark on an ancestor of " + to + " to advance; b c creates one here")
	}

	names := make([]string, 0, len(msg.advances))
	moves := make([]string, 0, len(msg.advances))
	trunk := ""

	for _, a := range msg.advances {
		names = append(names, a.Name)

		move := a.Name + " from " + a.From
		if a.Trunk {
			move = a.Name + " (trunk) from " + a.From
			trunk = a.Name
		}

		moves = append(moves, move)
	}

	question := "advance " + joinNames(moves) + " to " + to + "?"
	verb := "advance"

	if trunk != "" {
		question += " " + to + " is not on trunk yet"
		verb = "advance " + trunk
	}

	rev := msg.rev.ChangeID

	m.openAsk(ask{
		question: question,
		verb:     verb,
		declined: "kept " + joinNames(names) + " where " + pluralIt(len(names)),
		undoable: true,
		run: func(m Model) (Model, tea.Cmd) {
			cmd := m.act(rev, func(ctx context.Context, c *jj.Client) (string, error) {
				return c.BookmarkAdvance(ctx, names, rev)
			})

			return m, cmd
		},
	})

	return m, nil
}

// pluralIt is "it is" or "they are", for n things.
func pluralIt(n int) string {
	if n == 1 {
		return "it is"
	}

	return "they are"
}

// conflictedOn is the names of rev's bookmarks that moved both ways.
func conflictedOn(rev jj.Revision) []string {
	var out []string

	for _, b := range rev.Bookmarks {
		if name, ok := strings.CutSuffix(b, conflictSuffix); ok && !strings.Contains(name, "@") {
			out = append(out, name)
		}
	}

	return out
}

// sameChange reports whether two change id prefixes name one change, since the log and the bookmark
// list each shorten an id as far as their own reads allow.
func sameChange(a, b string) bool {
	return a != "" && b != "" && (strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
}

// candidates is the bookmarks a verb may act on from rev: for set, every local bookmark not already
// on it; for track, its remote bookmarks; for the rest, its local ones.
func candidates(action string, rev jj.Revision, marks []jj.Bookmark) []jj.Bookmark {
	var out []jj.Bookmark

	for _, b := range marks {
		switch action {
		case help.ActionBookmarkSet:
			if b.Remote == "" && !sameChange(b.ChangeID, rev.ChangeID) {
				out = append(out, b)
			}
		case help.ActionBookmarkTrack:
			if b.Remote != "" && slices.Contains(rev.RemoteBookmarks, b.Name) {
				out = append(out, b)
			}
		default:
			if b.Remote == "" && slices.Contains(rev.LocalBookmarks, b.Name) {
				out = append(out, b)
			}
		}
	}

	return out
}

// noCandidate is what a verb says when the read found nothing for it to act on.
func noCandidate(action string, rev jj.Revision) string {
	switch action {
	case help.ActionBookmarkSet:
		return "no other bookmark to move to " + shortID(&rev) + "; b c creates one here"
	case help.ActionBookmarkTrack:
		return shortID(&rev) + " has no remote bookmark; f fetches"
	}

	return shortID(&rev) + " has no bookmark; b c creates one"
}

// landBookmarks takes the list a verb read and either acts, asks which bookmark, or says why not.
func (m Model) landBookmarks(msg bookmarksForMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.fail(msg.err)

		return m, nil
	}

	return m.chooseBookmark(msg.action, msg.rev, candidates(msg.action, msg.rev, msg.marks))
}

// chooseBookmark acts on the one candidate there is, or opens the panel asking which.
func (m Model) chooseBookmark(action string, rev jj.Revision, marks []jj.Bookmark) (Model, tea.Cmd) {
	switch len(marks) {
	case 0:
		return m, m.say(noCandidate(action, rev))
	case 1:
		return m.bookmarkChosen(action, marks[0])
	}

	b, _ := m.keys.Binding(action)
	m.bookmarks.Choose(action, word(b)+": which bookmark on "+shortID(&rev)+"?", marks)

	return m, nil
}

// bookmarkChosen runs action on mark, for the revision the menu was opened on.
func (m Model) bookmarkChosen(action string, mark jj.Bookmark) (Model, tea.Cmd) {
	name := mark.Name
	m.noteWrite(action)

	switch action {
	case help.ActionBookmarkSet, help.ActionBookmarkResolve:
		cmd := m.openBookmarkPicker(name, mark.ChangeID)

		return m, cmd
	case help.ActionBookmarkRename:
		return m.askBookmarkNameFrom("rename "+name+" to", name, func(m Model, to string) (Model, tea.Cmd) {
			if strings.TrimSpace(to) == "" || to == name {
				return m, m.say("rename of " + name + " canceled, nothing ran")
			}

			cmd := m.act(name, func(ctx context.Context, c *jj.Client) (string, error) {
				return c.BookmarkRename(ctx, name, to)
			})

			return m, cmd
		}), nil
	case help.ActionBookmarkDelete:
		cmd := m.act(name, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.BookmarkDelete(ctx, name)
		})

		return m, cmd
	case help.ActionBookmarkForget:
		cmd := m.act(name, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.BookmarkForget(ctx, name)
		})

		return m, cmd
	case help.ActionBookmarkTrack:
		ref := mark.Ref()
		cmd := m.act(ref, func(ctx context.Context, c *jj.Client) (string, error) {
			if mark.Tracked {
				return c.BookmarkUntrack(ctx, ref)
			}

			return c.BookmarkTrack(ctx, ref)
		})

		return m, cmd
	}

	return m, nil
}

// openBookmarkPicker puts the log into the picker for moving name, starting on the cursor: the
// preview draws where it lands, and -B is there for a move jj calls backwards or sideways.
func (m *Model) openBookmarkPicker(name, from string) tea.Cmd {
	var sources []string
	if from != "" {
		sources = []string{from}
	}

	cmd := m.openPicker(pickBookmark, sources)
	m.pick.bookmark = name
	m.showPicked()

	return cmd
}

// bookmarkMenuMsg answers what the b menu's reads and its chooser send back.
func (m Model) bookmarkMenuMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case bookmarksForMsg:
		next, cmd := m.landBookmarks(msg)

		return next, cmd, true
	case advancesMsg:
		next, cmd := m.landAdvances(msg)

		return next, cmd, true
	case bookmarkview.ChosenMsg:
		next, cmd := m.bookmarkChosen(msg.Verb, msg.Bookmark)

		return next, cmd, true
	}

	return m, nil, false
}

// bookmarkGate is the b menu's part of the log's gate: what each verb needs the revision to carry.
func bookmarkGate(action string, rev *jj.Revision) (keys.Availability, bool) {
	switch action {
	case help.ActionBookmarkRename, help.ActionBookmarkDelete, help.ActionBookmarkForget:
		if len(rev.LocalBookmarks) == 0 {
			return keys.Refuse(shortID(rev) + " has no bookmark; b c creates one"), true
		}
	case help.ActionBookmarkTrack:
		if len(rev.RemoteBookmarks) == 0 {
			return keys.Refuse(shortID(rev) + " has no remote bookmark; f fetches"), true
		}
	case help.ActionBookmarkResolve:
		if len(conflictedOn(*rev)) == 0 {
			return keys.Refuse(shortID(rev) + " has no conflicted bookmark"), true
		}
	case help.ActionBookmarkAdvance:
		if len(rev.LocalBookmarks) > 0 {
			return keys.Refuse(shortID(rev) + " holds " + rev.LocalBookmarks[0] +
				" already; b a tugs the nearest bookmark on an ancestor up to the cursor"), true
		}
	default:
		return keys.Availability{}, false
	}

	return keys.Allow(), true
}

// pickGate is the picker's part of the gate: a bookmark move jj would refuse says so on enter rather
// than failing after it, and names the flag that allows it.
//
// A rebase of -r revisions starts on their parent, which is no destination at all: run there, jj
// lifts the revisions out and hangs their children on the parent, reshaping the stack the reader
// only meant to look at. Enter there says to pick a destination first.
func (m *Model) pickGate(action string) keys.Availability {
	p := &m.pick
	if action == help.ActionPickApply && p.verb == pickRebase && p.scope() == jj.RebaseScopeRevisions &&
		p.dest == p.initial && !p.unchanged {
		return keys.Refuse("the destination is still the parent, where -r would pull " + joinIDs(p.sources) +
			" out of its stack; move to where it goes, then enter")
	}

	if action != help.ActionPickApply || p.verb != pickBookmark || p.shownFor != p.command() {
		return keys.Allow()
	}

	switch {
	case jj.IsBackwards(p.previewErr):
		return keys.Refuse(p.bookmark + " would move backwards or sideways; - B allows it")
	case p.unchanged:
		return keys.Refuse(p.bookmark + " is there already; move the cursor to where it goes")
	}

	return keys.Allow()
}
