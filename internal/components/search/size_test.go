package search_test

import (
	"testing"

	"github.com/kyleking/jojo/internal/components/search"
	"github.com/kyleking/jojo/internal/tuitest"
)

// widths mirrors the widths in tuitest.OverlaySizes; search has no height of
// its own, since it renders a single line into the footer's row.
func widths() []int {
	sizes := tuitest.OverlaySizes()

	widths := make([]int, 0, len(sizes))
	for _, size := range sizes {
		widths = append(widths, size.Width)
	}

	return widths
}

// cjkPattern is a wide-glyph fixture, not a translation string.
const cjkPattern = "日本語のとても長い検索パターンです" //nolint:gosmopolitan // test fixture, not a translation

func TestViewFitsAtEveryWidth(t *testing.T) {
	t.Parallel()

	m := search.New(testSkin())
	m.Open()
	typeString(&m, cjkPattern+" 🚀🔥 and some ascii too")
	m.SetMatches(999, 42)

	for _, width := range widths() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("View(%d) panicked: %v", width, r)
				}
			}()

			tuitest.AssertLineFits(t, width, m.View(width))
		}()
	}
}
