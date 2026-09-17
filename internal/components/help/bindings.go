// Package help renders the `?` overlay: the full key legend, grouped by the
// four layers jojo's keymap is designed in, scrollable when the legend is
// taller than the frame.
package help

import (
	"errors"
	"fmt"
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
	// Action names the binding in config.toml's [keys] table. It is stable across a rebind, which
	// Run and Key are not.
	Action string
	Run    string
	keyhint.Hint
	Layer Layer
}

// defaultBindings is the whole keymap, as data. The statusbar and the root model both read from
// here, so a binding cannot drift between the footer, the help overlay, and the handler that acts
// on it. Never hand it out: NewKeymap clones it, because a rebind rewrites its entries.
var defaultBindings = []Binding{
	{Layer: L0, Hint: keyhint.Hint{What: "Universal", Head: true}},
	{Layer: L0, Hint: keyhint.Hint{Key: "↑ ↓ ← →", What: "move"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Enter", What: "select"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Esc", What: "back"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "q", What: "quit"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "Tab", What: "switch pane"}},

	{Layer: L1, Hint: keyhint.Hint{What: "Vim motions", Head: true}},
	{Layer: L1, Hint: keyhint.Hint{Key: "j / k", What: "move a line"}},
	{Layer: L1, Run: "/", Action: "search", Hint: keyhint.Hint{Key: "/", What: "search the log"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "?", What: "help"}},
	{Layer: L1, Hint: keyhint.Hint{Key: ":", What: "command palette"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "g / G", What: "top / bottom"}},

	{Layer: L2, Hint: keyhint.Hint{What: "Mnemonics", Head: true}},
	{Layer: L2, Run: "n", Action: "new", Cmd: "jj new <rev>", Hint: keyhint.Hint{Key: "n", What: "new"}},
	{Layer: L2, Run: "e", Action: "edit", Cmd: "jj edit <rev>", Hint: keyhint.Hint{Key: "e", What: "edit"}},
	{
		Layer: L2, Run: "d", Action: "describe", Cmd: "jj describe -r <rev> -m",
		Hint: keyhint.Hint{Key: "d", What: "describe"},
	},
	{
		Layer: L2, Run: "s", Action: "squash", Cmd: "jj squash --from <rev> --into <dest>",
		Hint: keyhint.Hint{Key: "s", What: "squash"},
	},
	{Layer: L2, Run: "S", Action: "split", Cmd: "jj split -i -r <rev>", Hint: keyhint.Hint{Key: "S", What: "split"}},
	{
		Layer: L2, Run: "i", Action: "squash-interactive", Cmd: "jj squash -i -r <rev>",
		Hint: keyhint.Hint{Key: "i", What: "squash interactively"},
	},
	{
		Layer: L2, Run: "I", Action: "move-hunks", Cmd: "jj squash -i --from <rev> --into <dest>",
		Hint: keyhint.Hint{Key: "I", What: "move hunks into any revision"},
	},
	{
		Layer: L2, Run: "enter", Action: "diff", Cmd: "jj diff -r <rev>",
		Hint: keyhint.Hint{Key: "Enter", What: "diff the revision"},
	},
	{
		Layer: L2, Run: "alt+d", Action: "diff-range", Cmd: "jj diff --from <a> --to <b>",
		Hint: keyhint.Hint{Key: "Alt+d", What: "diff any two revisions"},
	},
	{Layer: L2, Run: "a", Action: "abandon", Cmd: "jj abandon <rev>", Hint: keyhint.Hint{Key: "a", What: "abandon"}},
	{Layer: L2, Run: "A", Action: "absorb", Cmd: "jj absorb -r <rev>", Hint: keyhint.Hint{Key: "A", What: "absorb"}},
	{
		Layer: L2, Run: "D", Action: "duplicate", Cmd: "jj duplicate <rev>",
		Hint: keyhint.Hint{Key: "D", What: "duplicate"},
	},
	{
		Layer: L2, Run: "r", Action: "rebase", Cmd: "jj rebase -r <rev> --onto <dest>",
		Hint: keyhint.Hint{Key: "r", What: "rebase the revision"},
	},
	{
		Layer: L2, Run: "R", Action: "rebase-subtree", Cmd: "jj rebase -s <rev> --onto <dest>",
		Hint: keyhint.Hint{Key: "R", What: "rebase it and its descendants"},
	},
	{
		Layer: L2, Run: "l", Action: "files", Cmd: "jj diff -r <rev> --summary",
		Hint: keyhint.Hint{Key: "l", What: "files in the revision"},
	},
	{
		Layer: L2, Run: "c", Action: "resolve", Cmd: "jj resolve -r <rev>",
		Hint: keyhint.Hint{Key: "c", What: "resolve the conflict"},
	},
	{
		Layer: L2, Run: "b", Action: "bookmarks", Cmd: "jj bookmark list",
		Hint: keyhint.Hint{Key: "b", What: "bookmarks"},
	},
	{
		Layer: L2, Run: "f", Action: "fetch", Cmd: "jj git fetch --all-remotes",
		Hint: keyhint.Hint{Key: "f", What: "fetch"},
	},
	{
		Layer: L2, Run: "P", Action: "push", Cmd: "jj git push -r <rev>",
		Hint: keyhint.Hint{Key: "P", What: "push the revision's bookmarks"},
	},
	{Layer: L2, Run: "o", Action: "op-log", Cmd: "jj op log", Hint: keyhint.Hint{Key: "o", What: "op log"}},
	{
		Layer: L2, Run: "v", Action: "detail", Cmd: "jj log -r <rev> -T ...",
		Hint: keyhint.Hint{Key: "v", What: "details of the revision"},
	},
	{
		Layer: L2, Run: "E", Action: "evolog", Cmd: "jj evolog -r <rev>",
		Hint: keyhint.Hint{Key: "E", What: "evolution of the revision"},
	},
	{Layer: L2, Run: "u", Action: "undo", Cmd: "jj undo", Hint: keyhint.Hint{Key: "u", What: "undo"}},
	{
		Layer: L2, Run: "ctrl+r", Action: "redo", Cmd: "jj op restore <op>",
		Hint: keyhint.Hint{Key: "Ctrl+r", What: "redo"},
	},
	{
		Layer: L2, Run: "ctrl+l", Action: "reload", Cmd: "jj log",
		Hint: keyhint.Hint{Key: "Ctrl+l", What: "reload the log"},
	},
	{Layer: L2, Run: "@", Action: "working-copy", Hint: keyhint.Hint{Key: "@", What: "jump to working copy"}},
	{
		Layer: L2, Run: "w", Action: "workspaces", Cmd: "jj workspace list",
		Hint: keyhint.Hint{Key: "w", What: "workspaces"},
	},
	{Layer: L2, Run: "p", Action: "forge", Hint: keyhint.Hint{Key: "p", What: "forge pane"}},
	{Layer: L2, Run: "L", Action: "revset", Hint: keyhint.Hint{Key: "L", What: "edit the revset"}},
	{Layer: L2, Run: "space", Action: "check", Hint: keyhint.Hint{Key: "space", What: "check for a batch action"}},

	{Layer: L3, Hint: keyhint.Hint{What: "Power", Head: true}},
	{
		Layer: L3, Run: "|", Action: "parallelize", Cmd: "jj parallelize <revs>",
		Hint: keyhint.Hint{Key: "|", What: "parallelize the checked revisions"},
	},
	{
		Layer: L3, Run: "X", Action: "arrange", Cmd: "jj arrange",
		Hint: keyhint.Hint{Key: "X", What: "arrange the graph"},
	},
	{
		Layer: L3, Run: "B", Action: "bisect", Cmd: "jj bisect run --range <a>::<b>",
		Hint: keyhint.Hint{Key: "B", What: "bisect between the two checked revisions"},
	},
	{Layer: L3, Hint: keyhint.Hint{What: "edit the revset aliases"}},
}

