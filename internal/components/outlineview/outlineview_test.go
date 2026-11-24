package outlineview_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/outlineview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/readmark"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// stack is four chapters, parent first: a bookmarked one with a pull request, a conflicted one, an
// empty one, and the working copy, with the read strips in each of their states.
func stack() []outlineview.Chapter {
	return []outlineview.Chapter{
		{
			Rev: &jj.Revision{
				ChangeID: "kxqpmnzo", CommitID: "a1", Description: "feat(parse): read the new header format",
				Bookmarks: []string{"parse-header"}, LocalBookmarks: []string{"parse-header"},
				Files: 3, Added: 120, Removed: 14,
			},
			Forge: "#41 open · CI passing",
			Read:  readmark.Strip{Read: 5, Total: 5},
		},
		{
			Rev: &jj.Revision{
				ChangeID: "vwtrlsqy", CommitID: "b2", Description: "refactor: split the reader from the writer",
				Conflict: true, Files: 2, Added: 40, Removed: 38,
			},
			Read: readmark.Strip{Read: 1, Total: 4},
		},
		{Rev: &jj.Revision{ChangeID: "nnzolpqr", CommitID: "c3", Empty: true}},
		{
			Rev: &jj.Revision{
				ChangeID: "yqosmzxt", CommitID: "d4", Description: "test: cover the header fallbacks",
				WorkingCopy: true, Files: 1, Added: 9, Removed: 0,
			},
			Read: readmark.Strip{Total: 2},
		},
	}
}

func newOutline(width, height int) outlineview.Model {
	m := outlineview.New(ui.New(theme.Macchiato()))
	m.SetChapters("(trunk()..@) | (@:: ~ ::trunk())", stack())
	m.SetSize(width, height)

	return m
}

// The outline at the two sizes it is read at: every row fits the width, the frame is exactly the
// height, and the cursor starts on the working copy.
func TestView_Sizes(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := newOutline(size[0], size[1])
			frame := m.View()

			lines := strings.Split(frame, "\n")
			if len(lines) != size[1] {
				t.Errorf("frame is %d lines, want %d", len(lines), size[1])
			}

			for i, line := range lines {
				if w := lipgloss.Width(line); w > size[0] {
					t.Errorf("line %d is %d wide: %q", i, w, line)
				}
			}

			if m.Selected().ChangeID != "yqosmzxt" {
				t.Errorf("cursor on %s, want the working copy", m.Selected().ChangeID)
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

func TestMove_ClampsAndKeepsTheChapterAcrossARead(t *testing.T) {
	t.Parallel()

	m := newOutline(80, 10)

	if m.Move(1) {
		t.Error("moved past the last chapter")
	}

	if !m.Move(-2) || m.Selected().ChangeID != "vwtrlsqy" {
		t.Fatalf("Move(-2) landed on %s", m.Selected().ChangeID)
	}

	// A write reads the stack again; the cursor stays on the chapter it was on.
	m.SetChapters("again", stack()[1:])

	if m.Selected().ChangeID != "vwtrlsqy" {
		t.Errorf("after a reread the cursor is on %s", m.Selected().ChangeID)
	}
}

// At a narrow width the description is cut, and Overflow hands back the whole of it for the
// inspector, along with the forge state the collapsed column lost.
func TestOverflow_ReturnsWhatTheRowCut(t *testing.T) {
	t.Parallel()

	m := newOutline(50, 10)
	m.Move(-3)

	got := m.Overflow()
	if len(got) == 0 || got[0] != "feat(parse): read the new header format" {
		t.Errorf("Overflow() = %q, want the whole description first", got)
	}

	if !strings.Contains(strings.Join(got, "|"), "#41 open") {
		t.Errorf("Overflow() = %q, want the forge state the narrow row dropped", got)
	}

	wide := newOutline(200, 10)
	wide.Move(-3)

	if got := wide.Overflow(); len(got) != 0 {
		t.Errorf("Overflow() at 200 columns = %q, want nothing cut", got)
	}
}

func TestSetRead_UpdatesOneChapter(t *testing.T) {
	t.Parallel()

	m := newOutline(120, 10)
	m.SetRead("nnzolpqr", readmark.Strip{Read: 2, Total: 2})

	if got := m.Chapters()[2].Read; got.Read != 2 {
		t.Errorf("strip = %+v", got)
	}
}
