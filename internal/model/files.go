package model

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/jj"
)

func (m Model) loadFiles(revision string) tea.Cmd {
	client := m.client

	return load(m.ctx,
		func(ctx context.Context) ([]jj.FileChange, error) { return client.Files(ctx, revision) },
		showFiles(revision))
}

func showFiles(revision string) func(*Model, []jj.FileChange) tea.Cmd {
	return func(m *Model, files []jj.FileChange) tea.Cmd {
		if len(files) == 0 {
			m.message = revision + " changes no files"

			return nil
		}

		return m.files.Open(revision, files)
	}
}

func (m Model) handleFileMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case fileview.DiffMsg:
		return m, m.showDiff(jj.DiffTarget{Revision: msg.Revision, Paths: []string{msg.Path}}), true
	case fileview.RestoreMsg:
		path, revision := msg.Path, msg.Revision
		m.files.Close()

		cmd := m.act(path, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.RestoreFile(ctx, revision, path)
		})

		return m, cmd, true
	case fileview.ResolveMsg:
		m.files.Close()

		return m, m.resolve(msg.Revision, msg.Path), true
	}

	return m, nil, false
}

// resolveKey refuses on a clean revision rather than letting jj report an error the row already
// says is impossible.
func (m Model) resolveKey(rev jj.Revision) tea.Cmd {
	if !rev.Conflict {
		return m.say(rev.ChangeID + " has no conflict to resolve")
	}

	return m.resolve(rev.ChangeID)
}

// resolve hands the terminal to jj, because resolving runs whichever merge tool the repository
// configures and that tool owns the screen while it runs.
func (m Model) resolve(revision string, paths ...string) tea.Cmd {
	return m.execCommand("resolved"+undoHint, m.client.ResolveCommand(m.ctx, revision, paths...))
}

// fetch brings every remote current, which is what a rebase onto trunk needs first.
func (m Model) fetch() tea.Cmd {
	return m.run(undoHint, m.client.Fetch)
}
