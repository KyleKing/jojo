package model

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/jj"
)

type bookmarksLoadedMsg struct {
	err    error
	target string
	marks  []jj.Bookmark
}

// pushPreviewMsg carries what a dry run says a push would do, which is what the confirmation shows.
type pushPreviewMsg struct {
	err    error
	name   string
	notice string
}

func (m Model) loadBookmarks() tea.Cmd {
	client, ctx := m.client, m.ctx
	target := ""

	if rev := m.log.Selected(); rev != nil {
		target = rev.ChangeID
	}

	return func() tea.Msg {
		marks, err := client.Bookmarks(ctx)

		return bookmarksLoadedMsg{marks: marks, target: target, err: err}
	}
}

func (m Model) applyBookmarks(msg bookmarksLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.message = msg.err.Error()

		return m, nil
	}

	cmd := m.bookmarks.Open(msg.marks, msg.target)

	return m, cmd
}

// handleBookmarkMsg runs what the panel asked for. Every one of these is an operation jj can undo,
// so only the push goes through a confirmation.
func (m Model) handleBookmarkMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case bookmarksLoadedMsg:
		next, cmd := m.applyBookmarks(msg)

		return next, cmd, true
	case bookmarkview.SetMsg:
		return m, m.bookmarkWrite("moved "+msg.Name+" onto "+msg.Target, func(ctx context.Context) error {
			return m.client.BookmarkSet(ctx, msg.Name, msg.Target)
		}), true
	case bookmarkview.AdvanceMsg:
		return m, m.bookmarkWrite("advanced "+msg.Name, func(ctx context.Context) error {
			return m.client.BookmarkAdvance(ctx, []string{msg.Name}, msg.Target)
		}), true
	case bookmarkview.CreateRequestedMsg:
		return m.openBookmarkName(pendingBookmarkCreate, "new bookmark on "+msg.Target, msg.Target), nil, true
	case bookmarkview.RenameRequestedMsg:
		return m.openBookmarkName(pendingBookmarkRename, "rename "+msg.Name+" to", msg.Name), nil, true
	case bookmarkview.DeleteMsg:
		return m, m.bookmarkWrite("deleted "+msg.Name, func(ctx context.Context) error {
			return m.client.BookmarkDelete(ctx, msg.Name)
		}), true
	case bookmarkview.ForgetMsg:
		return m, m.bookmarkWrite("forgot "+msg.Name, func(ctx context.Context) error {
			return m.client.BookmarkForget(ctx, msg.Name)
		}), true
	case bookmarkview.TrackMsg:
		return m, m.bookmarkWrite("tracking "+msg.Ref, func(ctx context.Context) error {
			return m.client.BookmarkTrack(ctx, msg.Ref)
		}), true
	case bookmarkview.UntrackMsg:
		return m, m.bookmarkWrite("stopped tracking "+msg.Ref, func(ctx context.Context) error {
			return m.client.BookmarkUntrack(ctx, msg.Ref)
		}), true
	case bookmarkview.PushMsg:
		return m, m.previewPush(msg.Name), true
	case pushPreviewMsg:
		return m.offerPush(msg)
	}

	return m, nil, false
}

// openBookmarkName collects a name for a bookmark verb the panel cannot supply one for.
func (m Model) openBookmarkName(what pending, title, seed string) Model {
	m.bookmarks.Close()
	m.pending = what
	m.pendingOn = seed
	m.prompt.Open(title, "")

	return m
}

// dryRunNotice is the line jj adds to say it stopped short. The dialog is the thing about to not
// stop short, so the line would only confuse what is being agreed to.
const dryRunNotice = "Dry-run requested, not pushing."

// noticeLines turns jj's dry-run notice into the detail block, dropping blanks so it stays one
// screen.
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

func (m Model) bookmarkWrite(done string, write func(context.Context) error) tea.Cmd {
	return m.run(undoable(done), write)
}

// previewPush asks jj what the push would do before anything leaves the machine, so the confirmation
// shows the real answer rather than a description of it.
func (m Model) previewPush(name string) tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		notice, err := client.Push(ctx, jj.PushOptions{Bookmarks: []string{name}, DryRun: true})

		return pushPreviewMsg{name: name, notice: notice, err: err}
	}
}

func (m Model) offerPush(msg pushPreviewMsg) (tea.Model, tea.Cmd, bool) {
	if msg.err != nil {
		m.message = pushProblem(msg.err)

		return m, nil, true
	}

	m.pushing = jj.PushOptions{Bookmarks: []string{msg.name}}

	cmd := m.confirm.Open(confirm.Request{
		Key:    confirmPush,
		Title:  "push " + msg.name + "?",
		Detail: noticeLines(msg.notice),
		Danger: "a push leaves this machine, so u cannot take it back",
	})

	return m, cmd, true
}

// pushProblem turns jj's refusal into the line that gets the user out of it, because a push is
// refused for reasons that each have one obvious fix.
func pushProblem(err error) string {
	text := strings.TrimPrefix(err.Error(), jj.ErrPushRefused.Error()+": ")

	if !errors.Is(err, jj.ErrPushRefused) {
		return text
	}

	// jj's own hint names the rejected commit, which the row under the cursor already shows, so the
	// advice takes its place rather than sharing the one status line with it.
	headline, _, _ := strings.Cut(text, " (")

	switch {
	case strings.Contains(headline, "no description"):
		return headline + ". Press d to describe it"
	case strings.Contains(headline, "empty"):
		return headline + ". An empty revision has nothing to push"
	case strings.Contains(headline, "conflict"):
		return headline + ". Resolve the conflict first"
	}

	return text
}

func (m Model) push(opts jj.PushOptions) tea.Cmd {
	client, ctx, revset := m.client, m.ctx, m.revset

	return tea.Sequence(
		func() tea.Msg {
			if _, err := client.Push(ctx, opts); err != nil {
				return messageMsg{text: pushProblem(err)}
			}

			return messageMsg{text: "pushed " + strings.Join(opts.Bookmarks, ", ")}
		},
		m.loadLog(revset),
	)
}
