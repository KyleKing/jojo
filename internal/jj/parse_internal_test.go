package jj

import (
	"reflect"
	"strings"
	"testing"
)

// fields joins parts with fieldSep and a trailing newline, the shape every parse function reads
// one line of.
func fields(parts ...string) string {
	return strings.Join(parts, fieldSep) + "\n"
}

func TestParseLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		out  string
		want []LogRow
	}{
		{
			name: "graph-only connector line has no field separator",
			out:  "│\n",
			want: []LogRow{{Graph: &GraphLine{Graph: "│"}}},
		},
		{
			name: "truncated line short of the field count is dropped",
			out:  "abc" + fieldSep + "def\n",
			want: []LogRow{},
		},
		{
			name: "empty description and no bookmarks",
			out: "◉ zzzzzzzz" + fieldSep + "00000000" + fieldSep + fieldSep + fieldSep + fieldSep +
				"" + fieldSep + "a@b.com" + fieldSep + "1 hour ago" + fieldSep + "0" + fieldSep + "0" +
				fieldSep + "0" + fieldSep + "\n",
			want: []LogRow{{Revision: &Revision{
				Graph:       "◉ ",
				ChangeID:    "zzzzzzzz",
				CommitID:    "00000000",
				AuthorEmail: "a@b.com",
				Age:         "1 hour ago",
			}}},
		},
		{
			name: "flags combine immutable, conflict, empty, and working copy",
			out: "abcdefgh" + fieldSep + "12345678" + fieldSep + fieldSep + fieldSep + fieldSep +
				"ICE@" + fieldSep + "a@b.com" + fieldSep + "now" + fieldSep + "1" + fieldSep + "2" +
				fieldSep + "3" + fieldSep + "a change\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:    "abcdefgh",
				CommitID:    "12345678",
				AuthorEmail: "a@b.com",
				Age:         "now",
				Added:       1,
				Removed:     2,
				Files:       3,
				Description: "a change",
				Immutable:   true,
				Conflict:    true,
				Empty:       true,
				WorkingCopy: true,
			}}},
		},
		{
			name: "bookmarks split on comma and the same name tracked at two remotes collapses to one",
			out: "abcdefgh" + fieldSep + "12345678" + fieldSep + "main,feat" + fieldSep + "main,feat" +
				fieldSep + "main,main" + fieldSep + "" + fieldSep + "a@b.com" + fieldSep +
				"now" + fieldSep + "0" + fieldSep + "0" + fieldSep + "0" + fieldSep + "\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:        "abcdefgh",
				CommitID:        "12345678",
				AuthorEmail:     "a@b.com",
				Age:             "now",
				Bookmarks:       []string{"main", "feat"},
				LocalBookmarks:  []string{"main", "feat"},
				RemoteBookmarks: []string{"main"},
			}}},
		},
		{
			name: "a field adjacent to the separator byte in the ASCII table is not mistaken for it",
			out: "abcdefgh" + fieldSep + "12345678" + fieldSep + fieldSep + fieldSep + fieldSep +
				"" + fieldSep + "a@b.com" + fieldSep + "now" + fieldSep + "0" + fieldSep + "0" +
				fieldSep + "0" + fieldSep + "desc with \x1e and \x20 bytes\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:    "abcdefgh",
				CommitID:    "12345678",
				AuthorEmail: "a@b.com",
				Age:         "now",
				Description: "desc with \x1e and \x20 bytes",
			}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseLog(tc.out)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseLog(%q) = %#v, want %#v", tc.out, got, tc.want)
			}
		})
	}
}

