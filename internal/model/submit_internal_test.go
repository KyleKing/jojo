package model

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/gh/ghtest"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/stack"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// fakePlanInput is a stack of four on main: one merged, one to retarget after a reorder, one open
// and pushed again, and one new, so the plan screen draws every action it has a word for.
func fakePlanInput() stack.Input {
	changes := []stack.Change{
		{
			ID: "kxqpmnzo", LongID: "kxqpmnzoaaaa", Subject: "feat(parse): read the new header format",
			Bookmark: "parse-header", Pushed: true,
		},
		{
			ID: "vwtrlsqy", LongID: "vwtrlsqybbbb", Subject: "refactor: split the reader",
			Bookmark: "split-reader", Pushed: true,
		},
		{
			ID: "nnzolpqr", LongID: "nnzolpqrcccc", Subject: "fix: keep the fallback order",
			Bookmark: "push-nnzolpqrcccc",
		},
		{ID: "yqosmzxt", LongID: "yqosmzxtdddd", Subject: "test: cover the fallbacks"},
	}

	return stack.Input{
		Trunk: "main", Changes: changes, Draft: true, Repo: "github.com/kyleking/jojo",
		Skipped: []stack.Change{{ID: "wkwyyxvq", WorkingCopy: true, Empty: true}},
		PRs: map[string]*gh.PR{
			"parse-header": {Number: 41, State: gh.StateMerged, BaseRefName: "main", HeadRefName: "parse-header"},
			"split-reader": {
				Number: 42, State: gh.StateOpen, BaseRefName: "parse-header", ReviewDecision: "APPROVED",
				Checks: "pass",
			},
			"push-nnzolpqrcccc": {
				Number: 43, State: gh.StateOpen, IsDraft: true, BaseRefName: "split-reader", Checks: "pending",
			},
		},
	}
}

// planFrame is the outline's fake stack with the plan screen over it.
func planFrame(t *testing.T, width, height int) Model {
	t.Helper()

	m := outlineFrame(t, width, height)
	m.tick = instantTicks
	m.submit.active = true
	m.submit.view.SetLoading()
	m.resize()

	return asModel(t, update(m, submitReadMsg{in: fakePlanInput(), seq: m.submit.seq}))
}

func TestSubmit_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := planFrame(t, size[0], size[1])
			frame := m.render()
			assertFits(t, frame, size[0], size[1], 0)

			plain := tuitest.StripANSI(frame)
			for _, want := range []string{
				"Submit", "merged", "retarget", "new", "gh pr create", "--repo", "enter submit", "esc cancel",
			} {
				if !strings.Contains(plain, want) {
					t.Errorf("the plan lacks %q:\n%s", want, plain)
				}
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

// -d replans in place: the create loses --draft, and nothing is asked of the forge again.
func TestSubmit_DraftFlagRebuildsTheCommands(t *testing.T) {
	t.Parallel()

	m := planFrame(t, 100, 30)
	if !strings.Contains(strings.Join(commandLines(m), "\n"), "--draft") {
		t.Fatal("a plan opened without --draft")
	}

	m = press(t, m, "-", "d")

	if strings.Contains(strings.Join(commandLines(m), "\n"), "--draft") || m.submit.draft {
		t.Errorf("-d left --draft on:\n%s", strings.Join(commandLines(m), "\n"))
	}

	// The rebuilt plan still names the repository every gh call runs against.
	for _, line := range commandLines(m) {
		if strings.HasPrefix(line, "gh ") && !strings.Contains(line, "--repo github.com/kyleking/jojo") {
			t.Errorf("-d dropped --repo from %q", line)
		}
	}
}

// assertSubmitted checks the forge holds three ready pull requests stacked on main, and the trail
// every gh call the forge saw.
func assertSubmitted(t *testing.T, m Model, forge ghtest.Forge) {
	t.Helper()

	prs := forge.PRs(t)
	if len(prs) != 3 || prs[0].IsDraft || prs[1].BaseRefName != prs[0].HeadRefName || prs[0].BaseRefName != "main" {
		t.Fatalf("the forge holds %+v, want three ready pull requests stacked on main", prs)
	}

	var ghCalls int

	for _, c := range m.trail.Calls(false) {
		if c.Argv[0] == "gh" {
			ghCalls++
		}
	}

	if ghCalls != len(forge.Calls(t)) {
		t.Errorf("the trail holds %d gh calls, the forge saw %d", ghCalls, len(forge.Calls(t)))
	}
}

func commandLines(m Model) []string {
	out := make([]string, 0, len(m.submit.view.Plan().Commands))
	for _, c := range m.submit.view.Plan().Commands {
		out = append(out, c.Line())
	}

	return out
}

// Esc backs out of the plan to the outline and runs nothing.
func TestSubmit_EscCancels(t *testing.T) {
	t.Parallel()

	m := press(t, planFrame(t, 100, 30), "esc")

	if m.submit.active || !m.outline.active || !strings.Contains(m.message, "nothing ran") {
		t.Errorf("esc left submit=%v outline=%v, said %q", m.submit.active, m.outline.active, m.message)
	}
}

// The gate names what blocks a submit, and P says it instead of opening a plan.
func TestSubmit_RefusesAConflictedStack(t *testing.T) {
	t.Parallel()

	m := outlineFrame(t, 100, 30)
	m.tick = instantTicks
	m.submit.ghMissing = nil // the refusal under test is the stack's, wherever gh is
	m = press(t, m, "P")

	if m.submit.active {
		t.Fatal("P opened a plan over a conflicted stack")
	}

	for _, want := range []string{"vwtrlsqy is conflicted", "nnzolpqr is empty"} {
		if !strings.Contains(m.message, want) {
			t.Errorf("the refusal %q does not say %q", m.message, want)
		}
	}
}

// remoteRepo is a repository with main pushed to a bare remote and three described changes on it,
// the empty working copy on top.
func remoteRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping")
	}

	dir, remote := t.TempDir(), filepath.Join(t.TempDir(), "remote.git")

	//nolint:gosec // G204: fixture helper runs the literal git binary.
	out, err := exec.CommandContext(t.Context(), "git", "init", "-q", "--bare", remote).CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	jjRun(t, dir, "git", "init", ".")
	jjRun(t, dir, "git", "remote", "add", "origin", remote)

	for i, name := range []string{"base", "bottom", "middle", "top"} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		jjRun(t, dir, "commit", "-m", name)

		if i == 0 {
			jjRun(t, dir, "bookmark", "create", "main", "-r", "@-")
			jjRun(t, dir, "git", "push", "--bookmark", "main")
		}
	}

	return dir
}

