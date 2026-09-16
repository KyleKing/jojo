package jj

import "testing"

func TestRebaseScopeFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want  string
		scope RebaseScope
	}{
		{scope: RebaseScopeRevisions, want: "-r"},
		{scope: RebaseScopeSource, want: "-s"},
		{scope: RebaseScopeBranch, want: "-b"},
		{scope: RebaseScope(99), want: "-r"},
	}

	for _, tc := range cases {
		if got := tc.scope.flag(); got != tc.want {
			t.Errorf("RebaseScope(%d).flag() = %q, want %q", tc.scope, got, tc.want)
		}
	}
}

func TestRebaseDestinationModeFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want string
		mode RebaseDestinationMode
	}{
		{mode: RebaseDestOnto, want: "--onto"},
		{mode: RebaseDestInsertAfter, want: "--insert-after"},
		{mode: RebaseDestInsertBefore, want: "--insert-before"},
		{mode: RebaseDestinationMode(99), want: "--onto"},
	}

	for _, tc := range cases {
		if got := tc.mode.flag(); got != tc.want {
			t.Errorf("RebaseDestinationMode(%d).flag() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}
