package jj_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// Every workspace of one repository answers the same store, the main one's .jj/repo directory, so
// state keyed by it is shared across them.
func TestRepoStore_SecondWorkspaceAnswersTheMainStore(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping")
	}

	base := t.TempDir()
	main := filepath.Join(base, "main")
	run(t, base, "git", "init", "main")
	run(t, main, "workspace", "add", filepath.Join(base, "second"))

	want, err := jj.RepoStore(main)
	if err != nil {
		t.Fatalf("RepoStore(main): %v", err)
	}

	if filepath.Base(want) != "repo" || filepath.Base(filepath.Dir(want)) != ".jj" {
		t.Errorf("the main workspace's store is %s, want its .jj/repo", want)
	}

	got, err := jj.RepoStore(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("RepoStore(second): %v", err)
	}

	if got != want {
		t.Errorf("the second workspace's store is %s, want the main one's %s", got, want)
	}

	if _, err := jj.RepoStore(t.TempDir()); err == nil {
		t.Error("a directory with no .jj answered a store")
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(t.Context(), "jj", args...)
	cmd.Dir = dir

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}
}

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

// setPushTemplate writes templates.git_push_bookmark into the repository's own config file, which
// leaves the user's config directory alone.
func setPushTemplate(t *testing.T, dir, template string) {
	t.Helper()

	config := "[user]\nname = \"Test User\"\nemail = \"test@example.com\"\n" +
		"[templates]\ngit_push_bookmark = '" + template + "'\n"

	if err := os.WriteFile(filepath.Join(dir, ".jj", "repo", "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}