// Keymap is the effective keymap: the default table with the user's rebinds applied. Build it once
// and pass it to whatever renders or dispatches a key, so the legend, the palette, the footer, and
// the handlers can never disagree about which key runs what.
type Keymap struct {
	toDefault map[string]string
	bindings  []Binding
}

// ErrUnknownAction marks a rebind naming an action the keymap does not have.
var ErrUnknownAction = errors.New("unknown action")

// ErrDuplicateKey marks a rebind putting two actions on one key.
var ErrDuplicateKey = errors.New("two actions on one key")

// NewKeymap applies overrides, keyed by Binding.Action, to the default table. An unknown action or a
// collision is an error, because a rebind that silently does nothing is worse than one that refuses.
func NewKeymap(overrides map[string]string) (Keymap, error) {
	bindings := slices.Clone(defaultBindings)
	actions := map[string]int{}

	for i, b := range bindings {
		if b.Action != "" {
			actions[b.Action] = i
		}
	}

	for action, key := range overrides {
		i, ok := actions[action]
		if !ok {
			return Keymap{}, fmt.Errorf("%w: %q", ErrUnknownAction, action)
		}

		bindings[i].Run, bindings[i].Key = key, key
	}

	toDefault, err := translation(bindings)
	if err != nil {
		return Keymap{}, err
	}

	return Keymap{bindings: bindings, toDefault: toDefault}, nil
}

