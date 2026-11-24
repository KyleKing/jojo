package jj_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/rounds"
)

// stackRepo is a three-change stack on a base, where the middle change was amended, the top one was
// rebased onto the amended middle by that amendment, and the bottom was described anew:
//
//	top -> middle -> bottom -> base
func stackRepo(t *testing.T) (*jj.Client, string) {
	t.Helper()

	client, dir := testRepo(t)

	writeFile(t, dir, "base.txt", "base\n")
	runJJ(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "bottom.txt", "bottom\n")
	runJJ(t, dir, "commit", "-m", "bottom")
	writeFile(t, dir, "middle.txt", "middle\n")
	runJJ(t, dir, "commit", "-m", "middle")
	writeFile(t, dir, "top.txt", "top\n")
	runJJ(t, dir, "describe", "-m", "top")

	// Amend the middle change: the edit is snapshotted into it, and top is rebased on top of it.
	runJJ(t, dir, "edit", "subject(exact:middle)")
	writeFile(t, dir, "middle.txt", "middle, amended\n")
	runJJ(t, dir, "new", "subject(exact:top)")

	// Rebase top off and back on, two versions that change nothing in its own patch.
	runJJ(t, dir, "rebase", "-r", "subject(exact:top)", "-o", "subject(exact:bottom)")
	runJJ(t, dir, "rebase", "-r", "subject(exact:top)", "-o", "subject(exact:middle)")

	runJJ(t, dir, "describe", "-r", "subject(exact:bottom)", "-m", "bottom, reworded")

	return client, dir
}

func commitOf(t *testing.T, dir, revset string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", "log", "--no-graph", "--ignore-working-copy",
		"-r", revset, "-T", "commit_id")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jj log %s: %v", revset, err)
	}

	return strings.TrimSpace(string(out))
}

func TestVersions_AmendedMiddleChangeHasAContentRound(t *testing.T) {
	t.Parallel()

	client, dir := stackRepo(t)

	versions, err := client.Versions(t.Context(), "subject(exact:middle)")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	list := rounds.Group(versions)
	labels := make([]string, len(list))

	for i, r := range list {
		labels[i] = r.Label()
	}

	var amended *rounds.Round

	for i := range list {
		if list[i].Verb == "snapshot" && list[i].Files == 1 {
			amended = &list[i]
		}
	}

	if amended == nil {
		t.Fatalf("no snapshot round carrying the amendment in %v", labels)
	}

	text, err := client.Interdiff(t.Context(), amended.From, amended.To)
	if err != nil {
		t.Fatalf("Interdiff: %v", err)
	}

	if !strings.Contains(text, "-middle") || !strings.Contains(text, "+middle, amended") {
		t.Errorf("the amendment's interdiff does not show the edit:\n%s", text)
	}

	if last := versions[len(versions)-1].CommitID; last != commitOf(t, dir, "subject(exact:middle)") {
		t.Errorf("newest version %s is not the change's current commit", last)
	}
}

// The top change was rebased three times (by the amendment below it and by two explicit rebases)
// without its own patch changing, so those versions fold into one round that says so, and its
// interdiff is empty.
func TestVersions_RebasesWithoutContentChangeFoldIntoOneRound(t *testing.T) {
	t.Parallel()

	client, _ := stackRepo(t)

	versions, err := client.Versions(t.Context(), "subject(exact:top)")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	list := rounds.Group(versions)
	last := list[len(list)-1]

	if !last.Moved || last.Label() != rounds.Moved {
		t.Fatalf("newest round = %+v (%s), want the folded rebases", last, last.Label())
	}

	if last.Last-last.First < 2 {
		t.Errorf("folded %d versions, want the three that moved top", last.Last-last.First+1)
	}

	if !strings.HasPrefix(last.Banner(len(versions)), "versions ") {
		t.Errorf("banner %q does not name a span of versions", last.Banner(len(versions)))
	}

	text, err := client.Interdiff(t.Context(), last.From, last.To)
	if err != nil {
		t.Fatalf("Interdiff: %v", err)
	}

	if strings.TrimSpace(text) != "" {
		t.Errorf("a pure rebase has an interdiff:\n%s", text)
	}

	for _, v := range versions {
		if v.ChangeID != versions[0].ChangeID {
			t.Errorf("version %s belongs to another change", v.CommitID)
		}
	}
}

func TestVersions_ADescribeIsARoundOfItsOwn(t *testing.T) {
	t.Parallel()

	client, _ := stackRepo(t)

	versions, err := client.Versions(t.Context(), "subject(exact:'bottom, reworded')")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	list := rounds.Group(versions)
	last := list[len(list)-1]

	if last.Verb != "describe" || last.Moved || last.Label() != "describe, description only" {
		t.Errorf("newest round = %+v (%s), want a describe of the description alone", last, last.Label())
	}

	if !strings.Contains(last.Prose(), "description was: bottom") ||
		!strings.Contains(last.Prose(), "description now: bottom, reworded") {
		t.Errorf("prose %q does not show the old and new description", last.Prose())
	}

	if first := list[0]; first.From != "" || !strings.HasSuffix(first.Label(), "the first version") {
		t.Errorf("first round = %+v (%s), want the change's creation", first, first.Label())
	}
}

func TestVerbOf(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"snapshot working copy":                 "snapshot",
		"rebase commit 6b8683b0fc26":            "rebase",
		"describe commit 7693640c5172":          "describe",
		"squash commits into 1a2b3c":            "squash",
		"new empty commit":                      "new",
		"":                                      "unknown",
		"commit 8c028b507c800eb6266097a186e5a3": "commit",
	}

	for in, want := range cases {
		if got := jj.VerbOf(in); got != want {
			t.Errorf("VerbOf(%q) = %q, want %q", in, got, want)
		}
	}
}
