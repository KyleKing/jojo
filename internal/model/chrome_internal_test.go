package model

import (
	"testing"

	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
)

// The coach's next step follows the what-next table: what is wrong first (a conflict, divergence, an
// immutable revision, a bookmark that moved both ways), then the natural next step on @ (describe
// it, then start the next), then what is worth knowing. The cursor's row is the first one.
func TestNextStep_FollowsTheWhatNextTable(t *testing.T) {
	t.Parallel()

	wc := &jj.Revision{ChangeID: "wwww", WorkingCopy: true, Description: "wip", Files: 1}
	dirtyWC := &jj.Revision{ChangeID: "wwww", WorkingCopy: true, Files: 1}

	cases := []struct {
		name   string
		want   string
		key    string
		cursor *jj.Revision
		others []*jj.Revision
	}{
		{
			name:   "conflict outranks immutable",
			cursor: &jj.Revision{ChangeID: "cccc", Conflict: true, Immutable: true},
			want:   help.ActionResolve, key: "x",
		},
		{
			name: "divergent", cursor: &jj.Revision{ChangeID: "dddd", Divergent: true},
			want: help.ActionConverge, key: "X",
		},
		{
			name: "immutable starts a change on top", cursor: &jj.Revision{ChangeID: "iiii", Immutable: true},
			want: help.ActionNew, key: "n",
		},
		{
			name: "bookmark moved both ways", cursor: &jj.Revision{ChangeID: "bbbb", Bookmarks: []string{"main??"}},
			want: help.ActionBookmarkResolve, key: "bx",
		},
		{name: "@ dirty and undescribed", cursor: dirtyWC, want: keys.ActionDescribe, key: "d"},
		{
			name: "@ dirty, cursor elsewhere", cursor: &jj.Revision{ChangeID: "oooo", Description: "x"},
			others: []*jj.Revision{dirtyWC}, want: help.ActionWorkingCopy, key: "@",
		},
		{name: "@ described", cursor: wc, want: help.ActionNew, key: "n"},
		{
			name:   "bookmark ahead",
			cursor: &jj.Revision{ChangeID: "pppp", Description: "x", Bookmarks: []string{"feat*"}},
			others: []*jj.Revision{wc}, want: help.ActionPush, key: "P",
		},
		{
			name: "empty and undescribed", cursor: &jj.Revision{ChangeID: "eeee", Empty: true},
			others: []*jj.Revision{wc}, want: help.ActionAbandon, key: "a",
		},
		{
			name:   "the working copy is empty on purpose",
			cursor: &jj.Revision{ChangeID: "wwww", Empty: true, WorkingCopy: true},
		},
		{
			name: "ordinary revision", cursor: &jj.Revision{ChangeID: "abcd", Description: "x"},
			others: []*jj.Revision{wc},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rows := []jj.LogRow{{Revision: tc.cursor}}
			for _, r := range tc.others {
				rows = append(rows, jj.LogRow{Revision: r})
			}

			next, ok := testModel(t, rows).nextStep()
			if tc.want == "" {
				if ok {
					t.Fatalf("nextStep = %+v, want nothing to say", next)
				}

				return
			}

			if !ok || next.Action != tc.want {
				t.Fatalf("nextStep = %+v (%v), want %s", next, ok, tc.want)
			}

			if tc.key != "" && next.Key != tc.key {
				t.Errorf("key = %q, want %q", next.Key, tc.key)
			}
		})
	}
}
