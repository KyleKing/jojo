package model

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
)

// pushPreviewMsg carries what a dry run says a push would do, which is what the confirmation shows.
type pushPreviewMsg struct {
	err    error
	what   string
	notice string
	// why is a line the dialog adds above jj's dry run when the push is not the one asked for: a
	// new bookmark named so jj creates and tracks it, or a revision pushed under a generated name.
	why  string
	opts jj.PushOptions
}

// handleBookmarkMsg runs what the panel asked for. Only the push is gated, since jj can undo the
// rest.
func (m Model) handleBookmarkMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case bookmarkview.SetMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkSet(ctx, msg.Name, msg.Target)
		}), true
	case bookmarkview.AdvanceMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkAdvance(ctx, []string{msg.Name}, msg.Target)
		}), true
	case bookmarkview.CreateRequestedMsg:
		target := msg.Target

		return m.askBookmarkName("new bookmark on "+target, func(m Model, name string) (Model, tea.Cmd) {
			return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
				return m.client.BookmarkSet(ctx, name, target)
			})
		}), nil, true
	case bookmarkview.RenameRequestedMsg:
		from := msg.Name

		return m.askBookmarkName("rename "+from+" to", func(m Model, name string) (Model, tea.Cmd) {
			return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
				return m.client.BookmarkRename(ctx, from, name)
			})
		}), nil, true
	case bookmarkview.DeleteMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkDelete(ctx, msg.Name)
		}), true
	case bookmarkview.ForgetMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkForget(ctx, msg.Name)
		}), true
	case bookmarkview.TrackMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkTrack(ctx, msg.Ref)
		}), true
	case bookmarkview.UntrackMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkUntrack(ctx, msg.Ref)
		}), true
	case bookmarkview.PushMsg:
		return m, m.previewPush(msg.Name, jj.PushOptions{Bookmarks: []string{msg.Name}}), true
	case bookmarkview.ResolveMsg:
		return m, m.bookmarkWrite(func(ctx context.Context) (string, error) {
			return m.client.BookmarkSet(ctx, msg.Name, msg.Ref)
		}), true
	case pushPreviewMsg:
		return m.offerPush(msg)
	}

	return m, nil, false
}

// askBookmarkName collects a name for a bookmark verb the panel cannot supply one for.
func (m Model) askBookmarkName(title string, then submitted) Model {
	return m.askBookmarkNameFrom(title, "", then)
}

// askBookmarkNameFrom is askBookmarkName with the field seeded, which a rename seeds with the old name.
func (m Model) askBookmarkNameFrom(title, initial string, then submitted) Model {
	m.bookmarks.Close()
	m.onSubmit = then
	m.prompt.Open(title, initial)

	return m
}

// dryRunNotice is jj saying it stopped short, which would only confuse what the dialog agrees to.
const dryRunNotice = "Dry-run requested, not pushing."

// nothingToPushNotice is what jj reports when the revision carries no bookmark, which is a fact to
// state rather than an action to confirm.
const nothingToPushNotice = "Nothing changed."

// noticeLines turns jj's dry-run notice into the detail block.
func noticeLines(notice string) []string {
	var out []string

	for _, line := range strings.Split(notice, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == dryRunNotice {
			continue
		}

		out = append(out, trimmed)
	}

	return out
}

func (m Model) bookmarkWrite(write func(context.Context) (string, error)) tea.Cmd {
	return m.run(undoHint, write)
}

// previewPush asks jj what the push would do, so the dialog shows jj's answer rather than ours.
func (m Model) previewPush(what string, opts jj.PushOptions) tea.Cmd {
	return m.previewPushBecause(what, "", opts)
}

func (m Model) previewPushBecause(what, why string, opts jj.PushOptions) tea.Cmd {
	client, ctx := m.client, m.ctx
	dry := opts
	dry.DryRun = true

	return func() tea.Msg {
		notice, err := client.Push(ctx, dry)

		return pushPreviewMsg{what: what, why: why, opts: opts, notice: notice, err: err}
	}
}

