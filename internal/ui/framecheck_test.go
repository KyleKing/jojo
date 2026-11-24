package ui_test

import (
	"testing"

	"github.com/kyleking/aragonite/tui/framecheck"
)

// TestChromeIsWidthStable holds every string and rune literal in jojo's own code to the chrome glyph
// policy: no glyph terminals measure differently (a text-default emoji such as ◀, a selector, a Nerd
// icon) and no wide one, since the screen's own marks must line up in every terminal. Content from
// outside, descriptions and titles, goes through glyph.Settle where it enters a row instead.
func TestChromeIsWidthStable(t *testing.T) {
	t.Parallel()

	framecheck.Source(t, "../..")
}
