package forgepane_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/ghcassette"
	"github.com/kyleking/aragonite/tui/region"

	"github.com/kyleking/jojo/internal/components/forgepane"
	"github.com/kyleking/jojo/internal/forge"
	"github.com/kyleking/jojo/internal/tuitest"
)

func initRepo(t *testing.T, remoteURL string) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // fixed args in a test
		cmd.Dir = dir

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	run("init", "-q")

	if remoteURL != "" {
		run("remote", "add", "origin", remoteURL)
	}

	return dir
}

func withCassette(t *testing.T, cassette string) {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("..", "..", "forge", "testdata", cassette))
	if err != nil {
		t.Fatalf("resolving cassette path: %v", err)
	}

	ghcassette.Replay(t, path).Apply(t)
}

func testStyles() forgepane.Styles {
	return forgepane.Styles{Region: region.Styles{}}
}

func newModel(t *testing.T, remoteURL, bookmark string) forgepane.Model {
	t.Helper()

	dir := initRepo(t, remoteURL)

	m := forgepane.New(context.Background(), testStyles(), dir, bookmark, forge.WithDiskCache(nil))
	m.SetSize(60, 12)

	return m
}

func TestView_UnsupportedHostRendersPlainly(t *testing.T) {
	t.Parallel()

	m := newModel(t, "https://gitlab.com/kyleking/jojo.git", "feature")

	view := m.View()
	if strings.Contains(view, "error") || strings.Contains(view, "Error") {
		t.Errorf("view = %q, want no error language for an unimplemented host", view)
	}

	if !strings.Contains(view, "gitlab.com") {
		t.Errorf("view = %q, want it to name the unsupported host", view)
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestView_NoPullRequestRendersPlainly(t *testing.T) {
	withCassette(t, "no_pr.golden")

	m := newModel(t, "https://github.com/kyleking/jojo.git", "no-pr-branch")

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() = nil, want a fetch command")
	}

	m.Update(cmd())

	view := m.View()
	if strings.Contains(view, "error") || strings.Contains(view, "Error") {
		t.Errorf("view = %q, want no error language for a bookmark with no pull request", view)
	}

	if !strings.Contains(view, "no pull request") {
		t.Errorf("view = %q, want it to say the bookmark has no pull request", view)
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestView_ThreeStatesAreDistinguishable(t *testing.T) {
	withCassette(t, "pr_open.golden")

	m := newModel(t, "https://github.com/kyleking/jojo.git", "feature")

	stale := m.View()
	if strings.Contains(stale, "live") {
		t.Errorf("stale view = %q, should not claim to be live before any fetch completes", stale)
	}

	fetchCmd := m.Init()
	fetching := m.View()

	if fetching == stale {
		t.Error("fetching view should differ from the stale view that preceded it")
	}

	m.Update(fetchCmd())

	fresh := m.View()
	if fresh == stale || fresh == fetching {
		t.Error("fresh view should differ from both the stale and fetching views")
	}

	if !strings.Contains(fresh, "live") {
		t.Errorf("fresh view = %q, want it to say the data is live", fresh)
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestView_RateLimitSurfacesAsState(t *testing.T) {
	withCassette(t, "rate_limited.golden")

	m := newModel(t, "https://github.com/kyleking/jojo.git", "feature")

	cmd := m.Init()
	m.Update(cmd())

	view := m.View()
	if !strings.Contains(view, "rate limit") {
		t.Errorf("view = %q, want the rate limit error surfaced in the pane", view)
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestPoll_BacksOffWhenUnfocused(t *testing.T) {
	withCassette(t, "no_pr.golden")

	m := newModel(t, "https://github.com/kyleking/jojo.git", "no-pr-branch")

	cmd := m.Init()
	if got := m.Update(cmd()); got != nil {
		t.Error("Update(FetchedMsg) scheduled another tick while unfocused, want polling paused")
	}

	if focusCmd := m.SetFocused(true); focusCmd == nil {
		t.Error("SetFocused(true) = nil, want polling to resume")
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestRenderGoldenStates(t *testing.T) {
	withCassette(t, "pr_open.golden")

	m := newModel(t, "https://github.com/kyleking/jojo.git", "feature")

	cmd := m.Init()
	m.Update(cmd())

	tuitest.AssertGolden(t, m.View())
}
