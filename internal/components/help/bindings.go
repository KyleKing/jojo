// Package help renders the `?` overlay: the full key legend, grouped by the
// four layers jojo's keymap is designed in, scrollable when the legend is
// taller than the frame.
package help

import (
	"slices"

	"github.com/kyleking/aragonite/tui/keyhint"
)

// Layer is one tier of jojo's keymap, from the keys every screen understands
// to the ones only a power user reaches for.
type Layer int

// The four layers, in the order the legend groups them.
const (
	// L0 works everywhere: arrow movement, confirming, backing out.
	L0 Layer = iota
	// L1 is vim motion: hjkl, search, the command line.
	L1
	// L2 is jj's own mnemonics: one letter per verb.
	L2
	// L3 is the power tools: aliases, arrange, bisect, multi-select.
	L3
)

// Binding is one entry in jojo's keymap: the hint it renders as, and the
// layer it belongs to.
type Binding struct {
	keyhint.Hint
	Layer Layer
}

// Bindings is the whole keymap, as data. The statusbar and the root model
// both read from here, so a binding cannot drift between the footer, the
// help overlay, and the handler that acts on it.
var Bindings = []Binding{
	{Layer: L0, Hint: keyhint.Hint{What: "Universal", Head: true}},
	{Layer: L0, Hint: keyhint.Hint{Key: "↑ ↓ ← →", What: "move"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Enter", What: "select"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Esc", What: "back"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "q", What: "quit"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Tab", What: "switch pane"}},

	{Layer: L1, Hint: keyhint.Hint{What: "Vim motions", Head: true}},
	{Layer: L1, Hint: keyhint.Hint{Key: "j / k", What: "move a line"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "/", What: "search"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "?", What: "help"}},
	{Layer: L1, Hint: keyhint.Hint{Key: ":", What: "command"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "g / G", What: "top / bottom"}},

	{Layer: L2, Hint: keyhint.Hint{What: "Mnemonics", Head: true}},
	{Layer: L2, Hint: keyhint.Hint{Key: "n", What: "new"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "e", What: "edit"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "d", What: "describe"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "s", What: "squash"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "S", What: "split"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "a", What: "abandon"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "A", What: "absorb"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "D", What: "duplicate"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "r", What: "rebase"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "b", What: "bookmarks"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "o", What: "op log"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "u / Ctrl+r", What: "undo / redo"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "@", What: "jump to working copy"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "w", What: "workspaces"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "p", What: "forge pane"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "L", What: "edit the revset"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "space", What: "check for a batch action"}},

	{Layer: L3, Hint: keyhint.Hint{What: "Power", Head: true}},
	{Layer: L3, Hint: keyhint.Hint{What: "edit the revset aliases"}},
	{Layer: L3, Hint: keyhint.Hint{What: "jj arrange"}},
	{Layer: L3, Hint: keyhint.Hint{What: "bisect"}},
	{Layer: L3, Hint: keyhint.Hint{What: "multi-select operators"}},
}

// Hints returns the actionable hints (headings excluded) for the given
// layers, in table order. No layers returns every layer.
func Hints(layers ...Layer) []keyhint.Hint {
	out := make([]keyhint.Hint, 0, len(Bindings))

	for _, b := range Bindings {
		if b.Head {
			continue
		}

		if len(layers) > 0 && !slices.Contains(layers, b.Layer) {
			continue
		}

		out = append(out, b.Hint)
	}

	return out
}

// allHints returns every entry, headings included, in the shape the legend
// renders.
func allHints() []keyhint.Hint {
	out := make([]keyhint.Hint, len(Bindings))
	for i, b := range Bindings {
		out[i] = b.Hint
	}

	return out
}
