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

// Skin is the full set of faces jojo draws with: aragonite's generic skin,
// plus the jj-specific roles, each paired with an attribute (weight, slant,
// or decoration) that keeps the roles apart even once NO_COLOR strips color.
type Skin struct {
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
}

// accent returns the color that tells jojo apart from aragonite's other
// tools, which each pick their own per skin's design.
func accent(p theme.Palette) color.Color {
	return p.Peach
}

// New builds a skin from a palette, so callers (and tests) can pin a flavor
// rather than reading the terminal.
func New(p theme.Palette) Skin {
	a := accent(p)
	base := lipgloss.NewStyle()

	return Skin{
		Skin:           skin.New(p, a),
		Immutable:      base.Foreground(p.Overlay1).Faint(true),
		Conflicted:     base.Foreground(p.Red).Bold(true),
		Divergent:      base.Foreground(p.Yellow).Strikethrough(true),
		WorkingCopy:    base.Foreground(a).Bold(true).Underline(true),
		Bookmark:       base.Foreground(p.Green).Italic(true),
		ChangeIDPrefix: base.Foreground(a).Bold(true),
		Added:          base.Foreground(p.Green),
		Removed:        base.Foreground(p.Red),
		Selected:       base.Reverse(true),
		Icons:          icons.New(icons.Resolve(""), p),
		Diff:           skin.Diff(p, true),
		Palette:        p,
	}
}

// SignBar says whether a changed line's side can be drawn as a colored bar, which it cannot under
// NO_COLOR, where only a + or a - says which side a line is on.
func (s Skin) SignBar() bool {
	return s.Diff.Sign(true) == skin.SignBar
}

// forTerminal fits the diff faces to the terminal's colors: a 256-color terminal needs deeper
// tints than a truecolor one, or both sides quantize into the same grey.
func forTerminal(s Skin) Skin {
	s.Diff = skin.Diff(s.Palette, colorprofile.Detect(os.Stdout, os.Environ()) == colorprofile.TrueColor)

	return s
}

// Detected builds a skin for whichever flavor the terminal's background calls
// for, which is what main starts with.
func Detected() Skin {
	return forTerminal(New(theme.Detect()))
}

// ErrUnknownFlavor marks a theme name no flavor answers to.
var ErrUnknownFlavor = errors.New("unknown theme")

// Flavor builds a skin for a named flavor, or reads the terminal when the name is empty. A name
// jojo does not know is an error rather than a silent fallback, since a typo would otherwise look
// like the setting having no effect.
func Flavor(name string) (Skin, error) {
	switch strings.ToLower(name) {
	case "":
		return Detected(), nil
	case "latte", "light":
		return forTerminal(New(theme.Latte())), nil
	case "macchiato", "dark":
		return forTerminal(New(theme.Macchiato())), nil
	}

	return Skin{}, fmt.Errorf("%w: %q, want latte or macchiato", ErrUnknownFlavor, name)
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
		BorderForeground(s.Accent.GetForeground()).
		Padding(overlayPaddingV, overlayPaddingH)

	return overlay.Styles{Frame: frame, Elision: s.Muted}
}
