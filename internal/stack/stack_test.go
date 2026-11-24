package stack_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/gh/ghtest"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/stack"
)

// fixture is a jj repository with a bare git remote holding main, three changes stacked on it (a, b,
// and c, parent first), the empty working copy jj leaves on top, and a fake forge.
type fixture struct {
	client *jj.Client
	forge  ghtest.Forge
	cli    *gh.CLI
	dir    string
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping")
	}

	dir, remote := t.TempDir(), filepath.Join(t.TempDir(), "remote.git")
	run(t, "", "git", "init", "-q", "--bare", remote)
	run(t, dir, "jj", "git", "init", ".")
	run(t, dir, "jj", "config", "set", "--repo", "user.name", "Test User")
	run(t, dir, "jj", "config", "set", "--repo", "user.email", "test@example.com")
	run(t, dir, "jj", "git", "remote", "add", "origin", remote)

	write(t, dir, "base.txt", "base\n")
	run(t, dir, "jj", "commit", "-m", "base")
	run(t, dir, "jj", "bookmark", "create", "main", "-r", "@-")
	run(t, dir, "jj", "git", "push", "--bookmark", "main")

	for _, name := range []string{"a", "b", "c"} {
		write(t, dir, name+".txt", name+"\n")
		run(t, dir, "jj", "commit", "-m", "feat: "+name+"\n\nwhy "+name+" matters")
	}

	forge := ghtest.New(t)

	return fixture{
		dir: dir, forge: forge, client: jj.NewClient(dir),
		cli: gh.New(dir, nil, gh.WithBinary(forge.Bin), gh.WithEnv(forge.Env())),
	}
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs literal binaries with test arguments.
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Dir = dir

	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, stderr.String())
	}

	return string(out)
}

