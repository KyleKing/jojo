package gh_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/forge/github"
	"github.com/kyleking/aragonite/trail"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/gh/ghtest"
)

func TestPickRepo(t *testing.T) {
	t.Parallel()

	origin := gh.Remote{Name: "origin", URL: "git@github.com:me/fork.git"}
	upstream := gh.Remote{Name: "upstream", URL: "https://github.com/them/project"}

	tests := []struct {
		err     error
		name    string
		want    string
		remotes []gh.Remote
	}{
		{name: "one remote", remotes: []gh.Remote{origin}, want: "github.com/me/fork"},
		{
			name: "two naming one repository", want: "github.com/me/fork",
			remotes: []gh.Remote{origin, {Name: "mirror", URL: "https://github.com/me/fork.git"}},
		},
		{
			name: "gh's default wins", want: "github.com/them/project",
			remotes: []gh.Remote{origin, {Name: "upstream", URL: upstream.URL, Resolved: "base"}},
		},
		{
			name: "a default that is no remote's", want: "them/elsewhere",
			remotes: []gh.Remote{{Name: "origin", URL: origin.URL, Resolved: "them/elsewhere"}, upstream},
		},
		{name: "two with no default", remotes: []gh.Remote{origin, upstream}, err: gh.ErrAmbiguousRemote},
		{name: "none", err: gh.ErrNoRemote},
		{name: "a path on disk", remotes: []gh.Remote{{Name: "origin", URL: "/tmp/remote.git"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := gh.PickRepo(tc.remotes)
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Errorf("PickRepo = %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}

	_, err := gh.PickRepo([]gh.Remote{origin, upstream})
	if err == nil || !strings.Contains(err.Error(), "origin, upstream") ||
		!strings.Contains(err.Error(), "gh repo set-default") {
		t.Errorf("the ambiguous error does not say how to fix it: %v", err)
	}
}

// Every call carries --repo once it is known; a repository that cannot be named fails every call
// before gh runs, and is asked again on the next.
func TestCLI_PointsEveryCallAtTheRepo(t *testing.T) {
	t.Parallel()

	forge := ghtest.New(t)
	asked := 0
	resolve := func(context.Context) (string, error) {
		asked++
		if asked == 1 {
			return "", gh.ErrNoRemote
		}

		return "github.com/me/fork", nil
	}

	cli := gh.New(t.TempDir(), nil, gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env()), gh.WithRepo(resolve))

	if _, err := cli.PRFor(t.Context(), "feature"); !errors.Is(err, gh.ErrNoRemote) {
		t.Fatalf("a failed lookup ran gh anyway: %v", err)
	}

	if _, err := cli.PRFor(t.Context(), "feature"); err != nil {
		t.Fatalf("PRFor: %v", err)
	}

	if _, err := cli.PRFor(t.Context(), "feature"); err != nil {
		t.Fatalf("PRFor: %v", err)
	}

	calls := forge.Calls(t)
	if len(calls) != 2 || asked != 2 {
		t.Fatalf("%d calls after %d lookups, want 2 and 2: the answer is kept once known", len(calls), asked)
	}

	for _, c := range calls {
		if i := slices.Index(c.Args, "--repo"); i < 0 || c.Args[i+1] != "github.com/me/fork" {
			t.Errorf("%s does not name the repository", c.Line())
		}
	}
}

// The forge pane's polls run through Poll as aragonite's gh runner: each lands in the trail as a
// read, pointed at the repository, and gh's own words reach the caller.
func TestCLI_PollFilesThePanesCallsInTheTrail(t *testing.T) {
	t.Parallel()

	forge := ghtest.New(t)
	log := trail.New(0)
	repo := func(context.Context) (string, error) { return "github.com/me/fork", nil }
	cli := gh.New(t.TempDir(), log, gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env()), gh.WithRepo(repo))

	ctx := github.WithRunner(t.Context(), cli.Poll)

	_, err := github.GetPRForBranch(ctx, t.TempDir(), "github.com/me/fork", "feature", "")
	if err == nil || !strings.Contains(err.Error(), "no pull requests found") {
		t.Fatalf("a branch with no pull request answered %v, want gh's own words", err)
	}

	calls := log.Calls(false)
	if len(calls) != 1 || calls[0].Write || calls[0].Exit == 0 {
		t.Fatalf("trail = %+v, want the one failed read", calls)
	}

	line := calls[0].Shell()
	if !strings.Contains(line, "gh pr view feature") || !strings.Contains(line, "--repo github.com/me/fork") {
		t.Errorf("the trail files %q", line)
	}
}
