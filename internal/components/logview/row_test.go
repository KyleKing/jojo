package logview_test

import (
	"testing"

	"github.com/kyleking/aragonite/tui/table"

	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/jj"
)

func TestCells(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want map[string]string
		name string
		rev  jj.Revision
	}{
		{
			name: "described revision keeps its size and drops the email domain",
			rev: jj.Revision{
				ChangeID:    "nrottnuz",
				Description: "fix(help): scroll the overlay",
				AuthorEmail: "kyleking@users.noreply.github.com",
				Age:         "1 week ago",
				Bookmarks:   []string{"main", "wip"},
				Added:       203,
				Removed:     17,
				Files:       3,
			},
			want: map[string]string{
				logview.ColChange:    "nrottnuz",
				logview.ColBookmarks: "main wip",
				logview.ColDesc:      "fix(help): scroll the overlay",
				logview.ColSize:      "3f +203 -17",
				logview.ColAuthor:    "kyleking",
				logview.ColAge:       "1w",
			},
		},
		{
			name: "empty working copy reads as empty, not as undescribed",
			rev:  jj.Revision{ChangeID: "xprkzuxx", Empty: true, WorkingCopy: true},
			want: map[string]string{
				logview.ColChange: "xprkzuxx",
				logview.ColDesc:   "(empty)",
				logview.ColSize:   "",
			},
		},
		{
			name: "undescribed revision with content says so",
			rev:  jj.Revision{ChangeID: "zoupzqyv", Files: 1, Added: 4},
			want: map[string]string{
				logview.ColDesc: logview.NoDescription,
				logview.ColSize: "1f +4 -0",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := logview.Cells(tc.rev)
			for key, want := range tc.want {
				if got[key] != want {
					t.Errorf("cell %q = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}

// The complaint that started the project: metadata must lose the row before
// the description does.
func TestColumnsCollapseMetadataFirst(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		present []string
		absent  []string
		width   int
	}{
		{
			name:  "wide terminal shows everything",
			width: 160,
			present: []string{
				logview.ColChange,
				logview.ColBookmarks,
				logview.ColDesc,
				logview.ColSize,
				logview.ColAuthor,
				logview.ColAge,
			},
		},
		{
			name:    "author goes first",
			width:   66,
			present: []string{logview.ColChange, logview.ColDesc, logview.ColSize},
			absent:  []string{logview.ColAuthor},
		},
		{
			name:    "narrow keeps the change id and the description",
			width:   36,
			present: []string{logview.ColChange, logview.ColDesc},
			absent:  []string{logview.ColAuthor, logview.ColAge, logview.ColSize, logview.ColBookmarks},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			layout := table.FitCompact(logview.Columns(), tc.width)
			for _, key := range tc.present {
				if layout.Width(key) == 0 {
					t.Errorf("column %q was hidden at width %d, want visible", key, tc.width)
				}
			}

			for _, key := range tc.absent {
				if layout.Width(key) != 0 {
					t.Errorf("column %q was visible at width %d, want hidden", key, tc.width)
				}
			}
		})
	}
}

func TestCompactAge(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"1 week ago":       "1w",
		"3 months ago":     "3mo",
		"56 years ago":     "56y",
		"2 hours ago":      "2h",
		"now":              "now",
		"a moment fromnow": "a moment fromnow",
	}

	for ago, want := range cases {
		t.Run(ago, func(t *testing.T) {
			t.Parallel()

			if got := logview.Cells(jj.Revision{Age: ago})[logview.ColAge]; got != want {
				t.Errorf("age %q rendered %q, want %q", ago, got, want)
			}
		})
	}
}

func TestGraphWidth(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{
		{Revision: &jj.Revision{Graph: "@  "}},
		{Graph: &jj.GraphLine{Graph: "│ ~  (elided revisions)"}},
		{Revision: &jj.Revision{Graph: "│ ○  "}},
	}

	if got := logview.GraphWidth(rows); got != 3 {
		t.Errorf("GraphWidth = %d, want 3", got)
	}
}
