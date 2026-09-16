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

// pushPreviewMsg carries what a dry run says a push would do, which is what the confirmation shows.
type pushPreviewMsg struct {
	err    error
	name   string
	notice string
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
		return m, m.previewPush(msg.Name), true
	case pushPreviewMsg:
		return m.offerPush(msg)
	}

	return m, nil, false
}

// askBookmarkName collects a name for a bookmark verb the panel cannot supply one for.
func (m Model) askBookmarkName(title string, then submitted) Model {
	m.bookmarks.Close()
	m.onSubmit = then
	m.prompt.Open(title, "")

	return m
}

// dryRunNotice is jj saying it stopped short, which would only confuse what the dialog agrees to.
const dryRunNotice = "Dry-run requested, not pushing."

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