// pushInstead is the push that does what a push by revision refused: a bookmark new to the remote is
// pushed by name, which jj creates and tracks, and a revision with no bookmark goes up under one jj
// names for it. The second preview carries why, so the dialog says which of the two it is.
func (m Model) pushInstead(msg pushPreviewMsg) tea.Cmd {
	if msg.why != "" || len(msg.opts.Revisions) == 0 {
		return nil
	}

	if names, remote := jj.NewRemoteBookmarks(msg.notice); len(names) > 0 {
		refs := make([]string, len(names))
		for i, name := range names {
			refs[i] = name + "@" + remote
		}

		why := strings.Join(names, ", ") + " is new on " + remote + ": pushing it by name creates " +
			strings.Join(refs, ", ") + " and tracks it"

		return m.previewPushBecause(strings.Join(names, ", "), why,
			jj.PushOptions{Remote: msg.opts.Remote, Bookmarks: names})
	}

	if m.carryBookmarks(msg.opts.Revisions) {
		return nil
	}

	why := msg.what + " has no bookmark: jj git push -c creates push-<change id> for it"

	return m.previewPushBecause(msg.what, why, jj.PushOptions{Remote: msg.opts.Remote, Changes: msg.opts.Revisions})
}

// carryBookmarks reports whether any of revs has a local bookmark, which is what a push by revision
// sends; one with none has nothing to send until it is given one.
func (m Model) carryBookmarks(revs []string) bool {
	for _, row := range m.rows {
		if row.Revision != nil && slices.Contains(revs, row.Revision.ChangeID) && len(row.Revision.LocalBookmarks) > 0 {
			return true
		}
	}

	return false
}

func (m Model) offerPush(msg pushPreviewMsg) (tea.Model, tea.Cmd, bool) {
	if msg.err != nil {
		if errors.Is(msg.err, jj.ErrStaleWorkingCopy) {
			cmd := m.openStale(msg.err)

			return m, cmd, true
		}

		m.message, m.failing = m.landOnRefusal(msg.err), true

		return m, nil, true
	}

	detail := noticeLines(msg.notice)
	if len(detail) == 0 || strings.Contains(msg.notice, nothingToPushNotice) {
		if instead := m.pushInstead(msg); instead != nil {
			return m, instead, true
		}

		m.message = "nothing to push for " + msg.what

		return m, nil, true
	}

	if msg.why != "" {
		detail = append([]string{msg.why, ""}, detail...)
	}

	m.pushing = pendingPush{what: msg.what, opts: msg.opts}

	cmd := m.confirm.Open(confirm.Request{
		Key:    confirmPush,
		Title:  "push " + msg.what + "?",
		Detail: detail,
		Danger: "a push leaves this machine, so u cannot take it back",
	})

	return m, cmd, true
}

// pushRefusedMsg is a push that failed, which the update loop explains and, when jj named a commit
// the log shows, puts the cursor on.
type pushRefusedMsg struct {
	err error
}

// refusedCommit matches the commit id jj names in a push refusal ("Won't push commit 0717385…").
var refusedCommit = regexp.MustCompile(`commit ([0-9a-f]{6,})`)

// landOnRefusal is pushProblem with the cursor moved to the commit jj refused, named by its change id,
// so the d or x the line offers acts on that revision rather than on wherever the cursor was.
func (m *Model) landOnRefusal(err error) string {
	text := pushProblem(err)
	if !errors.Is(err, jj.ErrPushRefused) {
		return text
	}

	found := refusedCommit.FindStringSubmatch(err.Error())
	if found == nil {
		return text
	}

	for _, row := range m.rows {
		rev := row.Revision
		if rev == nil || rev.CommitID == "" {
			continue
		}

		if strings.HasPrefix(found[1], rev.CommitID) || strings.HasPrefix(rev.CommitID, found[1]) {
			m.log.Focus(rev.ChangeID)

			return strings.Replace(text, "commit "+found[1], rev.ChangeID, 1) + " (the cursor is on it)"
		}
	}

	return text
}

// pushProblem turns jj's refusal into the line that gets the user out of it.
func pushProblem(err error) string {
	text := strings.TrimPrefix(err.Error(), jj.ErrPushRefused.Error()+": ")

	if !errors.Is(err, jj.ErrPushRefused) {
		return text
	}

	// jj's hint names the commit the row already shows, so the advice takes its place.
	headline, _, _ := strings.Cut(text, " (")

	switch {
	case strings.Contains(headline, "no description"):
		return headline + ": d describes it"
	case strings.Contains(headline, "empty"):
		return headline + ". An empty revision has nothing to push"
	case strings.Contains(headline, "conflict"):
		return headline + ": x resolves it"
	}

	return text
}

func (m Model) push(p pendingPush) tea.Cmd {
	client, ctx, revset := m.client, m.ctx, m.revset

	return tea.Sequence(
		func() tea.Msg {
			if _, err := client.Push(ctx, p.opts); err != nil {
				return pushRefusedMsg{err: err}
			}

			return messageMsg{text: "pushed " + p.what}
		},
		m.loadLog(revset),
	)
}
