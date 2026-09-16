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

// Binding is one entry in jojo's keymap: the hint it renders as, the layer it
// belongs to, and the key the root model dispatches on. Run is empty for a
// binding no single key runs, such as movement or a heading, and those are the
// entries the command palette leaves out.
type Binding struct {
	// Cmd is the jj invocation this binding runs, written as a shape rather than a real command
	// line. The palette shows it, which is how someone learns the CLI from the TUI.
	Cmd string
	Run string
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
	{Layer: L1, Run: "/", Hint: keyhint.Hint{Key: "/", What: "search the log"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "?", What: "help"}},
	{Layer: L1, Hint: keyhint.Hint{Key: ":", What: "command palette"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "g / G", What: "top / bottom"}},

	{Layer: L2, Hint: keyhint.Hint{What: "Mnemonics", Head: true}},
	{Layer: L2, Run: "n", Cmd: "jj new <rev>", Hint: keyhint.Hint{Key: "n", What: "new"}},
	{Layer: L2, Run: "e", Cmd: "jj edit <rev>", Hint: keyhint.Hint{Key: "e", What: "edit"}},
	{Layer: L2, Run: "d", Cmd: "jj describe -r <rev> -m", Hint: keyhint.Hint{Key: "d", What: "describe"}},
	{Layer: L2, Run: "s", Cmd: "jj squash --from <rev> --into <dest>", Hint: keyhint.Hint{Key: "s", What: "squash"}},
	{Layer: L2, Run: "S", Cmd: "jj split -i -r <rev>", Hint: keyhint.Hint{Key: "S", What: "split"}},
	{Layer: L2, Run: "a", Cmd: "jj abandon <rev>", Hint: keyhint.Hint{Key: "a", What: "abandon"}},
	{Layer: L2, Run: "A", Cmd: "jj absorb -r <rev>", Hint: keyhint.Hint{Key: "A", What: "absorb"}},
	{Layer: L2, Run: "D", Cmd: "jj duplicate <rev>", Hint: keyhint.Hint{Key: "D", What: "duplicate"}},
	{
		Layer: L2, Run: "r", Cmd: "jj rebase -r <rev> --onto <dest>",
		Hint: keyhint.Hint{Key: "r", What: "rebase the revision"},
	},
	{
		Layer: L2, Run: "R", Cmd: "jj rebase -s <rev> --onto <dest>",
		Hint: keyhint.Hint{Key: "R", What: "rebase it and its descendants"},
	},
	{Layer: L2, Run: "b", Cmd: "jj bookmark list", Hint: keyhint.Hint{Key: "b", What: "bookmarks"}},
	{Layer: L2, Run: "o", Cmd: "jj op log", Hint: keyhint.Hint{Key: "o", What: "op log"}},
	{Layer: L2, Run: "E", Cmd: "jj evolog -r <rev>", Hint: keyhint.Hint{Key: "E", What: "evolution of the revision"}},
	{Layer: L2, Run: "u", Cmd: "jj undo", Hint: keyhint.Hint{Key: "u", What: "undo"}},
	{Layer: L2, Run: "ctrl+r", Cmd: "jj op restore <op>", Hint: keyhint.Hint{Key: "Ctrl+r", What: "redo"}},
	{Layer: L2, Run: "ctrl+l", Cmd: "jj log", Hint: keyhint.Hint{Key: "Ctrl+l", What: "reload the log"}},
	{Layer: L2, Run: "@", Hint: keyhint.Hint{Key: "@", What: "jump to working copy"}},
	{Layer: L2, Run: "w", Cmd: "jj workspace list", Hint: keyhint.Hint{Key: "w", What: "workspaces"}},
	{Layer: L2, Run: "p", Hint: keyhint.Hint{Key: "p", What: "forge pane"}},
	{Layer: L2, Run: "L", Hint: keyhint.Hint{Key: "L", What: "edit the revset"}},
	{Layer: L2, Run: "space", Hint: keyhint.Hint{Key: "space", What: "check for a batch action"}},

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

// Runnable returns the bindings the command palette can dispatch, in table order.
func Runnable() []Binding {
	out := make([]Binding, 0, len(Bindings))

	for _, b := range Bindings {
		if b.Run != "" {
			out = append(out, b)
		}
	}

	return out
}
