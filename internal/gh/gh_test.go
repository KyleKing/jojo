package gh_test

import (
	"errors"
	"testing"

	"github.com/kyleking/aragonite/trail"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/gh/ghtest"
)

func TestParsePR_RollsChecksUpToTheWorst(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		rollup string
		want   string
	}{
		"none":    {`[]`, ""},
		"passing": {`[{"status":"COMPLETED","conclusion":"SUCCESS"},{"state":"SUCCESS"}]`, "pass"},
		"running": {
			`[{"status":"COMPLETED","conclusion":"SUCCESS"},{"status":"IN_PROGRESS"}]`, "pending",
		},
		"failing": {`[{"status":"IN_PROGRESS"},{"status":"COMPLETED","conclusion":"FAILURE"}]`, "fail"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pr, err := gh.ParsePR([]byte(`{"number":7,"state":"OPEN","statusCheckRollup":` + tc.rollup + `}`))
			if err != nil {
				t.Fatal(err)
			}

			if pr.Number != 7 || pr.Checks != tc.want {
				t.Errorf("got #%d checks %q, want %q", pr.Number, pr.Checks, tc.want)
			}
		})
	}

	if _, err := gh.ParsePR([]byte("not json")); err == nil {
		t.Error("ParsePR accepted garbage")
	}
}

func TestSummary_SaysStateReviewAndCI(t *testing.T) {
	t.Parallel()

	for want, pr := range map[string]*gh.PR{
		"#1 · open · approved · CI pass": {
			Number: 1, State: gh.StateOpen, ReviewDecision: "APPROVED", Checks: "pass",
		},
		"#2 · draft · changes": {Number: 2, State: gh.StateOpen, IsDraft: true, ReviewDecision: "CHANGES_REQUESTED"},
		"#3 · merged":          {Number: 3, State: gh.StateMerged, Checks: "fail"},
		"#4 · closed":          {Number: 4, State: gh.StateClosed},
		"":                     nil,
	} {
		if got := pr.Summary(); got != want {
			t.Errorf("Summary = %q, want %q", got, want)
		}
	}
}

func TestNumberOf(t *testing.T) {
	t.Parallel()

	if gh.NumberOf("https://github.com/o/r/pull/12") != 12 || gh.NumberOf("nonsense") != 0 {
		t.Error("NumberOf misread a URL")
	}
}

// Every call lands in the trail, a failure with what gh said; a missing pull request is an answer.
func TestCLI_FilesEveryCallInTheTrail(t *testing.T) {
	t.Parallel()

	forge := ghtest.New(t)
	log := trail.New(0)
	cli := gh.New(t.TempDir(), log, gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env()))
	ctx := t.Context()

	if err := cli.Installed(); err != nil {
		t.Fatal(err)
	}

	if pr, err := cli.PRFor(ctx, "nope"); pr != nil || err != nil {
		t.Fatalf("PRFor a branch with none = %v, %v", pr, err)
	}

	n, url, err := cli.Create(ctx, gh.CreateOptions{Head: "a", Base: "main", Title: "t", Draft: true}, "")
	if err != nil || n != 1 || url == "" {
		t.Fatalf("Create = %d %q %v", n, url, err)
	}

	if err := cli.Retarget(ctx, "a", "dev"); err != nil {
		t.Fatal(err)
	}

	if err := cli.SetBody(ctx, "a", "hello"); err != nil {
		t.Fatal(err)
	}

	pr, err := cli.PRFor(ctx, "a")
	if err != nil || pr.BaseRefName != "dev" || pr.Body != "hello" || !pr.IsDraft {
		t.Fatalf("PRFor = %+v, %v", pr, err)
	}

	assertTrail(t, cli, log)
}

// assertTrail fails one more call and checks every call is in the trail, the failure with its say.
func assertTrail(t *testing.T, cli *gh.CLI, log *trail.Trail) {
	t.Helper()

	if err := cli.Retarget(t.Context(), "missing", "x"); !errors.Is(err, gh.ErrFailed) {
		t.Errorf("Retarget of a missing PR = %v, want ErrFailed", err)
	}

	calls := log.Calls(false)
	if len(calls) != 6 || calls[0].Argv[0] != "gh" || !calls[0].Failed() || calls[0].Said() == "" {
		t.Fatalf("trail %+v", calls)
	}

	if writes := log.Calls(true); len(writes) != 4 {
		t.Errorf("%d writes in the trail, want create, two retargets, and an edit", len(writes))
	}

	if err := gh.New("", nil, gh.WithBinary("no-such-gh-binary")).Installed(); !errors.Is(err, gh.ErrNotInstalled) {
		t.Errorf("Installed = %v", err)
	}
}
