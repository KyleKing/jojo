package model

import (
	"context"
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

func (m Model) loadFiles(rev jj.Revision) tea.Cmd {
	client, revision := m.client, rev.ChangeID

	return load(m.ctx,
		func(ctx context.Context) ([]jj.FileChange, error) { return client.Files(ctx, revision) },
		showFiles(rev))
}

func showFiles(rev jj.Revision) func(*Model, []jj.FileChange) tea.Cmd {
	return func(m *Model, files []jj.FileChange) tea.Cmd {
		if len(files) == 0 {
			m.message = rev.ChangeID + " changes no files"

			return nil
		}

		if rev.WorkingCopy {
			return m.files.OpenWorkingCopy(rev.ChangeID, rev.Description, files)
		}

		return m.files.Open(rev.ChangeID, rev.Description, files)
	}
}

// filesKey opens the file list, where space checks files for s, S, and (on @) c.
func (m Model) filesKey(rev jj.Revision) tea.Cmd {
	return m.loadFiles(rev)
}

// openCommit closes the file list, which would otherwise draw over the prompt, and commits the
// checked files with the message typed (`jj commit <paths> -m`).
func (m *Model) openCommit(msg fileview.CommitMsg) tea.Cmd {
	m.files.Close()

	paths := msg.Paths
	m.onSubmit = func(m Model, message string) (Model, tea.Cmd) {
		if strings.TrimSpace(message) == "" {
			return m, m.say("a commit needs a message")
		}

		cmd := m.act(joinPaths(paths), func(ctx context.Context, c *jj.Client) (string, error) {
			return c.CommitPaths(ctx, message, paths)
		})

		return m, cmd
	}

	return m.prompt.Open("commit "+countFiles(paths), msg.Message)
}

// openSplit closes the file list and splits the checked files out of the revision into a commit of
// their own, before what is left (`jj split -r <rev> <paths> -m`).
func (m *Model) openSplit(msg fileview.SplitMsg) tea.Cmd {
	m.files.Close()

	revision, paths := msg.Revision, msg.Paths
	m.onSubmit = func(m Model, message string) (Model, tea.Cmd) {
		if strings.TrimSpace(message) == "" {
			return m, m.say("a split needs a message for the commit it splits out")
		}

		cmd := m.act(joinPaths(paths), func(ctx context.Context, c *jj.Client) (string, error) {
			return c.SplitPaths(ctx, revision, message, paths)
		})

		return m, cmd
	}

	return m.prompt.Open(fmt.Sprintf("split %s out of %s", countFiles(paths), revision), msg.Message)
}

// joinPaths names the files a write acts on, for the status line.
func joinPaths(paths []string) string { return strings.Join(paths, " ") }

// countFiles is "a.txt" for one file and "3 files" for more.
func countFiles(paths []string) string {
	if len(paths) == 1 {
		return paths[0]
	}

	return fmt.Sprintf("%d files", len(paths))
}

func (m Model) handleFileMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case fileview.CommitMsg:
		cmd := m.openCommit(msg)

		return m, cmd, true
	case fileview.SplitMsg:
		cmd := m.openSplit(msg)

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
		cmd := m.openMoveFile(msg.Revision, msg.Paths)

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
	help.ActionSplit:       {Code: 'S', Text: "S"},
	help.ActionFileResolve: {Code: 'x', Text: "x"},
	keys.ActionCheck:       {Code: tea.KeySpace, Text: " "},
	help.ActionFileCommit:  {Code: 'c', Text: "c"},
	keys.ActionBack:        {Code: tea.KeyEscape},
	keys.ActionQuit:        {Code: tea.KeyEscape},
}

// dispatchList runs an action in the file list.
func (m Model) dispatchList(action string) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.listJump(action); handled {
		return next, cmd
	}

	if action == help.ActionHistory {
		return m.startHistory()
	}

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

// listJump is ' and the jump list in the file list, which jump through its rows as the log's do
// through revisions.
func (m Model) listJump(action string) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionLabelJump:
		if !m.files.StartLabels() {
			return m, m.say("nothing on screen to jump to"), true
		}

		return m, nil, true
	case keys.ActionJumpBack:
		if !m.files.JumpBack() {
			return m, m.say("no jump to go back to yet: gg, G, and ' each leave one"), true
		}

		return m, nil, true
	case keys.ActionJumpForward:
		if !m.files.JumpForward() {
			return m, m.say("nothing to go forward to: ctrl+o goes back first"), true
		}

		return m, nil, true
	}

	return m, nil, false
}

// routeLabel hands a key to whichever label jump is waiting for its label, the log's or the file
// list's, and reports whether one was.
func (m Model) routeLabel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.labelsShown():
		next, cmd := m.labelKey(msg)

		return next, cmd, true
	case m.files.IsOpen() && m.openModal() == ctxFiles && m.files.Labeling():
		next, cmd := m.fileLabelKey(msg)

		return next, cmd, true
	}

	return m, nil, false
}

// fileLabelKey takes a key while the file list's labels are up.
func (m Model) fileLabelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if _, missed := m.files.LabelKey(msg.String()); missed != "" {
		return m, m.say("no label " + missed)
	}

	return m, nil
}

// fileHints are the file list's footer keys as the user has them bound.
func fileHints(km help.Keymap) fileview.Hints {
	h := fileview.DefaultHints()
	for action, hint := range map[string]*keyhint.Hint{
		help.ActionFileOpen: &h.Diff, help.ActionFileRestore: &h.Restore, help.ActionFileSquash: &h.Squash,
		help.ActionSplit: &h.Split, help.ActionFileResolve: &h.Resolve, keys.ActionCheck: &h.Check,
		help.ActionFileCommit: &h.Commit,
		keys.ActionBack:       &h.Close,
	} {
		if bound, ok := km.Hint(action); ok {
			bound.What = hint.What
			*hint = bound
		}
	}

	return h
}

// diffHints are the full-screen diff's footer keys as the user has them bound, the read marks' among
// them while it shows one change's whole diff and marks can be kept.
func diffHints(km help.Keymap, reads bool) []keyhint.Hint {
	actions := []string{keys.ActionLineDown, keys.NextAction(keys.ObjFile), keys.NextAction(keys.ObjHunk)}
	if reads {
		actions = append(actions, keys.ActionRead, keys.NextAction(keys.ObjUnread), keys.ActionOnlyUnread)
	}

	actions = append(actions, keys.ActionAgain, keys.ActionViewSideBySide, keys.ActionBack)

	return tellOnlyUnread(km, km.Hints(actions...))
}
