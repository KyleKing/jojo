package detailview_test

import (
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/detailview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 70, 16

func testSkin() ui.Skin {
	return ui.New(theme.Macchiato())
}

func detail() jj.Detail {
	return jj.Detail{
		ChangeID:    "kxryrzwz",
		CommitID:    "d11fcf8e9a1b",
		AuthorName:  "Test User",
		AuthorEmail: "test@example.com",
		Authored:    "2026-09-16 08:33",
		Committed:   "2026-09-16 09:10",
		Description: "fix the parser\n\nThe separator can appear inside a description, so the\n" +
			"split has to be bounded.",
		Parents: []string{"mursrsmx", "ovrvxwyt"},
	}
}

// The overlay exists for the lines the log row cannot show, so the body after the first must survive.
func TestViewKeepsTheWholeDescription(t *testing.T) {
	t.Parallel()

	m := detailview.New(testSkin())
	m.Open(detail())

	view := tuitest.StripANSI(m.View(testWidth, testHeight))

	wants := []string{"fix the parser", "split has to be bounded", "mursrsmx ovrvxwyt", "test@example.com"}

	for _, want := range wants {
		if !strings.Contains(view, want) {
			t.Errorf("View() is missing %q:\n%s", want, view)
		}
	}
}

func TestViewNamesAnUndescribedRevision(t *testing.T) {
	t.Parallel()

	empty := detail()
	empty.Description = "   \n"

	m := detailview.New(testSkin())
	m.Open(empty)

	if view := tuitest.StripANSI(m.View(testWidth, testHeight)); !strings.Contains(view, "(no description set)") {
		t.Errorf("View() does not name an undescribed revision:\n%s", view)
	}
}

// Any key closes, because the overlay only reads and a reader should not have to remember which one.
func TestAnyKeyCloses(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"esc", "q", "j", "enter"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := detailview.New(testSkin())
			m.Open(detail())

			if cmd := m.Update(tuitest.Key(key)); cmd == nil {
				t.Fatalf("Update(%s) produced no command", key)
			}

			if m.IsOpen() {
				t.Errorf("Update(%s) left the overlay open", key)
			}
		})
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := detailview.New(testSkin())
	m.Open(detail())

	tuitest.AssertGolden(t, tuitest.StripANSI(m.View(testWidth, testHeight)))
}
