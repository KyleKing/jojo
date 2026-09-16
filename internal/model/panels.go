package model

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
)

// panelMsg is a finished background read that knows how to open its own panel, so every panel shares
// one load path and one place a failure is reported.
type panelMsg interface {
	open(m *Model) tea.Cmd
}

type loaded[T any] struct {
	err   error
	show  func(*Model, T) tea.Cmd
	value T
}

func (l loaded[T]) open(m *Model) tea.Cmd {
	if l.err != nil {
		m.message = l.err.Error()

		return nil
	}

	return l.show(m, l.value)
}

func load[T any](ctx context.Context, read func(context.Context) (T, error), show func(*Model, T) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		value, err := read(ctx)

		return loaded[T]{value: value, err: err, show: show}
	}
}

// opLogLimit caps the operation log, which grows without bound in an active repository.
const opLogLimit = 200

func showOpLog(m *Model, ops []jj.Operation) tea.Cmd { return m.opLog.Open(ops) }

func showWorkspaces(m *Model, spaces []jj.Workspace) tea.Cmd { return m.spaces.Open(spaces) }

func showDetail(m *Model, detail jj.Detail) tea.Cmd { return m.detail.Open(detail) }

func (m Model) loadOpLog() tea.Cmd {
	client := m.client

	return load(m.ctx,
		func(ctx context.Context) ([]jj.Operation, error) { return client.OpLog(ctx, opLogLimit) },
		showOpLog)
}

func (m Model) loadWorkspaces() tea.Cmd {
	return load(m.ctx, m.client.Workspaces, showWorkspaces)
}

func (m Model) loadEvolog(changeID string) tea.Cmd {
	client := m.client

	return load(m.ctx,
		func(ctx context.Context) ([]jj.Evolution, error) { return client.Evolog(ctx, changeID) },
		showEvolog(changeID))
}

func showEvolog(changeID string) func(*Model, []jj.Evolution) tea.Cmd {
	return func(m *Model, entries []jj.Evolution) tea.Cmd { return m.evolog.Open(changeID, entries) }
}

func (m Model) loadDetail(rev string) tea.Cmd {
	client := m.client

	return load(m.ctx,
		func(ctx context.Context) (jj.Detail, error) { return client.RevisionDetail(ctx, rev) },
		showDetail)
}

// loadBookmarks carries the selected revision along, because the panel acts against it and the log
// cursor may have moved by the time the read returns.
func (m Model) loadBookmarks() tea.Cmd {
	client, target := m.client, ""

	if rev := m.log.Selected(); rev != nil {
		target = rev.ChangeID
	}

	return load(m.ctx, client.Bookmarks, showBookmarks(target))
}

func showBookmarks(target string) func(*Model, []jj.Bookmark) tea.Cmd {
	return func(m *Model, marks []jj.Bookmark) tea.Cmd { return m.bookmarks.Open(marks, target) }
}

// deliver hands a finished read to the model the way the runtime would, so a test can open a panel
// without a repository behind it.
func deliver[T any](m Model, value T, err error, show func(*Model, T) tea.Cmd) (Model, tea.Cmd) {
	cmd := loaded[T]{value: value, err: err, show: show}.open(&m)

	return m, cmd
}
