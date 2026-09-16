package model

import (
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

func rev(id string, opts ...func(*jj.Revision)) *jj.Revision {
	r := &jj.Revision{ChangeID: id}
	for _, opt := range opts {
		opt(r)
	}

	return r
}

func local(names ...string) func(*jj.Revision) {
	return func(r *jj.Revision) { r.LocalBookmarks = names }
}

func remote(names ...string) func(*jj.Revision) {
	return func(r *jj.Revision) { r.RemoteBookmarks = names }
}

func atWorkingCopy(r *jj.Revision) { r.WorkingCopy = true }

func TestBranchName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rev  *jj.Revision
		name string
		want string
	}{
		{name: "no revision", rev: nil, want: ""},
		{name: "local wins", rev: rev("a", local("main"), remote("other")), want: "main"},
		{name: "remote only still counts", rev: rev("b", remote("feature")), want: "feature"},
		{name: "no bookmark at all", rev: rev("c"), want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := branchName(tc.rev); got != tc.want {
				t.Errorf("branchName = %q, want %q", got, tc.want)
			}
		})
	}
}

// The forge watches the bookmark on @ when there is one, because jj has no
// current branch to fall back on.
func TestBookmarkNearWorkingCopy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		rows []jj.LogRow
	}{
		{
			name: "bookmark on the working copy wins over an earlier one",
			want: "mine",
			rows: []jj.LogRow{
				{Revision: rev("a", local("older"))},
				{Revision: rev("b", local("mine"), atWorkingCopy)},
			},
		},
		{
			name: "nearest bookmark stands in when the working copy has none",
			want: "older",
			rows: []jj.LogRow{
				{Revision: rev("a", atWorkingCopy)},
				{Graph: &jj.GraphLine{Graph: "│"}},
				{Revision: rev("b", local("older"))},
			},
		},
		{name: "no bookmarks anywhere", want: "", rows: []jj.LogRow{{Revision: rev("a")}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := bookmarkNearWorkingCopy(tc.rows); got != tc.want {
				t.Errorf("bookmarkNearWorkingCopy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJoinIDs(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"abc":            {"abc"},
		"abc and 1 more": {"abc", "def"},
		"abc and 2 more": {"abc", "def", "ghi"},
	}

	for want, ids := range cases {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if got := joinIDs(ids); got != want {
				t.Errorf("joinIDs(%v) = %q, want %q", ids, got, want)
			}
		})
	}
}

// The status line reports the jj invocation rather than a description of the edit, which is how the
// TUI teaches the CLI it drives, and a refusal becomes the offer of the escape hatch.
func TestWroteReportsTheCommandAndTheWayBack(t *testing.T) {
	t.Parallel()

	const ran = "jj abandon abc"

	cases := []struct {
		err   error
		name  string
		want  string
		offer bool
	}{
		{name: "success echoes the command", err: nil, want: ran + undoHint},
		{name: "no-op says so", err: jj.ErrNothingChanged, want: ran + ": nothing changed"},
		{name: "other failures report themselves", err: jj.ErrCommandFailed, want: jj.ErrCommandFailed.Error()},
		{name: "immutable offers the escape", err: jj.ErrImmutable, offer: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := wrote(ran, undoHint, tc.err)

			if tc.offer {
				if _, ok := got.(refusedMsg); !ok {
					t.Fatalf("wrote returned %T, want the escape hatch offer", got)
				}

				return
			}

			msg, ok := got.(messageMsg)
			if !ok {
				t.Fatalf("wrote returned %T, want a message", got)
			}

			if msg.text != tc.want {
				t.Errorf("message = %q, want %q", msg.text, tc.want)
			}
		})
	}
}

// Tab has to skip the forge pane while it is closed, or a third of the cycle
// focuses something nobody can see.
func TestCycleFocusSkipsTheClosedForgePane(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		start focus
		want  focus
		open  bool
	}{
		{name: "log to revset", start: focusLog, want: focusRevset},
		{name: "revset back to log when the forge is closed", start: focusRevset, want: focusLog},
		{name: "revset to forge when it is open", start: focusRevset, want: focusForge, open: true},
		{name: "forge back to log", start: focusForge, want: focusLog, open: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()),
				t.TempDir(), "@", testOptions(t))
			m.focus, m.showForge = tc.start, tc.open

			next, _ := m.cycleFocus()

			got, ok := next.(Model)
			if !ok {
				t.Fatalf("cycleFocus returned %T, want Model", next)
			}

			if got.focus != tc.want {
				t.Errorf("cycleFocus from %v = %v, want %v", tc.start, got.focus, tc.want)
			}
		})
	}
}