// The whole loop through the keys: I, P, -d, Enter. The forge gets one pull request per change,
// each on the one below, every gh call lands in the trail, and the outline shows the numbers.
func TestSubmit_LiveOpensStackedPullRequests(t *testing.T) {
	t.Parallel()

	dir := remoteRepo(t)
	forge := ghtest.New(t)

	opts := testOptions(t)
	opts.Ticker = instantTicks
	opts.GH = []gh.Option{gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env())}

	m := New(t.Context(), jj.NewClient(dir), ui.New(theme.Macchiato()), dir, "all()", opts)
	m = asModel(t, update(m, tea.WindowSizeMsg{Width: 100, Height: 30}))
	m = pump(t, m, m.loadLog("all()"))

	if !m.log.Focus(changeFor(t, dir, "top")) {
		t.Fatal("no top")
	}

	m = press(t, m, "I", "P")

	if !m.submit.active || m.submit.view.Plan() == nil {
		t.Fatalf("I P opened no plan: %q, err %v", m.message, m.submit.view.Err())
	}

	if got := len(m.submit.view.Plan().Steps); got != 3 {
		t.Fatalf("the plan has %d steps, want 3", got)
	}

	m = press(t, m, "-", "d", "enter")

	if m.submit.active {
		t.Fatalf("the plan is still up: %v", m.submit.view.Err())
	}

	assertSubmitted(t, m, forge)

	if !strings.Contains(m.message, "submitted #1 #2 #3") {
		t.Errorf("said %q", m.message)
	}

	if got := m.outline.view.Chapters()[0].Forge; got != "#1 · open" {
		t.Errorf("the bottom chapter shows %q, want its pull request", got)
	}

	m = press(t, m, "P")

	if p := m.submit.view.Plan(); p == nil || p.Writes() {
		t.Errorf("a second P right after plans writes: %v", commandLines(m))
	}
}

// Opening the outline asks the forge for each chapter's pull request behind the first frame, under
// the name a submit would use, so the column says what GitHub holds before any plan; a second opening
// within the TTL asks nothing.
func TestSubmit_OutlineAsksTheForgeAsItOpens(t *testing.T) {
	t.Parallel()

	dir := remoteRepo(t)
	forge := ghtest.New(t)

	bottom := changeFor(t, dir, "bottom")
	name := strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "-r", bottom, "-T", `"push-" ++ change_id.short()`))
	forge.SetPRs(t, []ghtest.PR{
		{Number: 5, State: gh.StateOpen, IsDraft: true, HeadRefName: name, BaseRefName: "main"},
	})

	opts := testOptions(t)
	opts.Ticker = instantTicks
	opts.GH = []gh.Option{gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env())}

	m := New(t.Context(), jj.NewClient(dir), ui.New(theme.Macchiato()), dir, "all()", opts)
	m = asModel(t, update(m, tea.WindowSizeMsg{Width: 100, Height: 30}))
	m = pump(t, m, m.loadLog("all()"))
	m.log.Focus(changeFor(t, dir, "top"))

	m = press(t, m, "I")

	if got := m.outline.view.Chapters()[0].Forge; got != "#5 · draft" {
		t.Errorf("the bottom chapter shows %q before any plan, want its draft pull request", got)
	}

	asked := len(forge.Calls(t))
	if asked == 0 {
		t.Fatal("opening the outline asked the forge nothing")
	}

	m = press(t, m, "esc", "I")

	if again := len(forge.Calls(t)); again != asked {
		t.Errorf("a second opening within the TTL asked %d more times", again-asked)
	}

	if !strings.Contains(tuitest.StripANSI(m.render()), "#5") {
		t.Error("the outline does not draw the pull request\n" + tuitest.StripANSI(m.render()))
	}
}
