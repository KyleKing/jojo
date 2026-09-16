package jj_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

func TestWorkspacesMarksTheOneTheClientRunsIn(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "only change")

	spaces, err := client.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}

	if len(spaces) != 1 {
		t.Fatalf("expected one workspace, got %d", len(spaces))
	}

	if !spaces[0].Current {
		t.Error("the only workspace is not marked current")
	}

	if spaces[0].Name == "" || spaces[0].ChangeID == "" || spaces[0].Root == "" {
		t.Errorf("workspace is missing a field: %+v", spaces[0])
	}
}

func TestWorkspaceAddThenForget(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "only change")

	if err := client.WorkspaceAdd(ctx, "slow-tests", filepath.Join(dir, "..", "slow-tests")); err != nil {
		t.Fatalf("WorkspaceAdd: %v", err)
	}

	spaces, err := client.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}

	added, current := byName(spaces, "slow-tests"), byName(spaces, "default")
	if added == nil {
		t.Fatal("the added workspace is not listed")
	}

	if added.Current {
		t.Error("the added workspace is marked as the one the client runs in")
	}

	if current == nil || !current.Current {
		t.Error("the client's own workspace lost its current marking")
	}

	if err := client.WorkspaceForget(ctx, "slow-tests"); err != nil {
		t.Fatalf("WorkspaceForget: %v", err)
	}

	spaces, err = client.Workspaces(ctx)
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}

	if byName(spaces, "slow-tests") != nil {
		t.Error("the forgotten workspace is still listed")
	}
}

func byName(spaces []jj.Workspace, name string) *jj.Workspace {
	for i := range spaces {
		if spaces[i].Name == name {
			return &spaces[i]
		}
	}

	return nil
}
