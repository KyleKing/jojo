// Package ui is jojo's own skin: the translation from aragonite's palette and
// generic skin into the vocabulary this app's rows and chrome ask for, and the
// Styles values aragonite's rendering packages need handed to them. Nothing
// else in jojo builds a lipgloss.Style inline.
package ui

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/kyleking/aragonite/tui/icons"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/overlay"
	"github.com/kyleking/aragonite/tui/skin"
	"github.com/kyleking/aragonite/tui/theme"
)

// Skin is the full set of faces jojo draws with, held by pointer: the faces are about 23 KB, and
// every pane keeps the skin it was built with, so a value would be copied into each pane and again
// on every message the root model handles. A Skin is never changed once built; ForProfile and
// WithIcons return a new one.
type Skin struct {
	*Faces
}

// Faces is aragonite's generic skin plus the jj-specific roles, each paired with an attribute
// (weight, slant, or decoration) that keeps the roles apart even once NO_COLOR strips color.
type Faces struct {
	skin.Skin

	// Immutable dims a row jj refuses to rewrite.
	Immutable lipgloss.Style
	// Conflicted marks a row with an unresolved conflict.
	Conflicted lipgloss.Style
	// Divergent marks a row whose change a rewrite left unreachable; the
	// strikethrough is what a dead side is.
	Divergent lipgloss.Style
	// WorkingCopy marks the row @ points at.
	WorkingCopy lipgloss.Style
	// Bookmark draws a bookmark chip beside a row.
	Bookmark lipgloss.Style
	// ChangeIDPrefix highlights the unique, typeable prefix of a change ID.
	ChangeIDPrefix lipgloss.Style
	Added          lipgloss.Style
	Removed        lipgloss.Style
	// Selected marks the row under the cursor.
	Selected lipgloss.Style
	// Palette is the flavor this skin was built from, for the few callers that need a raw color
	// rather than a face: a diff needs more of them than a skin has roles for.
	Palette theme.Palette
	// Icons draws a file's type beside its name: Nerd Font glyphs unless
	// ARAGONITE_ICONS asks for plain unicode, shared with the sibling tools.
	Icons icons.Set
	// Diff is what a changed line is drawn with in the diff pane: its wash, the deeper mark on
	// the words that changed, and the sign bar, the same faces second-look and jj-diff use.
	Diff skin.DiffFaces
	// Profile is the color depth the skin was fitted to, which Hue fits a meaningful color to.
	Profile colorprofile.Profile
}

// accent returns the color that tells jojo apart from aragonite's other
// tools, which each pick their own per skin's design.
func accent(p theme.Palette) color.Color {
	return p.Peach
}

// New builds a skin from a palette, so callers (and tests) can pin a flavor
// rather than reading the terminal.
func New(p theme.Palette) Skin {
	f := &Faces{
		Skin:     skin.New(p, accent(p)),
		Selected: lipgloss.NewStyle().Reverse(true),
		Icons:    icons.New(icons.Resolve(""), p),
		Diff:     skin.Diff(p, true),
		Palette:  p,
		Profile:  colorprofile.TrueColor,
	}
	f.withRoles()

	return Skin{f}
}

// withRoles builds jojo's own faces for the skin's profile. Each carries its meaning by hue (a
// conflict red, a bookmark green), so each goes through Hue, which keeps it off the grey ramp in 256
// colors: Latte's red would otherwise answer grey 241.
func (s *Faces) withRoles() {
	p, base := s.Palette, lipgloss.NewStyle()
	hue := func(c color.Color) color.Color { return theme.Hue(c, s.Profile) }
	a := hue(accent(p))

	s.Immutable = base.Foreground(p.Overlay1).Faint(true)
	s.Conflicted = base.Foreground(hue(p.Red)).Bold(true)
	s.Divergent = base.Foreground(hue(p.Yellow)).Strikethrough(true)
	s.WorkingCopy = base.Foreground(a).Bold(true).Underline(true)
	s.Bookmark = base.Foreground(hue(p.Green)).Italic(true)
	s.ChangeIDPrefix = base.Foreground(a).Bold(true)
	s.Added = base.Foreground(hue(p.Green))
	s.Removed = base.Foreground(hue(p.Red))
}

// Hue is c fitted to the skin's profile for a color that means something by its hue: a side of a
// diff, a conflict, a bookmark.
func (s Skin) Hue(c color.Color) color.Color { return theme.Hue(c, s.Profile) }

