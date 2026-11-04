package jj_test

import (
	"context"
	"errors"
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

	if _, err := client.WorkspaceAdd(ctx, "slow-tests", filepath.Join(dir, "..", "slow-tests")); err != nil {
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

	if _, err := client.WorkspaceForget(ctx, "slow-tests"); err != nil {
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

func TestUpdateStaleWorkspace_RecoversTheCheckout(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	ctx := context.Background()

	second := filepath.Join(t.TempDir(), "two")
	runJJ(t, dir, "workspace", "add", "--name", "two", second)

	// The checkout goes stale when an operation elsewhere removes the commit its snapshot
	// recorded; an untouched working copy quietly recovers instead.
	writeFile(t, dir, "dirty.txt", "dirty\n")
	runJJ(t, dir, "status")
	runJJ(t, second, "abandon", "default@")

	if _, err := client.Log(ctx, "", 10); !errors.Is(err, jj.ErrStaleWorkingCopy) {
		t.Fatalf("Log err = %v, want ErrStaleWorkingCopy", err)
	}

	if _, err := client.UpdateStaleWorkspace(ctx); err != nil {
		t.Fatalf("UpdateStaleWorkspace: %v", err)
	}

	if _, err := client.Log(ctx, "", 10); err != nil {
		t.Fatalf("Log after update-stale: %v", err)
	}
}
