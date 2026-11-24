package model

import (
	"context"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
)

// Values config.toml's diff_tool takes besides a tool's name.
const (
	diffToolAuto = "auto"
	diffToolNone = "none"
	// The sibling tool's diff-editor mode (`jj-diff $left $right`) is the argument shape jj passes a
	// tool it has no configuration for, so naming it needs no merge-tools entry.
	jjDiff = "jj-diff"
)

// diffToolMsg carries the diff editor the interactive verbs name, "" to leave it to jj's config.
type diffToolMsg struct {
	tool string
}

// lookPath is how a binary on PATH is found, a variable so a test can say what is installed.
type lookPath func(string) (string, error)

// resolveDiffTool decides once what --tool the hunk verbs pass. A diff editor the user configured in
// jj wins over the sibling tool being installed, and jojo's own diff_tool wins over both.
func resolveDiffTool(ctx context.Context, preference string, configured func(context.Context) (bool, error),
	find lookPath,
) string {
	switch preference {
	case diffToolNone:
		return ""
	case "", diffToolAuto:
	default:
		return preference
	}

	if _, err := find(jjDiff); err != nil {
		return ""
	}

	if set, err := configured(ctx); err != nil || set {
		return ""
	}

	return jjDiff
}

func (m Model) loadDiffTool() tea.Cmd {
	client, ctx, preference := m.client, m.ctx, m.diffPreference

	return func() tea.Msg {
		return diffToolMsg{tool: resolveDiffTool(ctx, preference, client.DiffEditorConfigured, exec.LookPath)}
	}
}

// hunkArgs names the diff editor on a verb that hands hunk selection to one.
func (m Model) hunkArgs(args ...string) []string {
	if m.diffTool == "" {
		return args
	}

	return append(args, "--tool", m.diffTool)
}

// squashHunkArgs is the interactive move of part of from into into.
func (m Model) squashHunkArgs(from, into string) []string {
	return m.hunkArgs(append(jj.SquashArgs(from, into, nil), "--interactive")...)
}