func write(t *testing.T, dir, name, text string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// revs is the stack under @ the way the outline reads it, parent first.
func (f fixture) revs(t *testing.T) []jj.Revision {
	t.Helper()

	rows, err := f.client.Log(t.Context(), "trunk()..@", 50)
	if err != nil {
		t.Fatal(err)
	}

	var out []jj.Revision

	for _, r := range rows {
		if r.Revision != nil {
			out = append(out, *r.Revision)
		}
	}

	slices.Reverse(out)

	return out
}

func (f fixture) plan(t *testing.T) stack.Plan {
	t.Helper()

	in, err := stack.Read(t.Context(), f.revs(t), f.client, f.cli, "", true)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if why := stack.Refusals(in.Changes); len(why) > 0 {
		t.Fatalf("refused: %v", why)
	}

	return stack.Build(in)
}

func (f fixture) submit(t *testing.T) (stack.Plan, stack.Result) {
	t.Helper()

	plan := f.plan(t)
	f.forge.Reset(t)

	res, err := stack.Run(t.Context(), plan, f.client.Apply, f.cli)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	return plan, res
}

func (f fixture) changeOf(t *testing.T, subject string) string {
	t.Helper()

	return strings.TrimSpace(run(t, f.dir, "jj", "log", "--no-graph", "-r",
		"description(substring:'"+subject+"')", "-T", "change_id.short(12)"))
}

func lines(plan stack.Plan) []string {
	out := make([]string, 0, len(plan.Commands))
	for _, c := range plan.Commands {
		out = append(out, c.Line())
	}

	return out
}

func actions(plan stack.Plan) string {
	out := make([]string, 0, len(plan.Steps))
	for k := range plan.Steps {
		s := &plan.Steps[k]
		out = append(out, s.Change.Subject+"="+s.Action.String()+"→"+s.Base)
	}

	return strings.Join(out, ", ")
}

func TestSubmit_FirstOpensOnePullRequestPerChangeEachOnTheOneBelow(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	a, b, c := f.changeOf(t, "feat: a"), f.changeOf(t, "feat: b"), f.changeOf(t, "feat: c")

	plan, res := f.submit(t)

	want := []string{
		"jj bookmark create push-" + a + " -r " + a,
		"jj bookmark create push-" + b + " -r " + b,
		"jj bookmark create push-" + c + " -r " + c,
		"jj git push --bookmark push-" + a + " --bookmark push-" + b + " --bookmark push-" + c,
		"gh pr create --head push-" + a + " --base main --title 'feat: a' --body-file - --draft <<< body",
		"gh pr create --head push-" + b + " --base push-" + a + " --title 'feat: b' --body-file - --draft <<< body",
		"gh pr create --head push-" + c + " --base push-" + b + " --title 'feat: c' --body-file - --draft <<< body",
		"gh pr edit push-" + a + " --body-file - <<< body",
		"gh pr edit push-" + b + " --body-file - <<< body",
		"gh pr edit push-" + c + " --body-file - <<< body",
	}
	if got := lines(plan); !slices.Equal(got, want) {
		t.Fatalf("plan:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if len(plan.Skipped) != 1 || !plan.Skipped[0].WorkingCopy {
		t.Errorf("skipped %v, want the empty working copy on top", plan.Skipped)
	}

	if len(res.Opened) != 3 {
		t.Fatalf("opened %v, want three", res.Opened)
	}

	// The calls the forge saw are the ones the plan listed, after its reads.
	var gotWrites []string
	for _, l := range lines(plan) {
		if strings.HasPrefix(l, "gh ") {
			gotWrites = append(gotWrites, strings.TrimSuffix(l, " <<< body"))
		}
	}

	if got := f.forge.Writes(t); !slices.Equal(got, gotWrites) {
		t.Errorf("forge saw:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(gotWrites, "\n"))
	}

	pb := f.forge.For(t, "push-"+b)
	if !pb.IsDraft || pb.BaseRefName != "push-"+a {
		t.Errorf("b's PR is draft=%v on %q, want a draft on a's bookmark", pb.IsDraft, pb.BaseRefName)
	}

	for _, want := range []string{
		"why b matters", stack.SectionStart, "#3 feat: c", "**#2 feat: b** ← this pull request", "#1 feat: a", "`main`",
	} {
		if !strings.Contains(pb.Body, want) {
			t.Errorf("b's body lacks %q:\n%s", want, pb.Body)
		}
	}

	if out := run(t, f.dir, "jj", "bookmark", "list", "--tracked", "push-"+c); !strings.Contains(out, "@origin") {
		t.Errorf("c's bookmark is not tracked on origin:\n%s", out)
	}
}

func TestSubmit_AgainAfterAnAmendOnlyPushes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.submit(t)

	// A reviewer's note: edit a, which jj rebases b and c onto.
	b := f.changeOf(t, "feat: b")
	run(t, f.dir, "jj", "edit", f.changeOf(t, "feat: a"))
	write(t, f.dir, "a.txt", "a, amended\n")
	run(t, f.dir, "jj", "new", f.changeOf(t, "feat: c"))

	// The user's own words under the section survive, and the section is not rewritten.
	prs := f.forge.PRs(t)
	prs[1].Body = "Reviewer: start with the parser.\n\n" + prs[1].Body
	f.forge.SetPRs(t, prs)

	plan, _ := f.submit(t)

	if got := actions(plan); !strings.Contains(got, "feat: a=update→main") || !strings.Contains(got, "feat: b=update") {
		t.Errorf("actions %s, want every change an update on its old base", got)
	}

	want := "jj git push --bookmark push-" + f.changeOf(t, "feat: a") + " --bookmark push-" + b +
		" --bookmark push-" + f.changeOf(t, "feat: c")
	if got := lines(plan); len(got) != 1 || got[0] != want {
		t.Errorf("plan %q, want only the push", got)
	}

	if writes := f.forge.Writes(t); len(writes) != 0 {
		t.Errorf("an amend wrote to the forge: %q", writes)
	}

	if body := f.forge.For(t, "push-"+b).Body; !strings.HasPrefix(body, "Reviewer: start with the parser.") {
		t.Errorf("the author's text was lost:\n%s", body)
	}
}

func TestSubmit_AfterAReorderRetargetsEachBase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.submit(t)

	a, b, c := f.changeOf(t, "feat: a"), f.changeOf(t, "feat: b"), f.changeOf(t, "feat: c")

	// c moves to the bottom: main, c, a, b.
	run(t, f.dir, "jj", "rebase", "-r", c, "-B", a)

	plan, _ := f.submit(t)

	if got := actions(plan); got != "feat: c=retarget→main, feat: a=retarget→push-"+c+", feat: b=update→push-"+a {
		t.Errorf("actions %s", got)
	}

	bases := map[string]string{"push-" + c: "main", "push-" + a: "push-" + c, "push-" + b: "push-" + a}
	for head, base := range bases {
		if got := f.forge.For(t, head).BaseRefName; got != base {
			t.Errorf("%s is based on %s, want %s", head, got, base)
		}
	}

	body := f.forge.For(t, "push-"+a).Body
	at := func(s string) int { return strings.Index(body, s) }
	if at("feat: b") > at("feat: a") || at("feat: a") > at("feat: c") {
		t.Errorf("the section does not read top first b, a, c:\n%s", body)
	}

	if strings.Count(body, stack.SectionStart) != 1 {
		t.Errorf("the section was added twice:\n%s", body)
	}
}

func TestSubmit_AfterTheMiddleIsAbandonedRetargetsTheOneAbove(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.submit(t)

	a, b, c := f.changeOf(t, "feat: a"), f.changeOf(t, "feat: b"), f.changeOf(t, "feat: c")
	run(t, f.dir, "jj", "abandon", b)

	plan, _ := f.submit(t)

	if got := actions(plan); got != "feat: a=up to date→main, feat: c=retarget→push-"+a {
		t.Errorf("actions %s", got)
	}

	if len(plan.Notes) != 1 || !strings.Contains(plan.Notes[0], "push-"+b+", which is no longer in the stack") {
		t.Errorf("notes %q, want one about b's pull request left open", plan.Notes)
	}

	if got := f.forge.For(t, "push-"+c).BaseRefName; got != "push-"+a {
		t.Errorf("c is based on %s, want a", got)
	}

	if strings.Contains(f.forge.For(t, "push-"+a).Body, "feat: b") {
		t.Error("a's section still lists the abandoned change")
	}

	// The remote keeps b, since deleting a base closes every pull request on it.
	out := run(t, f.dir, "git", "--git-dir", filepath.Join(f.dir, ".git"), "ls-remote", "origin")
	if !strings.Contains(out, "push-"+b) {
		t.Errorf("b's branch was deleted from the remote:\n%s", out)
	}
}

func TestSubmit_WithTheBottomMergedLeavesItAndBasesTheNextOnTrunk(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.submit(t)

	a, b := f.changeOf(t, "feat: a"), f.changeOf(t, "feat: b")
	f.forge.Merge(t, "push-"+a)

	plan, _ := f.submit(t)

	want := "feat: a=merged→main, feat: b=retarget→main, feat: c=up to date→push-" + b
	if got := actions(plan); !strings.HasPrefix(got, want) {
		t.Errorf("actions %s", got)
	}

	if len(plan.Notes) == 0 || !strings.Contains(plan.Notes[0], "#1 ("+plan.Steps[0].Change.ID+") merged") {
		t.Errorf("notes %q, want the merge named with the rebase it calls for", plan.Notes)
	}

	for _, w := range f.forge.Writes(t) {
		if strings.Contains(w, "push-"+a+" ") || strings.HasSuffix(w, "push-"+a) {
			t.Errorf("the merged pull request was written to: %s", w)
		}
	}

	if !strings.Contains(f.forge.For(t, "push-"+b).Body, "#1 feat: a (merged)") {
		t.Errorf("b's section does not show a merged:\n%s", f.forge.For(t, "push-"+b).Body)
	}
}

func TestRefusals_NameEachChangeThatBlocks(t *testing.T) {
	t.Parallel()

	changes := []stack.Change{
		{ID: "aaa", Subject: "ok"},
		{ID: "bbb", Subject: "x", Immutable: true},
		{ID: "ccc", Subject: "x", Conflict: true},
		{ID: "ddd", Subject: "x", Divergent: true},
		{ID: "eee", Subject: "x", Empty: true},
		{ID: "fff"},
	}

	got := stack.Refusals(changes)
	want := []string{
		"bbb is immutable", "ccc is conflicted", "ddd is divergent", "eee is empty", "fff has no description",
	}

	if len(got) != len(want) {
		t.Fatalf("refusals %q", got)
	}

	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("refusal %d is %q, want %q…", i, got[i], want[i])
		}
	}

	if got := stack.Refusals(nil); len(got) != 1 {
		t.Errorf("an empty stack refused with %q", got)
	}
}

