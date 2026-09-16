package forge_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyleking/aragonite/ghcassette"

	"github.com/kyleking/jojo/internal/forge"
)

// initRepo creates an empty git repository, optionally with an origin remote,
// so Detect has something local to read without a network call.
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

func TestDetect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		remoteURL     string
		wantHost      string
		wantSupported bool
	}{
		{name: "no remote at all", remoteURL: "", wantHost: "", wantSupported: false},
		{
			name:          "github remote is supported",
			remoteURL:     "https://github.com/kyleking/jojo.git",
			wantHost:      "github.com",
			wantSupported: true,
		},
		{
			name:          "gitlab remote names the host but has no provider yet",
			remoteURL:     "https://gitlab.com/kyleking/jojo.git",
			wantHost:      "gitlab.com",
			wantSupported: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := initRepo(t, tc.remoteURL)

			got := forge.Detect(context.Background(), dir)
			if got.Supported() != tc.wantSupported {
				t.Errorf("Supported() = %v, want %v", got.Supported(), tc.wantSupported)
			}

			if got.Host != tc.wantHost {
				t.Errorf("Host = %q, want %q", got.Host, tc.wantHost)
			}
		})
	}
}

// withCassette points this process's own gh calls at cassette for the
// duration of the test, so githubProvider (reached only through Detect and a
// Poller) never touches live GitHub.
func withCassette(t *testing.T, cassette string) {
	t.Helper()

	path, err := filepath.Abs(filepath.Join("testdata", cassette))
	if err != nil {
		t.Fatalf("resolving cassette path: %v", err)
	}

	ghcassette.Replay(t, path).Apply(t)
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestPoller_Fetch(t *testing.T) {
	withCassette(t, "pr_open.golden")

	dir := initRepo(t, "https://github.com/kyleking/jojo.git")

	detected := forge.Detect(context.Background(), dir)
	if !detected.Supported() {
		t.Fatal("expected a github provider")
	}

	poller := forge.NewPoller(detected.Provider, dir, detected.RemoteID, "feature",
		forge.WithDiskCache(nil))

	status, err := poller.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if status.PR == nil || status.PR.Number != 42 {
		t.Fatalf("PR = %+v, want number 42", status.PR)
	}

	if !status.Behind {
		t.Error("Behind = false, want true for mergeStateStatus BEHIND")
	}

	if status.Checks.Passing != 1 || status.Checks.Failing != 1 {
		t.Errorf("Checks = %+v, want 1 passing and 1 failing", status.Checks)
	}

	if !poller.FetchedLive() {
		t.Error("FetchedLive() = false after a successful Fetch")
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestPoller_Fetch_NoPullRequest(t *testing.T) {
	withCassette(t, "no_pr.golden")

	dir := initRepo(t, "https://github.com/kyleking/jojo.git")

	detected := forge.Detect(context.Background(), dir)

	poller := forge.NewPoller(detected.Provider, dir, detected.RemoteID, "no-pr-branch",
		forge.WithDiskCache(nil))

	status, err := poller.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v, want a bookmark with no pull request to render plainly", err)
	}

	if status.PR != nil {
		t.Errorf("PR = %+v, want nil", status.PR)
	}
}

//nolint:paralleltest // withCassette uses t.Setenv, which forbids Parallel
func TestPoller_Fetch_RateLimited(t *testing.T) {
	withCassette(t, "rate_limited.golden")

	dir := initRepo(t, "https://github.com/kyleking/jojo.git")

	detected := forge.Detect(context.Background(), dir)

	poller := forge.NewPoller(detected.Provider, dir, detected.RemoteID, "feature",
		forge.WithDiskCache(nil))

	_, err := poller.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch: want an error when the GraphQL budget is exhausted")
	}

	if !errors.Is(err, forge.ErrRateLimited) {
		t.Errorf("Fetch error = %v, want it to wrap forge.ErrRateLimited", err)
	}
}
