package confirm_test

import (
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"

	"github.com/kyleking/jojo/internal/components/confirm"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// cjkDetail is a wide-glyph fixture, not a translation string.
const cjkDetail = "日本語の長い詳細説明文がここに入ります" //nolint:gosmopolitan // test fixture, not a translation

// worstCaseRequest gives the dialog a long detail line, wide CJK text, and an
// emoji, none of which the dialog scrolls.
func worstCaseRequest() confirm.Request {
	return confirm.Request{
		Key:   "push",
		Title: "Push to remote?",
		Detail: []string{
			strings.Repeat("this push updates a bookmark on the remote, which is not undoable ", 3),
			cjkDetail,
			"emoji detail 🚀🔥",
		},
		Danger: "cannot be undone once it leaves this machine",
	}
}

func TestViewFitsAtEverySize(t *testing.T) {
	t.Parallel()

	m := confirm.New(testSkin())
	m.Open(worstCaseRequest())

	tuitest.AssertOverlayInvariants(t, m.View)
}

func TestViewDegradesHintsRatherThanSplittingOne(t *testing.T) {
	t.Parallel()

	skin := testSkin()
	m := confirm.New(skin)
	m.Open(worstCaseRequest())

	styles := skin.OverlayStyles()
	for _, size := range tuitest.HintSizes() {
		contentWidth := overlay.ContentWidth(size.Width, styles)

		want := tuitest.StripANSI(ui.FitHints(skin.KeyHintStyles(), []keyhint.Hint{
			{Key: "y", What: "yes"},
			{Key: "n", What: "no"},
		}, contentWidth))

		got := tuitest.StripANSI(m.View(size.Width, size.Height))
		if !strings.Contains(got, want) {
			t.Errorf("View(%s) does not contain the degraded hint line %q:\n%s", size, want, got)
		}
	}
}
