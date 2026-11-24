package model

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

var errNotOnPath = errors.New("not on PATH")

func installed(names ...string) lookPath {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/usr/bin/" + name, nil
		}

		return "", errNotOnPath
	}
}

func jjConfig(set bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return set, nil }
}

// Hunk work went to whatever ui.diff-editor was, so an unconfigured user got jj's builtin editor
// even with jj-diff installed beside it. The sibling is named only where nothing else was chosen.
func TestResolveDiffTool_NamesJJDiffOnlyWhenNothingElseWasChosen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		find       lookPath
		name       string
		preference string
		want       string
		configured bool
	}{
		{name: "installed and jj unconfigured", find: installed(jjDiff), want: jjDiff},
		{name: "auto spelled out", preference: diffToolAuto, find: installed(jjDiff), want: jjDiff},
		{name: "not installed", find: installed(), want: ""},
		{name: "jj's ui.diff-editor wins", find: installed(jjDiff), configured: true, want: ""},
		{name: "none leaves it to jj", preference: diffToolNone, find: installed(jjDiff), want: ""},
		{
			name: "a named tool wins over everything", preference: "meld", find: installed(),
			configured: true, want: "meld",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveDiffTool(t.Context(), tc.preference, jjConfig(tc.configured), tc.find)
			if got != tc.want {
				t.Errorf("resolveDiffTool = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHunkArgs_NameTheToolOnEveryHunkVerb(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), diffToolMsg{tool: jjDiff}))

	for _, args := range [][]string{
		m.hunkArgs("split", "-i", "-r", "aaaa"),
		m.hunkArgs("squash", "-i", "-r", "aaaa"),
		m.squashHunkArgs("aaaa", "bbbb"),
	} {
		if got := strings.Join(args, " "); !strings.HasSuffix(got, "--tool jj-diff") {
			t.Errorf("%q does not name jj-diff", got)
		}
	}

	plain := testModel(t, twoRows())
	if got := strings.Join(plain.hunkArgs("split", "-i"), " "); got != "split -i" {
		t.Errorf("with no tool resolved the args are %q", got)
	}
}
