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

// A big refactor's size was cut to "92f +1837 -…" even at 200 columns. Counts are compact, as
// second-look writes them, and the widest one fits the column whole.
func TestCells_CompactSize(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		want                  string
		files, added, removed int
	}{
		{"92f +1.8k -1.8k", 92, 1837, 1837},
		{"3f +999 -1k", 3, 999, 1000},
		{"1f +1k -1.1k", 1, 1049, 1050},
		{"12f +10k -18k", 12, 9960, 18_400},
		{"999f +180k -1M", 999, 180_000, 999_600},
		{"1.2kf +1.5M -12M", 1234, 1_450_000, 12_000_000},
	} {
		got := logview.Cells(jj.Revision{Files: tc.files, Added: tc.added, Removed: tc.removed})[logview.ColSize]
		if got != tc.want {
			t.Errorf("size of %d/%d/%d = %q, want %q", tc.files, tc.added, tc.removed, got, tc.want)
		}
	}

	var size table.Column

	for _, col := range logview.Columns() {
		if col.Key == logview.ColSize {
			size = col
		}
	}

	if widest := len("999f +9.9k -9.9k"); size.Max < widest {
		t.Errorf("the size column holds %d cells, want room for %d", size.Max, widest)
	}
}

// The size column stayed at its floor, so even at 200 columns a big change's counts were cut. It is
// as wide as the widest size among the rows, and only that wide.
func TestFitRows_SizeColumnHoldsTheWidestSize(t *testing.T) {
	t.Parallel()

	small := jj.LogRow{Revision: &jj.Revision{ChangeID: "aaaa", Files: 2, Added: 4}}
	big := jj.LogRow{Revision: &jj.Revision{ChangeID: "bbbb", Files: 92, Added: 1837, Removed: 1837}}

	for _, width := range []int{120, 200} {
		got := logview.FitRows(width, 2, []jj.LogRow{small, big}).Width(logview.ColSize)
		if got < len("92f +1.8k -1.8k") {
			t.Errorf("at %d the size column is %d cells, too narrow for 92f +1.8k -1.8k", width, got)
		}

		if got = logview.FitRows(width, 2, []jj.LogRow{small}).Width(logview.ColSize); got != 12 {
			t.Errorf("at %d with small sizes the column is %d cells, want its floor of 12", width, got)
		}
	}
}
