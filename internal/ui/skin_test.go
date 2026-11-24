package ui_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/ui"
)

// NO_COLOR strips color SGR codes but not bold, faint, italic, underline, or
// reverse (see colorprofile.Detect's doc comment), so every row state has to
// carry a distinct attribute, not only a distinct hue, or two states collapse
// into the same plain text once color is gone.
func TestNew_DegradesUnderNoColor(t *testing.T) {
	t.Parallel()

	skin := ui.New(theme.Macchiato())

	roles := map[string]string{
		"immutable":    skin.Immutable.Render("row"),
		"conflicted":   skin.Conflicted.Render("row"),
		"working copy": skin.WorkingCopy.Render("row"),
		"bookmark":     skin.Bookmark.Render("row"),
		"selected":     skin.Selected.Render("row"),
	}

	degraded := make(map[string]string, len(roles))

	for name, rendered := range roles {
		out := downsample(t, rendered)
		degraded[name] = out

		for _, colorCode := range []string{"30", "31", "32", "33", "34", "35", "36", "37", "38", "39"} {
			if strings.Contains(out, "\x1b["+colorCode) {
				t.Errorf("%s still carries color code %s after NO_COLOR downsampling: %q", name, colorCode, out)
			}
		}
	}

	seen := make(map[string]string, len(degraded))

	for name, out := range degraded {
		if other, ok := seen[out]; ok {
			t.Errorf("%s and %s render identically once color is stripped: %q", name, other, out)
		}

		seen[out] = name
	}
}

func downsample(t *testing.T, s string) string {
	t.Helper()

	var buf bytes.Buffer

	w := colorprofile.Writer{Forward: &buf, Profile: colorprofile.ASCII}
	if _, err := w.WriteString(s); err != nil {
		t.Fatalf("downsample: %v", err)
	}

	return buf.String()
}

// The skin is refit to the profile the renderer draws at: in truecolor the diff washes are the two
// tints, and in 256 colors they are something else, not the truecolor tint quantized into the cube's
// near-black greys. A tmux with COLORTERM=truecolor is a truecolor terminal.
func TestForProfile_FitsTheDiffWashToTheProfile(t *testing.T) {
	t.Parallel()

	base := ui.New(theme.Macchiato())
	truecolor := ui.ForProfile(base, colorprofile.TrueColor)
	ansi256 := ui.ForProfile(base, colorprofile.ANSI256)

	if truecolor.Diff.AddWash == truecolor.Diff.RemoveWash {
		t.Error("in truecolor the added and removed washes are the same color")
	}

	if truecolor.Diff.AddWash == ansi256.Diff.AddWash {
		t.Error("the 256-color wash is the truecolor one, left to quantize")
	}

	env := []string{"TERM=tmux-256color", "COLORTERM=truecolor", "TTY_FORCE=1"}
	if got := theme.Profile(os.Stdout, env); got != colorprofile.TrueColor {
		t.Errorf("tmux with COLORTERM=truecolor is %v, want truecolor", got)
	}
}