func TestBookmarkName_FillsTheTemplate(t *testing.T) {
	t.Parallel()

	c := stack.Change{LongID: "qpvuntsmwlqt", Subject: "Fix: the parser's (very) long header handling, at last!"}

	for template, want := range map[string]string{
		"":                         "push-qpvuntsmwlqt",
		"kk/{subject}":             "kk/fix-the-parser-s-very-long-header-handli",
		"stack/{change}-{subject}": "stack/qpvuntsmwlqt-fix-the-parser-s-very-long-header-handli",
	} {
		if got := stack.BookmarkName(template, c); got != want {
			t.Errorf("%q gave %q, want %q", template, got, want)
		}
	}
}

func TestCompose_ReplacesOnlyBetweenTheMarkers(t *testing.T) {
	t.Parallel()

	section := stack.SectionStart + "\nnew\n" + stack.SectionEnd
	body := "intro\n\n" + stack.SectionStart + "\nold\n" + stack.SectionEnd + "\n\nfooter"

	if got := stack.Compose(body, section); got != "intro\n\n"+section+"\n\nfooter" {
		t.Errorf("Compose = %q", got)
	}

	if got := stack.Compose("", section); got != section {
		t.Errorf("Compose on an empty body = %q", got)
	}

	if got := stack.Compose("words\n\n", section); got != "words\n\n"+section {
		t.Errorf("Compose appends = %q", got)
	}
}
