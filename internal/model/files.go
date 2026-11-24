package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/jjdiff"
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

// workingCopyFiles is what the file list of @ shows: its own files as unstaged, and the index below.
type workingCopyFiles struct {
	staging fileview.Staging
	files   []jj.FileChange
}

// loadWorkingCopyFiles reads the files of @ and of the parent standing in for the index. A parent
// that cannot be the index is not an error here: the list still opens, and says why when a staging
// key is pressed. A refresh only redraws a list still open, so a write finishing after the list was
// closed does not reopen it.
func (m Model) loadWorkingCopyFiles(revision string, refresh bool) tea.Cmd {
	client := m.client

	return load(m.ctx,
		func(ctx context.Context) (workingCopyFiles, error) {
			return readWorkingCopyFiles(ctx, client, revision)
		},
		showWorkingCopyFiles(revision, refresh))
}

func readWorkingCopyFiles(ctx context.Context, client *jj.Client, revision string) (workingCopyFiles, error) {
	files, err := client.Files(ctx, revision)
	if err != nil {
		return workingCopyFiles{}, fmt.Errorf("listing the files of %s: %w", revision, err)
	}

	index, err := client.Index(ctx)

	switch {
	case errors.Is(err, jj.ErrNoIndex):
		return workingCopyFiles{files: files, staging: fileview.Staging{Refused: err.Error()}}, nil
	case err != nil:
		return workingCopyFiles{}, fmt.Errorf("reading the index: %w", err)
	}

	staged, err := client.Files(ctx, index.ChangeID)
	if err != nil {
		return workingCopyFiles{}, fmt.Errorf("listing what is staged in %s: %w", index.ChangeID, err)
	}

	return workingCopyFiles{
		files:   files,
		staging: fileview.Staging{Index: index.ChangeID, Message: index.Description, Files: staged},
	}, nil
}

func showWorkingCopyFiles(revision string, refresh bool) func(*Model, workingCopyFiles) tea.Cmd {
	return func(m *Model, wc workingCopyFiles) tea.Cmd {
		if refresh && !m.files.IsOpen() {
			return nil
		}

		if !refresh && len(wc.files) == 0 && len(wc.staging.Files) == 0 {
			m.message = revision + " changes no files and nothing is staged"

			return nil
		}

		return m.files.OpenWorkingCopy(revision, wc.files, wc.staging)
	}
}

// filesKey opens the file list, which for the working copy also lists what is staged beneath it.
func (m Model) filesKey(rev jj.Revision) tea.Cmd {
	if rev.WorkingCopy {
		return m.loadWorkingCopyFiles(rev.ChangeID, false)
	}

	return m.loadFiles(rev.ChangeID)
}

// stagingWrite runs one staging verb and redraws the list it was pressed in, which stays open so a
// run of files can be staged one after another.
func (m Model) stagingWrite(revision string, write func(context.Context) (string, error)) tea.Cmd {
	return tea.Sequence(m.run(undoHint, write), m.loadWorkingCopyFiles(revision, true))
}

// openCommit closes the file list, which would otherwise draw over the prompt, and reopens it once
// the commit lands so the next round of staging starts where this one left off.
func (m *Model) openCommit(msg fileview.CommitMsg) tea.Cmd {
	m.files.Close()

	revision := msg.Revision
	m.onSubmit = func(m Model, message string) (Model, tea.Cmd) {
		if strings.TrimSpace(message) == "" {
			return m, tea.Batch(m.say("a commit needs a message"), m.loadWorkingCopyFiles(revision, false))
		}

		client := m.client
		cmd := tea.Sequence(
			m.run(", press u twice to undo", func(ctx context.Context) (string, error) {
				return client.CommitIndex(ctx, message)
			}),
			m.loadWorkingCopyFiles(revision, false),
		)

		return m, cmd
	}

	return m.prompt.Open("commit what is staged", msg.Message)
}

