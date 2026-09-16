// Package ui is jojo's own skin: the translation from aragonite's palette and
// generic skin into the vocabulary this app's rows and chrome ask for, and the
// Styles values aragonite's rendering packages need handed to them. Nothing
// else in jojo builds a lipgloss.Style inline.
package ui

import (
	"errors"
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
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
		WorkingCopy:    base.Foreground(a).Bold(true).Underline(true),
		Bookmark:       base.Foreground(p.Green).Italic(true),
		ChangeIDPrefix: base.Foreground(a).Bold(true),
		Added:          base.Foreground(p.Green),
		Removed:        base.Foreground(p.Red),
		Selected:       base.Reverse(true),
		Palette:        p,
	}
}

// Detected builds a skin for whichever flavor the terminal's background calls
// for, which is what main starts with.
func Detected() Skin {
	return New(theme.Detect())
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
		return New(theme.Latte()), nil
	case "macchiato", "dark":
		return New(theme.Macchiato()), nil
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
