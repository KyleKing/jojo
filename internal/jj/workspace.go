package jj

import (
	"context"
	"strings"
)

// workspaceTemplate renders one line per workspace: its name, the change and commit its working copy
// sits on, that change's description, and the workspace root, which jj leaves empty for a workspace
// created before 0.38 or one whose directory has moved.
const workspaceTemplate = `self.name() ++ "` + fieldSep + `" ++ ` +
	`self.target().change_id().shortest(8) ++ "` + fieldSep + `" ++ ` +
	`self.target().description().first_line() ++ "` + fieldSep + `" ++ ` +
	`self.root() ++ "\n"`

const workspaceFieldCount = 4

// Workspaces lists every working copy attached to this repository. The one the client runs in is
// marked Current, since `jj workspace list` does not say which that is.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	out, err := c.executeJJ(ctx, "workspace", "list", "--color=never", "--no-pager", "-T", workspaceTemplate)
	if err != nil {
		return nil, err
	}

	root, err := c.Root(ctx)
	if err != nil {
		return nil, err
	}

	return parseWorkspaces(out, root), nil
}

func parseWorkspaces(out, root string) []Workspace {
	lines := strings.Split(out, "\n")

	spaces := make([]Workspace, 0, len(lines))

	for _, line := range lines {
		fields := strings.Split(line, fieldSep)
		if len(fields) != workspaceFieldCount {
			continue
		}

		spaces = append(spaces, Workspace{
			Name:        fields[0],
			ChangeID:    fields[1],
			Description: fields[2],
			Root:        fields[3],
			Current:     fields[3] != "" && fields[3] == root,
		})
	}

	return spaces
}

// WorkspaceAdd creates a workspace named name with its working copy at path.
func (c *Client) WorkspaceAdd(ctx context.Context, name, path string) (string, error) {
	return c.mutate(ctx, "workspace", "add", "--name", name, path)
}

// WorkspaceForget stops tracking name's working-copy commit. The directory itself is left on disk.
func (c *Client) WorkspaceForget(ctx context.Context, name string) (string, error) {
	return c.mutate(ctx, "workspace", "forget", name)
}

// UpdateStaleWorkspace recovers a working copy jj refuses to run against, which is also how jj
// re-imports a colocated git store it has drifted from.
func (c *Client) UpdateStaleWorkspace(ctx context.Context) (string, error) {
	return c.mutate(ctx, "workspace", "update-stale")
}
