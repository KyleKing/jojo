package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/workspaceview"
	"github.com/kyleking/jojo/internal/jj"
)

// A workspace nested under the repository root would be snapshotted into its own commits, so the new
// working copy goes beside the repository.
func TestWorkspacePathSitsBesideTheRepository(t *testing.T) {
	t.Parallel()

	if got := workspacePath("/home/kyle/repo", "slow"); got != "/home/kyle/repo-slow" {
		t.Errorf("workspacePath = %q, want a sibling of the repository", got)
	}
}

func TestWorkspaceAddRefusesAnEmptyName(t *testing.T) {
	t.Parallel()

	cmd := testModel(t, twoRows()).addWorkspace("")
	if cmd == nil {
		t.Fatal("an empty name produced no command")
	}

	msg, ok := cmd().(messageMsg)
	if !ok {
		t.Fatalf("an empty name produced %T, want a message", cmd())
	}

	if msg.text != "a workspace needs a name" {
		t.Errorf("message is %q, want the empty-name message", msg.text)
	}
}

// Forgetting closes the panel, or the list still showing the forgotten workspace invites a second
// forget against a name jj no longer knows.
func TestForgetTargetsTheCursorRowAndClosesThePanel(t *testing.T) {
	t.Parallel()

	model := testModel(t, twoRows())

	model, _ = deliver(model, []jj.Workspace{{Name: "default", Current: true}, {Name: "slow"}}, nil, showWorkspaces)
	model = asModel(t, update(model, tea.KeyPressMsg{Code: 'j', Text: "j"}))

	next, cmd := model.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if cmd == nil {
		t.Fatal("f produced no command")
	}

	forget, ok := cmd().(workspaceview.ForgetMsg)
	if !ok {
		t.Fatalf("f produced %T, want a forget", cmd())
	}

	if forget.Name != "slow" {
		t.Errorf("forgetting %q, want the workspace under the cursor", forget.Name)
	}

	closed, _ := asModel(t, next).Update(forget)

	if asModel(t, closed).spaces.IsOpen() {
		t.Error("the panel stayed open after forgetting a workspace")
	}
}

func asModel(t *testing.T, m tea.Model) Model {
	t.Helper()

	model, ok := m.(Model)
	if !ok {
		t.Fatalf("expected a Model, got %T", m)
	}

	return model
}

func update(m Model, msg tea.Msg) tea.Model {
	next, _ := m.Update(msg)

	return next
}
