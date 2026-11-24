package jj_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/hermetic"
)

// PushNames is the bookmark `jj git push -c` would create, from templates.git_push_bookmark, and
// Described carries it, so a submit and jj agree on the name.
func TestPushNames_FollowJJsTemplate(t *testing.T) {
	t.Parallel()

	client, dir := testRepo(t)
	runJJ(t, dir, "describe", "-m", "first")

	id := strings.TrimSpace(logOne(t, dir, "change_id.shortest(8)"))
	short := strings.TrimSpace(logOne(t, dir, "change_id.short()"))

	names, err := client.PushNames(t.Context(), []string{id})
	if err != nil || names[id] != "push-"+short {
		t.Fatalf("default names = %v (%v), want push-%s", names, err, short)
	}

	setPushTemplate(t, dir, `"me/" ++ change_id.short(6)`)

	described, err := client.Described(t.Context(), []string{id})
	if err != nil || described[id].PushName != "me/"+short[:6] {
		t.Errorf("Described = %+v (%v), want the configured me/%s", described[id], err, short[:6])
	}

	setPushTemplate(t, dir, `"broken" ++`)

	if names, err := client.PushNames(t.Context(), []string{id}); err != nil || names[id] != "push-"+short {
		t.Errorf("a template jj cannot read answered %v (%v), want jj's default", names, err)
	}
}

func logOne(t *testing.T, dir, template string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(t.Context(), "jj", "log", "--no-graph", "-r", "@", "-T", template)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jj log: %v", err)
	}

	return string(out)
}

// setPushTemplate sets templates.git_push_bookmark in the repository's own config.
func setPushTemplate(t *testing.T, dir, template string) {
	t.Helper()

	hermetic.RepoConfig(t, dir, "[templates]\ngit_push_bookmark = '"+template+"'")
}
