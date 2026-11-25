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
			out: "◉ zzzzzzzz" + fieldSep + fieldSep + fieldSep + fieldSep +
				"" + fieldSep + "a@b.com" + fieldSep + "1 hour ago" + fieldSep + "000000000000" + fieldSep +
				"zzzzzzzzzzzz" + fieldSep + fieldSep + "\n",
			want: []LogRow{{Revision: &Revision{
				Graph:        "◉ ",
				ChangeID:     "zzzzzzzz",
				FullChangeID: "zzzzzzzzzzzz",
				CommitID:     "000000000000",
				AuthorEmail:  "a@b.com",
				Age:          "1 hour ago",
			}}},
		},
		{
			name: "flags combine immutable, conflict, empty, working copy, and trunk, and parents split on comma",
			out: "abcdefgh" + fieldSep + fieldSep + fieldSep + fieldSep +
				"ICE@T" + fieldSep + "a@b.com" + fieldSep + "now" + fieldSep + "c0ffee" + fieldSep +
				"abcdefghij" + fieldSep + "beef00000001,beef00000002" + fieldSep + "a change\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:     "abcdefgh",
				FullChangeID: "abcdefghij",
				CommitID:     "c0ffee",
				AuthorEmail:  "a@b.com",
				Age:          "now",
				Description:  "a change",
				Immutable:    true,
				Conflict:     true,
				Empty:        true,
				WorkingCopy:  true,
				Trunk:        true,
				Parents:      []string{"beef00000001", "beef00000002"},
			}}},
		},
		{
			name: "bookmarks split on comma and the same name tracked at two remotes collapses to one",
			out: "abcdefgh" + fieldSep + "main,feat" + fieldSep + "main,feat" +
				fieldSep + "main,main" + fieldSep + "" + fieldSep + "a@b.com" + fieldSep +
				"now" + fieldSep + "c0ffee" + fieldSep + "abcdefghij" + fieldSep + fieldSep + "\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:        "abcdefgh",
				FullChangeID:    "abcdefghij",
				CommitID:        "c0ffee",
				AuthorEmail:     "a@b.com",
				Age:             "now",
				Bookmarks:       []string{"main", "feat"},
				LocalBookmarks:  []string{"main", "feat"},
				RemoteBookmarks: []string{"main"},
			}}},
		},
		{
			name: "a field adjacent to the separator byte in the ASCII table is not mistaken for it",
			out: "abcdefgh" + fieldSep + fieldSep + fieldSep + fieldSep +
				"" + fieldSep + "a@b.com" + fieldSep + "now" + fieldSep + "c0ffee" + fieldSep +
				"abcdefghij" + fieldSep + fieldSep + "desc with \x1e and \x20 bytes\n",
			want: []LogRow{{Revision: &Revision{
				ChangeID:     "abcdefgh",
				FullChangeID: "abcdefghij",
				CommitID:     "c0ffee",
				AuthorEmail:  "a@b.com",
				Age:          "now",
				Description:  "desc with \x1e and \x20 bytes",
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

// Stats arrive after the graph, keyed by commit id, so a row whose read is still out shows nothing
// rather than a wrong count, and a malformed line is skipped rather than zeroing a row.
func TestParseStats(t *testing.T) {
	t.Parallel()

	out := fields("c0ffee", "1", "2", "3") + "garbage\n" + fields("beef01", "0", "0", "0")

	want := map[string]Stats{
		"c0ffee": {Added: 1, Removed: 2, Files: 3},
		"beef01": {},
	}

	if got := parseStats(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseStats = %#v, want %#v", got, want)
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
			out:  "abc12345" + fieldSep + "" + fieldSep + "" + fieldSep + "2 minutes ago" + fieldSep + "\n",
			want: []Operation{{ID: "abc12345", Age: "2 minutes ago"}},
		},
		{
			name: "current operation marker and a long hex id shortened mid-description",
			out: "abc12345" + fieldSep + "snapshot working copy abcdef0123456789abcdef0123456789" +
				fieldSep + "@" + fieldSep + "now" + fieldSep + "\n",
			want: []Operation{{
				ID:          "abc12345",
				Description: "snapshot working copy abcdef01",
				Current:     true,
				Age:         "now",
			}},
		},
		{
			name: "the command line loses its args label and its long ids",
			out: "abc12345" + fieldSep + "rebase commit 1" + fieldSep + "" + fieldSep + "now" + fieldSep +
				"args: jj rebase -r abcdef0123456789abcdef -o xyz\n",
			want: []Operation{{
				ID:          "abc12345",
				Description: "rebase commit 1",
				Age:         "now",
				Args:        "jj rebase -r abcdef01 -o xyz",
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
			out:  "default" + fieldSep + "abcd1234" + fieldSep + "desc" + fieldSep + "/repo\n",
			root: "/repo",
			want: []Workspace{{
				Name: "default", ChangeID: "abcd1234",
				Description: "desc", Root: "/repo", Current: true,
			}},
		},
		{
			name: "a moved workspace renders an empty root and is never current",
			out:  "old" + fieldSep + "abcd1234" + fieldSep + "desc" + fieldSep + "\n",
			root: "",
			want: []Workspace{{Name: "old", ChangeID: "abcd1234", Description: "desc"}},
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

// An op log row reads as the command a person would type: the flags jojo adds to parse jj's output
// go, and a template, which can hold spaces inside its quotes, goes whole.
func TestReadableArgs_DropsJojosPlumbing(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"jj --no-pager '--color=never' diff -r abc --git":                              "jj diff -r abc --git",
		"jj --no-pager --color=never --ignore-working-copy log -T 'a ++ \"b c\"' -r @": "jj log -r @",
		"jj squash --into xyz":         "jj squash --into xyz",
		"jj describe -m 'two words'":   "jj describe -m 'two words'",
		"jj log '--template=x y' -n 1": "jj log -n 1",
	}

	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			if got := readableArgs(in); got != want {
				t.Errorf("readableArgs(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

func TestUndoTarget_StepsBackPastEachUndo(t *testing.T) {
	t.Parallel()

	op := func(d string) Operation { return Operation{Description: d} }
	cases := []struct {
		name string
		want string
		ops  []Operation
	}{
		{name: "the newest operation", want: "squash", ops: []Operation{op("squash"), op("describe")}},
		{
			name: "one undo steps past what it undid", want: "describe",
			ops: []Operation{op("undo: restore to operation 1"), op("squash"), op("describe"), op("new")},
		},
		{
			name: "two undos step two back", want: "new",
			ops: []Operation{
				op("undo: restore to operation 2"), op("undo: restore to operation 1"), op("squash"),
				op("describe"), op("new"),
			},
		},
		{name: "nothing left", want: "", ops: []Operation{op("undo: restore to operation 1"), op("squash")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := UndoTarget(tc.ops)
			if got.Description != tc.want || ok != (tc.want != "") {
				t.Errorf("UndoTarget = %q, %v; want %q", got.Description, ok, tc.want)
			}
		})
	}
}

func TestRefusal_SnapshotAndPush(t *testing.T) {
	t.Parallel()

	for desc, refused := range map[string]bool{
		"snapshot working copy":                   true,
		"push bookmark main to git remote origin": true,
		"squash commits into 1234":                false,
		"describe commit 1234":                    false,
	} {
		if got := (Operation{Description: desc}).Refusal() != ""; got != refused {
			t.Errorf("Refusal(%q) refused = %v, want %v", desc, got, refused)
		}
	}
}

// A bookmark is the trunk's when trunk() carries its name, even after the local one has moved ahead
// of the remote onto a mutable commit, or when it sits on an immutable commit. Only the from-revset's
// commits are candidates; trunk() rides along to name the trunk.
func TestParseAdvances_NamesTrunkByTrunksBookmarks(t *testing.T) {
	t.Parallel()

	out := fields("mainlocl", "main", "", "F") +
		fields("featfeat", "feat,wip", "", "F") +
		fields("oldoldol", "release", "release", "IF") +
		fields("trunktrk", "", "main", "IT")

	want := []Advance{
		{Name: "main", From: "mainlocl", Trunk: true},
		{Name: "feat", From: "featfeat"},
		{Name: "wip", From: "featfeat"},
		{Name: "release", From: "oldoldol", Trunk: true},
	}

	if got := parseAdvances(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseAdvances = %+v, want %+v", got, want)
	}
}