// SignBar says whether a changed line's side can be drawn as a colored bar, which it cannot under
// NO_COLOR, where only a + or a - says which side a line is on.
func (s Skin) SignBar() bool {
	return s.Diff.Sign(true) == skin.SignBar
}

// forTerminal fits every background to the terminal's color profile: blended tints in truecolor,
// greys in 256 colors (the cube's dark greens and reds are saturated blocks), none in 16.
func forTerminal(s Skin) Skin {
	return ForProfile(s, theme.Profile(os.Stdout, os.Environ()))
}

// ForProfile refits s to profile, which is what main also hands tea.WithColorProfile.
func ForProfile(s Skin, profile colorprofile.Profile) Skin {
	f := *s.Faces
	f.Skin = skin.For(f.Palette, accent(f.Palette), profile)
	f.Diff = skin.DiffFor(f.Palette, profile)
	f.Profile = profile
	f.withRoles()

	return Skin{&f}
}

// WithIcons is s drawing file types with set.
func (s Skin) WithIcons(set icons.Set) Skin {
	f := *s.Faces
	f.Icons = set

	return Skin{&f}
}

// ErrUnknownFlavor marks a theme name no flavor answers to.
var ErrUnknownFlavor = errors.New("unknown theme")

// Picker is the skin for a terminal whose background is dark or not. A named flavor ignores it.
type Picker func(dark bool) Skin

// Flavor resolves a configured theme name. A named flavor is fixed; an empty name leaves it to
// CATPPUCCIN_THEME and then to the terminal's background, and ask reports that the background is
// needed. Flavor never queries the terminal itself: a query made before the program reads its
// input reads the keys typed meanwhile too, and throws them away with the reply, so the caller
// learns the background from the program's own input (launch). A name jojo does not know is an
// error rather than a silent fallback, since a typo would otherwise look like the setting having
// no effect.
func Flavor(name string) (Picker, bool, error) {
	switch strings.ToLower(name) {
	case "":
		override := os.Getenv(theme.EnvOverride)
		ask := false
		theme.Select(override, func() bool { ask = true; return true })

		return func(dark bool) Skin {
			return forTerminal(New(theme.Select(override, func() bool { return dark })))
		}, ask, nil
	case "latte", "light":
		return func(bool) Skin { return forTerminal(New(theme.Latte())) }, false, nil
	case "macchiato", "dark":
		return func(bool) Skin { return forTerminal(New(theme.Macchiato())) }, false, nil
	}

	return nil, false, fmt.Errorf("%w: %q, want latte or macchiato", ErrUnknownFlavor, name)
}

// KeyHintStyles is the face keyhint renders every binding hint and legend
// with, so callers never assemble it themselves.
func (s Skin) KeyHintStyles() keyhint.Styles {
	return keyhint.Styles{
		Key:  s.Key,
		Text: s.Body,
		Head: s.Title,
	}
}

// FooterHintStyles is the face a footer draws its keys in, the one jj-diff and second-look draw
// theirs in: the key in the accent and the word muted beside it, without brackets, so the two faces
// alone tell key from word. A hint marked Off (an unchosen option, a flag that is off) drops to the
// quietest face, so a row of flags reads as the command's current shape.
func (s Skin) FooterHintStyles() keyhint.Styles {
	return keyhint.Styles{
		Key:  s.Accent,
		Text: s.Muted,
		Head: s.Title,
		Off:  lipgloss.NewStyle().Foreground(s.Palette.Overlay0),
		Bare: true,
		Gap:  keyhint.Gap,
	}
}

// overlayPaddingV and overlayPaddingH are the frame's inner margin, vertical
// and horizontal.
const (
	overlayPaddingV = 1
	overlayPaddingH = 2
)

// OverlayStyles is the face overlay frames a modal with.
func (s Skin) OverlayStyles() overlay.Styles {
	frame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.Edge.GetForeground()).
		Padding(overlayPaddingV, overlayPaddingH)

	return overlay.Styles{Frame: frame, Elision: s.Muted}
}

// PanelStyles is the face of the which-key panel a waiting prefix floats over the frame: the
// overlay's border, tight, around the legend's faces.
func (s Skin) PanelStyles() keyhint.PanelStyles {
	return keyhint.PanelStyles{
		Frame: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(s.Edge.GetForeground()).Padding(0, 1),
		Styles: s.FooterHintStyles(),
		Count:  s.Muted,
		On:     s.Accent,
	}
}
