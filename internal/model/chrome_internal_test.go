package model

import (
	"testing"

	"github.com/kyleking/jojo/internal/jj"
)

// The footer says one thing about the row under the cursor, because a footer that says everything
// says nothing, and the order below is what a reader most needs to know first.
func TestRowHintsSayTheOneThingThatMatters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		rev  *jj.Revision
		name string
		want string
	}{
		{name: "no row", rev: nil, want: ""},
		{
			name: "conflict outranks immutable",
			rev:  &jj.Revision{Conflict: true, Immutable: true},
			want: "conflicted, edit it to resolve",
		},
		{name: "immutable", rev: &jj.Revision{Immutable: true}, want: "immutable, an edit asks first"},
		{name: "empty", rev: &jj.Revision{Empty: true}, want: "empty, abandon it"},
		{name: "the working copy is empty on purpose", rev: &jj.Revision{Empty: true, WorkingCopy: true}, want: ""},
		{name: "bookmark", rev: &jj.Revision{LocalBookmarks: []string{"main"}}, want: "bookmarks, push from there"},
		{name: "ordinary revision", rev: &jj.Revision{ChangeID: "abc"}, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hints := rowHints(tc.rev)

			if tc.want == "" {
				if len(hints) != 0 {
					t.Fatalf("rowHints = %v, want nothing to say", hints)
				}

				return
			}

			if len(hints) != 1 {
				t.Fatalf("rowHints returned %d hints, want exactly one", len(hints))
			}

			if hints[0].What != tc.want {
				t.Errorf("hint = %q, want %q", hints[0].What, tc.want)
			}
		})
	}
}
