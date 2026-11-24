package fileview_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func workingCopy(staging fileview.Staging) fileview.Model {
	m := fileview.New(ui.New(theme.Macchiato()))
	m.OpenWorkingCopy("wcwcwcwc", []jj.FileChange{
		{Status: "M", Path: "unstaged.go"},
		{Status: "A", Path: "new.go"},
	}, staging)

	return m
}

func indexed() fileview.Staging {
	return fileview.Staging{
		Index:   "idxidxid",
		Message: "wip: parser",
		Files:   []jj.FileChange{{Status: "M", Path: "staged.go"}},
	}
}

//nolint:ireturn // a key's outcome is whichever message the list sends.
func press(t *testing.T, m *fileview.Model, keys ...string) tea.Msg {
	t.Helper()

	var cmd tea.Cmd
	for _, k := range keys {
		cmd = m.Update(tuitest.Key(k))
	}

	if cmd == nil {
		return nil
	}

	return cmd()
}

// The file list of @ reads like lazygit's status panel: the working copy's files first, then what the
// index holds, each tagged so the two cannot be mistaken for one another at any width.
func TestView_WorkingCopyListsStagedFilesBelowUnstaged(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{60, 12}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			m := workingCopy(indexed())
			tuitest.AssertGolden(t, m.View(size.width, size.height))
		})
	}
}

func TestSpace_StagesAWorkingCopyFileAndUnstagesAStagedOne(t *testing.T) {
	t.Parallel()

	m := workingCopy(indexed())

	stage, ok := press(t, &m, "space").(fileview.StageMsg)
	if !ok || stage.Path != "unstaged.go" || stage.Revision != "wcwcwcwc" {
		t.Fatalf("space on an unstaged file sent %#v, want a stage of unstaged.go", stage)
	}

	unstage, ok := press(t, &m, "j", "j", "space").(fileview.UnstageMsg)
	if !ok || unstage.Path != "staged.go" {
		t.Fatalf("space on a staged file sent %#v, want an unstage of staged.go", unstage)
	}
}

// A staged row belongs to the index, so the per-file verbs that already existed act on @- for it.
func TestEnter_DiffsAStagedFileInTheIndex(t *testing.T) {
	t.Parallel()

	m := workingCopy(indexed())

	diff, ok := press(t, &m, "G", "enter").(fileview.DiffMsg)
	if !ok || diff.Revision != "idxidxid" || diff.Path != "staged.go" {
		t.Fatalf("enter on a staged file sent %#v, want its diff in the index", diff)
	}
}

func TestCommit_OffersTheIndexDescription(t *testing.T) {
	t.Parallel()

	m := workingCopy(indexed())

	commit, ok := press(t, &m, "c").(fileview.CommitMsg)
	if !ok || commit.Message != "wip: parser" {
		t.Fatalf("C sent %#v, want a commit offering the index's description", commit)
	}
}

// Each refusal stays in the list as a warning and sends nothing, so no key here can rewrite a revision
// the vocabulary does not own.
func TestStagingKeys_RefuseWithAReason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		open func() fileview.Model
		name string
		key  string
		want string
	}{
		{
			name: "space when the parent cannot be the index",
			open: func() fileview.Model {
				return workingCopy(fileview.Staging{Refused: "@- cannot act as the index: zzz is immutable"})
			},
			key:  "space",
			want: "is immutable",
		},
		{
			name: "commit with nothing staged",
			open: func() fileview.Model { return workingCopy(fileview.Staging{Index: "idxidxid"}) },
			key:  "c",
			want: "nothing staged",
		},
		{
			name: "space outside the working copy",
			open: func() fileview.Model {
				m := fileview.New(ui.New(theme.Macchiato()))
				m.Open("other", []jj.FileChange{{Status: "M", Path: "a.go"}})

				return m
			},
			key:  "space",
			want: "working copy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := tc.open()
			if msg := press(t, &m, tc.key); msg != nil {
				t.Fatalf("%s sent %T, want nothing", tc.key, msg)
			}

			if view := tuitest.StripANSI(m.View(80, 12)); !strings.Contains(view, tc.want) {
				t.Errorf("the list does not say %q:\n%s", tc.want, view)
			}
		})
	}
}

// Staging file after file reopens the list each time, and sending the cursor back to the top on every
// press would make a run of stages walk the same row.
func TestOpenWorkingCopy_KeepsTheCursorWhenRefreshed(t *testing.T) {
	t.Parallel()

	m := workingCopy(indexed())
	press(t, &m, "j")

	m.OpenWorkingCopy("wcwcwcwc", []jj.FileChange{{Status: "A", Path: "new.go"}}, fileview.Staging{
		Index: "idxidxid",
		Files: []jj.FileChange{{Status: "M", Path: "staged.go"}, {Status: "M", Path: "unstaged.go"}},
	})

	unstage, ok := press(t, &m, "space").(fileview.UnstageMsg)
	if !ok || unstage.Path != "staged.go" {
		t.Fatalf("space after a refresh sent %#v, want the row now under the kept cursor", unstage)
	}
}
