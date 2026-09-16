package jj

import (
	"context"
	"os/exec"
)

// Fetch runs `jj git fetch` against every remote, which is what makes trunk current before a rebase.
// It is an operation like any other write, so undo takes it back.
func (c *Client) Fetch(ctx context.Context) (string, error) {
	return c.mutate(ctx, "git", "fetch", "--all-remotes")
}

// ResolveCommand returns `jj resolve` ready for tea.ExecProcess. Resolving runs the user's merge
// tool, which needs the real terminal, so this hands it over rather than capturing output.
func (c *Client) ResolveCommand(ctx context.Context, revision string, paths ...string) *exec.Cmd {
	args := append([]string{"resolve", "-r", revision}, paths...)

	//nolint:gosec // G204: the binary is a literal; only the arguments vary and no shell is involved.
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = c.baseDir

	return cmd
}
