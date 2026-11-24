package jj

import (
	"context"
	"os/exec"
	"strings"
)

// Fetch runs `jj git fetch` against every remote, which is what makes trunk current before a rebase.
// It is an operation like any other write, so undo takes it back.
func (c *Client) Fetch(ctx context.Context) (string, error) {
	return c.mutate(ctx, "git", "fetch", "--all-remotes")
}

// ResolveCommand returns `jj resolve` ready for tea.ExecProcess. Resolving runs the user's merge
// tool, which needs the real terminal, so this hands it over rather than capturing output.
func (c *Client) ResolveCommand(ctx context.Context, revision string, paths ...string) *exec.Cmd {
	args := append([]string{"resolve", "-r", revision}, filePatterns(paths)...)

	//nolint:gosec // G204: the binary is a literal; only the arguments vary and no shell is involved.
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = c.baseDir

	return cmd
}

// GitRemote is one of the repository's git remotes, and what gh's default repository says of it:
// gh repo set-default writes remote.<name>.gh-resolved, "base" on the remote it chose.
type GitRemote struct {
	Name       string
	URL        string
	GHResolved string
}

// GitRemotes lists the repository's git remotes with gh's mark on each. A remote list jj cannot read
// is an error; gh's marks are best effort, since a repository gh never touched has none.
func (c *Client) GitRemotes(ctx context.Context) ([]GitRemote, error) {
	out, err := c.executeJJ(ctx, "git", "remote", "list")
	if err != nil {
		return nil, err
	}

	var remotes []GitRemote

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, url, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok {
			remotes = append(remotes, GitRemote{Name: name, URL: strings.TrimSpace(url)})
		}
	}

	if len(remotes) == 0 {
		return nil, nil
	}

	resolved := c.ghResolved(ctx)
	for i := range remotes {
		remotes[i].GHResolved = resolved[remotes[i].Name]
	}

	return remotes, nil
}

// ghResolved is remote.<name>.gh-resolved for each remote that has one, read from the git directory
// jj keeps (`jj git root`), which a repository that is not colocated keeps under .jj.
func (c *Client) ghResolved(ctx context.Context) map[string]string {
	out := map[string]string{}

	root, err := c.executeJJ(ctx, "git", "root")
	if err != nil {
		return out
	}

	//nolint:gosec // G204: the binary is a literal; only the git directory varies and no shell is involved.
	cmd := exec.CommandContext(ctx, "git", "--git-dir", strings.TrimSpace(root), "config", "--get-regexp",
		`^remote\..*\.gh-resolved$`)

	text, err := cmd.Output()
	if err != nil {
		return out
	}

	for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}

		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".gh-resolved")
		out[name] = strings.TrimSpace(value)
	}

	return out
}

// FetchTracked runs `jj git fetch --tracked`: only the bookmarks this repository already tracks, from
// the remotes git.fetch names, which is the narrower sync.
func (c *Client) FetchTracked(ctx context.Context) (string, error) {
	return c.mutate(ctx, "git", "fetch", "--tracked")
}

// Trunk is the change id trunk() resolves to, which jj makes the root commit when no remote bookmark
// names a trunk.
func (c *Client) Trunk(ctx context.Context) (string, error) {
	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", "trunk()",
		"-T", "change_id.shortest(8)")

	return strings.TrimSpace(out), err
}

// RestackRevset is every mutable stack not already on trunk(), by its roots: what a sync moves onto
// trunk once a fetch has moved it.
const RestackRevset = "roots(trunk().. & mutable()) ~ trunk()+"

// RestackArgs is the rebase a sync offers: each stack RestackRevset names, onto trunk().
func RestackArgs() []string {
	return []string{"rebase", "-s", RestackRevset, "-o", "trunk()"}
}

// StackRoots is the change ids of RestackRevset, the stacks a restack would move.
func (c *Client) StackRoots(ctx context.Context) ([]string, error) {
	return c.ChangeIDs(ctx, RestackRevset)
}

// Restack rebases every stack StackRoots names onto trunk(), as one operation.
func (c *Client) Restack(ctx context.Context) (string, error) {
	return c.mutate(ctx, RestackArgs()...)
}

// ChangeIDs is the change ids revset names, newest first, as the log shortens them.
func (c *Client) ChangeIDs(ctx context.Context, revset string) ([]string, error) {
	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", revset,
		"-T", `change_id.shortest(8) ++ "\n"`)
	if err != nil {
		return nil, err
	}

	return strings.Fields(out), nil
}
