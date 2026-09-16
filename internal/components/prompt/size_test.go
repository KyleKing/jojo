package prompt_test

import (
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
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

func TestViewDegradesHintsRatherThanSplittingOne(t *testing.T) {
	t.Parallel()

	skin := testSkin()
	m := prompt.New(skin)
	m.Open(worstCaseTitle, worstCaseInitial)

	styles := skin.OverlayStyles()
	for _, size := range tuitest.HintSizes() {
		contentWidth := overlay.ContentWidth(size.Width, styles)

		want := tuitest.StripANSI(ui.FitHints(skin.KeyHintStyles(), []keyhint.Hint{
			{Key: "enter", What: "submit"},
			{Key: "esc", What: "cancel"},
		}, contentWidth))

		got := tuitest.StripANSI(m.View(size.Width, size.Height))
		if !strings.Contains(got, want) {
			t.Errorf("View(%s) does not contain the degraded hint line %q:\n%s", size, want, got)
		}
	}
}
