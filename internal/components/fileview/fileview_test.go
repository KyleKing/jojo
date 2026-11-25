package fileview_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/fileview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func threeFiles() []jj.FileChange {
	return []jj.FileChange{
		{Status: "M", Path: "one.go"},
		{Status: "A", Path: "two.go"},
		{Status: "D", Path: "three.go"},
	}
}

func workingCopy() fileview.Model {
	m := fileview.New(ui.New(theme.Macchiato()))
	m.OpenWorkingCopy("wcwcwcwc", "wip: parser", threeFiles())

	return m
}

func other() fileview.Model {
	m := fileview.New(ui.New(theme.Macchiato()))
	m.Open("otherrev", "older work", threeFiles())

	return m
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

// A checked row leads with the log's check mark and the footer names the tick, at every width.
func TestView_CheckedRowsCarryTheMark(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{80, 12}, {60, 12}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			m := workingCopy()
			press(t, &m, "space", "j", "j", "space")
			tuitest.AssertGolden(t, m.View(size.width, size.height))
		})
	}
}

// Space is a tick: it sends nothing, so nothing can write, and pressing it again unchecks.
func TestSpace_ChecksWithoutWriting(t *testing.T) {
	t.Parallel()

	m := workingCopy()

	if msg := press(t, &m, "space"); msg != nil {
		t.Fatalf("space sent %T, want nothing: a check never writes", msg)
	}

	press(t, &m, "j", "j", "space")

	if got := m.Checked(); !slices.Equal(got, []string{"one.go", "three.go"}) {
		t.Errorf("checked %v", got)
	}

	press(t, &m, "k", "k", "space")

	if got := m.Checked(); !slices.Equal(got, []string{"three.go"}) {
		t.Errorf("space again did not uncheck: %v", got)
	}
}

// s, S, and c act on every checked file, and on the file under the cursor when none is checked.
func TestVerbs_ActOnTheCheckedFilesElseTheCursor(t *testing.T) {
	t.Parallel()

	paths := func(msg tea.Msg) []string {
		switch msg := msg.(type) {
		case fileview.MoveMsg:
			return msg.Paths
		case fileview.SplitMsg:
			return msg.Paths
		case fileview.CommitMsg:
			return msg.Paths
		}

		return nil
	}

	for _, key := range []string{"s", "S", "c"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := workingCopy()
			if got := paths(press(t, &m, "j", key)); !slices.Equal(got, []string{"two.go"}) {
				t.Errorf("%s with nothing checked acted on %v, want the cursor's file", key, got)
			}

			press(t, &m, "k", "space", "j", "j", "space")

			if got := paths(press(t, &m, key)); !slices.Equal(got, []string{"one.go", "three.go"}) {
				t.Errorf("%s acted on %v, want the checked files", key, got)
			}
		})
	}
}

func TestCommit_OffersTheWorkingCopyDescription(t *testing.T) {
	t.Parallel()

	m := workingCopy()

	commit, ok := press(t, &m, "c").(fileview.CommitMsg)
	if !ok || commit.Message != "wip: parser" || commit.Revision != "wcwcwcwc" {
		t.Fatalf("c sent %#v, want a commit offering @'s description", commit)
	}
}

func TestSplit_NamesTheRevision(t *testing.T) {
	t.Parallel()

	m := other()

	split, ok := press(t, &m, "S").(fileview.SplitMsg)
	if !ok || split.Revision != "otherrev" || split.Message != "older work" {
		t.Fatalf("S sent %#v, want a split of otherrev", split)
	}
}

// jj commit only takes the working copy, so c elsewhere stays in the list as a warning.
func TestCommit_RefusesOutsideTheWorkingCopy(t *testing.T) {
	t.Parallel()

	m := other()
	if msg := press(t, &m, "c"); msg != nil {
		t.Fatalf("c sent %T, want nothing", msg)
	}

	if view := tuitest.StripANSI(m.View(100, 12)); !strings.Contains(view, "working copy") {
		t.Errorf("the list does not say why:\n%s", view)
	}

	if view := tuitest.StripANSI(m.View(100, 12)); strings.Contains(view, "c commit") {
		t.Errorf("the footer offers c where it refuses:\n%s", view)
	}
}

// Reopening the list, as every write's reload does, starts with nothing checked.
func TestOpen_ClearsTheChecks(t *testing.T) {
	t.Parallel()

	m := workingCopy()
	press(t, &m, "space")
	m.OpenWorkingCopy("wcwcwcwc", "wip", threeFiles())

	if got := m.Checked(); len(got) != 0 {
		t.Errorf("checks survived a reopen: %v", got)
	}
}
