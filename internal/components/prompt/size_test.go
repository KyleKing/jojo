package prompt_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/tuitest"
)

// worstCaseTitle and worstCaseInitial are a long title and a long,
// non-ASCII seeded value, since a describe prompt can be reopened on an
// existing multi-line-worthy description.
const worstCaseTitle = "Describe a rather long change with a title that keeps going and going"

// cjkWord is a wide-glyph fixture, not a translation string.
const cjkWord = "日本語" //nolint:gosmopolitan // test fixture, not a translation

var worstCaseInitial = strings.Repeat("a very long seeded description ", 4) + cjkWord + " 🚀"

func TestViewFitsAtEverySize(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open(worstCaseTitle, worstCaseInitial)

	tuitest.AssertOverlayInvariants(t, m.View)
}

// The footer sheds keys as the width runs out and keeps the badge and save, and it is the frame's last
// row at exactly the frame's width.
func TestViewDegradesHintsRatherThanSplittingOne(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open(worstCaseTitle, worstCaseInitial)

	for _, size := range tuitest.HintSizes() {
		lines := strings.Split(m.View(size.Width, size.Height), "\n")
		footer := lines[len(lines)-1]

		if w := lipgloss.Width(footer); w != size.Width {
			t.Errorf("View(%s) footer is %d wide", size, w)
		}

		if got := tuitest.StripANSI(footer); size.Width >= len("INSERT ⏎ save") && !strings.Contains(got, "save") {
			t.Errorf("View(%s) footer %q dropped save", size, got)
		}
	}
}