func (m Model) handleFileMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case fileview.StageMsg:
		path, client := msg.Path, m.client

		return m, m.stagingWrite(msg.Revision, func(ctx context.Context) (string, error) {
			return client.Stage(ctx, path)
		}), true
	case fileview.UnstageMsg:
		path, client := msg.Path, m.client

		return m, m.stagingWrite(msg.Revision, func(ctx context.Context) (string, error) {
			return client.Unstage(ctx, path)
		}), true
	case fileview.CommitMsg:
		cmd := m.openCommit(msg)

		return m, cmd, true
	case fileview.DiffMsg:
		return m, m.showDiff(jj.DiffTarget{Revision: msg.Revision, Paths: []string{msg.Path}}), true
	case fileview.RestoreMsg:
		path, revision := msg.Path, msg.Revision
		m.files.Close()

		cmd := m.act(path, func(ctx context.Context, c *jj.Client) (string, error) {
			return c.RestoreFile(ctx, revision, path)
		})

		return m, cmd, true
	case fileview.MoveMsg:
		cmd := m.openMoveFile(msg.Revision, msg.Path)

		return m, cmd, true
	case fileview.ResolveMsg:
		m.files.Close()

		return m, m.resolve(msg.Revision, msg.Path), true
	}

	return m, nil, false
}

// resolveKey refuses on a clean revision rather than letting jj report an error the row already
// says is impossible, and opens jj-diff's conflict viewer on a conflicted one.
func (m Model) resolveKey(rev jj.Revision) tea.Cmd {
	if !rev.Conflict {
		return m.say(rev.ChangeID + " has no conflict to resolve")
	}

	return m.openJJDiff(jjdiff.Resolve, rev.ChangeID)
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

// fileKeys are the keys the file list reads for itself, by the action that presses them, so a
// rebound file verb reaches a list that only knows its own letters.
var fileKeys = map[string]tea.KeyPressMsg{
	help.ActionFileOpen:    {Code: tea.KeyEnter},
	help.ActionFileRestore: {Code: 'R', Text: "R"},
	help.ActionFileSquash:  {Code: 's', Text: "s"},
	help.ActionFileResolve: {Code: 'x', Text: "x"},
	help.ActionFileStage:   {Code: tea.KeySpace, Text: " "},
	help.ActionFileCommit:  {Code: 'c', Text: "c"},
	keys.ActionBack:        {Code: tea.KeyEscape},
	keys.ActionQuit:        {Code: tea.KeyEscape},
}

// dispatchList runs an action in the file list.
func (m Model) dispatchList(action string) (tea.Model, tea.Cmd) {
	if key, ok := navKey(action); ok {
		return m, m.files.Update(key)
	}

	if key, ok := fileKeys[action]; ok {
		return m, m.files.Update(key)
	}

	if next, cmd, handled := m.globalAction(action); handled {
		return next, cmd
	}

	return m, nil
}

// fileHints are the file list's footer keys as the user has them bound.
func fileHints(km help.Keymap) fileview.Hints {
	h := fileview.DefaultHints()
	for action, hint := range map[string]*keyhint.Hint{
		help.ActionFileOpen: &h.Diff, help.ActionFileRestore: &h.Restore, help.ActionFileSquash: &h.Squash,
		help.ActionFileResolve: &h.Resolve, help.ActionFileStage: &h.Stage, help.ActionFileCommit: &h.Commit,
		keys.ActionBack: &h.Close,
	} {
		if bound, ok := km.Hint(action); ok {
			bound.What = hint.What
			*hint = bound
		}
	}

	return h
}

// diffHints are the full-screen diff's footer keys as the user has them bound.
func diffHints(km help.Keymap) []keyhint.Hint {
	return km.Hints(keys.ActionLineDown, keys.NextAction(keys.ObjFile), keys.NextAction(keys.ObjHunk),
		keys.ActionAgain, keys.ActionViewSideBySide, keys.ActionBack)
}