func TestParseOpLog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		out  string
		want []Operation
	}{
		{
			name: "truncated line is dropped",
			out:  "abc" + fieldSep + "desc\n",
			want: []Operation{},
		},
		{
			name: "empty description",
			out:  "abc12345" + fieldSep + "" + fieldSep + "" + fieldSep + "2 minutes ago\n",
			want: []Operation{{ID: "abc12345", Age: "2 minutes ago"}},
		},
		{
			name: "current operation marker and a long hex id shortened mid-description",
			out: "abc12345" + fieldSep + "snapshot working copy abcdef0123456789abcdef0123456789" +
				fieldSep + "@" + fieldSep + "now\n",
			want: []Operation{{
				ID:          "abc12345",
				Description: "snapshot working copy abcdef01",
				Current:     true,
				Age:         "now",
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseOpLog(tc.out)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseOpLog(%q) = %#v, want %#v", tc.out, got, tc.want)
			}
		})
	}
}

func TestParseBookmarks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		out  string
		want []Bookmark
	}{
		{
			name: "the colocated git remote is filtered out",
			out:  fields("main", "git", "abcd1234", "", "", "", "", ""),
			want: []Bookmark{},
		},
		{
			name: "untracked bookmark leaves ahead and behind at zero",
			out:  fields("main", "", "abcd1234", "", "", "", "", "first"),
			want: []Bookmark{{Name: "main", ChangeID: "abcd1234", Description: "first"}},
		},
		{
			name: "tracked, diverged, and conflicted",
			out:  fields("main", "origin", "abcd1234", "T", "2", "3", "C", "third"),
			want: []Bookmark{{
				Name: "main", Remote: "origin", ChangeID: "abcd1234",
				Tracked: true, Ahead: 2, Behind: 3, Conflict: true, Description: "third",
			}},
		},
		{
			name: "a bookmark with no target renders an empty change id",
			out:  fields("orphan", "", "", "", "", "", "", ""),
			want: []Bookmark{{Name: "orphan"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseBookmarks(tc.out)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseBookmarks(%q) = %#v, want %#v", tc.out, got, tc.want)
			}
		})
	}
}

func TestParseWorkspaces(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		out  string
		root string
		want []Workspace
	}{
		{
			name: "root matches the client's own workspace",
			out:  "default" + fieldSep + "abcd1234" + fieldSep + "12345678" + fieldSep + "desc" + fieldSep + "/repo\n",
			root: "/repo",
			want: []Workspace{{
				Name: "default", ChangeID: "abcd1234", CommitID: "12345678",
				Description: "desc", Root: "/repo", Current: true,
			}},
		},
		{
			name: "a moved workspace renders an empty root and is never current",
			out:  "old" + fieldSep + "abcd1234" + fieldSep + "12345678" + fieldSep + "desc" + fieldSep + "\n",
			root: "",
			want: []Workspace{{Name: "old", ChangeID: "abcd1234", CommitID: "12345678", Description: "desc"}},
		},
		{
			name: "truncated line is dropped",
			out:  "default" + fieldSep + "abcd1234\n",
			root: "/repo",
			want: []Workspace{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseWorkspaces(tc.out, tc.root)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseWorkspaces(%q, %q) = %#v, want %#v", tc.out, tc.root, got, tc.want)
			}
		})
	}
}

func TestParseEvolog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		out  string
		want []Evolution
	}{
		{
			name: "empty description on a non-empty commit",
			out:  "abcd1234" + fieldSep + "" + fieldSep + "5 minutes ago" + fieldSep + "describe" + fieldSep + "\n",
			want: []Evolution{{CommitID: "abcd1234", Age: "5 minutes ago", Operation: "describe"}},
		},
		{
			name: "empty commit flag set",
			out:  "abcd1234" + fieldSep + "wip" + fieldSep + "now" + fieldSep + "new" + fieldSep + "E\n",
			want: []Evolution{{CommitID: "abcd1234", Description: "wip", Age: "now", Operation: "new", Empty: true}},
		},
		{
			name: "truncated line is dropped",
			out:  "abcd1234" + fieldSep + "wip\n",
			want: []Evolution{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseEvolog(tc.out)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseEvolog(%q) = %#v, want %#v", tc.out, got, tc.want)
			}
		})
	}
}