// unbound is what a rebound action's old key resolves to: a byte no terminal sends, so every switch
// falls through and the key does nothing.
const unbound = "\x00"

// translation maps each effective key back to the key the handlers switch on, which is what makes a
// rebind one lookup at the door rather than a rename through every switch. A key whose action moved
// away maps to unbound, so a rebind moves a key rather than adding one.
func translation(bindings []Binding) (map[string]string, error) {
	fromDefault := map[string]string{}

	for _, b := range defaultBindings {
		if b.Action != "" {
			fromDefault[b.Action] = b.Run
		}
	}

	held := map[string]string{}

	for _, b := range bindings {
		if b.Action == "" {
			continue
		}

		if other, taken := held[b.Run]; taken {
			return nil, fmt.Errorf("%w: %q runs both %s and %s", ErrDuplicateKey, b.Run, other, b.Action)
		}

		held[b.Run] = b.Action
	}

	out := map[string]string{}

	for key, action := range held {
		if def := fromDefault[action]; def != key {
			out[key] = def
		}
	}

	for _, def := range fromDefault {
		if _, stillHeld := held[def]; !stillHeld {
			out[def] = unbound
		}
	}

	return out, nil
}

// Resolve turns a pressed key into the key the handlers know. A key nothing rebinds is its own
// answer, so the common case costs one map miss.
func (k Keymap) Resolve(pressed string) string {
	if key, ok := k.toDefault[pressed]; ok {
		return key
	}

	return pressed
}

// Hints returns the actionable hints (headings excluded) for the given
// layers, in table order. No layers returns every layer.
func (k Keymap) Hints(layers ...Layer) []keyhint.Hint {
	out := make([]keyhint.Hint, 0, len(k.bindings))

	for _, b := range k.bindings {
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

// AllHints returns every entry, headings included, in the shape the legend
// renders.
func (k Keymap) AllHints() []keyhint.Hint {
	out := make([]keyhint.Hint, len(k.bindings))
	for i, b := range k.bindings {
		out[i] = b.Hint
	}

	return out
}

// Runnable returns the bindings the command palette can dispatch, in table order.
func (k Keymap) Runnable() []Binding {
	out := make([]Binding, 0, len(k.bindings))

	for _, b := range k.bindings {
		if b.Run != "" {
			out = append(out, b)
		}
	}

	return out
}
