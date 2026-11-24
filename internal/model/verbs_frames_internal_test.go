package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

// verbSizes are the terminal sizes the new verbs' frames are pinned at.
//
//nolint:gochecknoglobals // a fixture
var verbSizes = [][2]int{{80, 24}, {100, 30}}

// verbFrame is the stack fixture at size with the cursor on the revision named.
func verbFrame(t *testing.T, size [2]int, cursor string) Model {
	t.Helper()

	m := asModel(t, update(testModel(t, stackRows()), tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
	if cursor != "" && !m.log.Focus(cursor) {
		t.Fatalf("no row %s", cursor)
	}

	return m
}

// assertVerbFrame holds frame to the terminal's size and framecheck, then to its golden.
func assertVerbFrame(t *testing.T, frame string, size [2]int) {
	t.Helper()

	if lines := strings.Split(frame, "\n"); len(lines) != size[1] {
		t.Errorf("the frame is %d rows, want %d", len(lines), size[1])
	}

	assertFits(t, frame, size[0], size[1], 0)
	tuitest.AssertGolden(t, frame)
}

// The b menu's which-key panel, and the bookmark picker with a preview landed: the chip on where it
// goes, the live command line, and -B off.
func TestBookmarkMenu_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("menu/%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m, _ := pressOne(t, verbFrame(t, size, "rnzxmskv"), "b")
			assertVerbFrame(t, m.render(), size)
		})

		t.Run(fmt.Sprintf("picker/%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, size, "rnzxmskv")
			m.openBookmarkPicker("parse", "twnsvwmr")

			moved := stackRows()
			moved[1].Revision.Bookmarks, moved[1].Revision.LocalBookmarks = []string{"parse*"}, []string{"parse"}
			moved[3].Revision.Bookmarks, moved[3].Revision.LocalBookmarks = nil, nil

			m = asModel(t, update(m, previewMsg{
				command: m.pick.command(), session: m.pick.session, preview: jj.Preview{Rows: moved},
			}))

			assertVerbFrame(t, m.render(), size)
		})
	}
}
