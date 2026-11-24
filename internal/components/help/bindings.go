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
	// Mode is where the key is live: "" for the log, or one of the modes the log enters (ModePick,
	// ModeTravel), whose keys are read only while it lasts. Two modes may give one key different
	// actions; one mode may not give it two.
	Mode string
	keyhint.Hint
	Layer Layer
}

// The modes the log enters, each with keys of its own that mean nothing outside it.
const (
	// ModePick is the in-log destination picker that rebase, squash, duplicate, and new open.
	ModePick = "pick"
	// ModeTravel is the log as it stood at an earlier operation.
	ModeTravel = "travel"
	// ModeOutline is the stack outline, which takes the log's place while it is open.
	ModeOutline = "outline"
)

// The labels of the keys every mode shares, which the legend spells the same way everywhere.
const (
	enterLabel = "Enter"
	escLabel   = "Esc"
	tabLabel   = "Tab"
)

// The keys several modes bind the same way, as the dispatcher spells them, and the row every list's
// legend opens with.
const (
	enterKey = "enter"
	escKey   = "esc"
	moveRows = "j / k"
)

// spaceKey checks a revision in the log and stages a file in the file list of @.
const spaceKey = "space"

// defaultBindings is the whole keymap, as data. The statusbar and the root model both read from
// here, so a binding cannot drift between the footer, the help overlay, and the handler that acts
// on it. Never hand it out: NewKeymap clones it, because a rebind rewrites its entries.
var defaultBindings = []Binding{
	{Layer: L0, Hint: keyhint.Hint{What: "Universal", Head: true}},
	{Layer: L0, Hint: keyhint.Hint{Key: "↑ ↓ ← →", What: "move"}},
	{Layer: L0, Hint: keyhint.Hint{Key: enterLabel, What: "select"}},
	{Layer: L0, Hint: keyhint.Hint{Key: escLabel, What: "back"}},
	{Layer: L0, Hint: keyhint.Hint{Key: "q", What: "quit"}},
	{Layer: L0, Hint: keyhint.Hint{Key: tabLabel, What: "switch pane"}},

	{Layer: L1, Hint: keyhint.Hint{What: "Vim motions", Head: true}},
	{Layer: L1, Hint: keyhint.Hint{Key: moveRows, What: "move a line"}},
	{Layer: L1, Run: "/", Action: "search", Hint: keyhint.Hint{Key: "/", What: "search the log"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "?", What: "help"}},
	{Layer: L1, Hint: keyhint.Hint{Key: ":", What: "command palette"}},
	{Layer: L1, Hint: keyhint.Hint{Key: "g / G", What: "top / bottom"}},

	{Layer: L2, Hint: keyhint.Hint{What: "Mnemonics", Head: true}},
	{Layer: L2, Run: "n", Action: "new", Cmd: "jj new <rev>", Hint: keyhint.Hint{Key: "n", What: "new"}},
	{Layer: L2, Run: "e", Action: "edit", Cmd: "jj edit <rev>", Hint: keyhint.Hint{Key: "e", What: "edit"}},
	{
		Layer: L2, Run: "d", Action: "describe", Cmd: "jj describe -r <rev> -m <full description>",
		Hint: keyhint.Hint{Key: "d", What: "describe"},
	},
	{
		Layer: L2, Run: "alt+e", Action: "describe-editor", Cmd: "jj describe -r <rev>",
		Hint: keyhint.Hint{Key: "Alt+e", What: "describe in $EDITOR"},
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
		Layer: L2, Run: "alt+i", Action: "move-hunks", Cmd: "jj squash -i --from <rev> --into <dest>",
		Hint: keyhint.Hint{Key: "Alt+i", What: "move hunks into any revision"},
	},
	{
		Layer: L2, Run: enterKey, Action: "diff", Cmd: "jj diff -r <rev>",
		Hint: keyhint.Hint{Key: enterLabel, What: "diff the revision"},
	},
	{
		Layer: L2, Run: "alt+d", Action: "diff-range", Cmd: "jj diff --from <a> --to <b>",
		Hint: keyhint.Hint{Key: "Alt+d", What: "diff any two revisions"},
	},
	{
		Layer: L2, Run: "alt+p", Action: "dock-diff", Cmd: "jj diff -r <rev> --git",
		Hint: keyhint.Hint{Key: "Alt+p", What: "diff docked beside the log"},
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
	{
		Layer: L2, Run: "o", Action: "op-log", Cmd: "jj op log",
		Hint: keyhint.Hint{Key: "o", What: "op log, with the log as it stood at each operation"},
	},
	{
		Layer: L2, Run: "[o", Action: "op-older", Cmd: "jj log --at-op <op> --ignore-working-copy",
		Hint: keyhint.Hint{Key: "[o", What: "the log one operation back, read-only"},
	},
	{
		Layer: L2, Run: "]o", Action: "op-newer", Cmd: "jj log --at-op <op> --ignore-working-copy",
		Hint: keyhint.Hint{Key: "]o", What: "the log one operation forward"},
	},
	{
		Layer: L2, Run: "v", Action: "detail", Cmd: "jj log -r <rev> -T ...",
		Hint: keyhint.Hint{Key: "v", What: "details of the revision"},
	},
	{
		Layer: L2, Run: "E", Action: "evolog", Cmd: "jj evolog -r <rev>",
		Hint: keyhint.Hint{Key: "E", What: "evolution of the revision"},
	},
	{
		Layer: L2, Run: "I", Action: "outline", Cmd: "jj log -r 'trunk()..<rev> | <rev>:: ~ ::trunk()'",
		Hint: keyhint.Hint{Key: "I", What: "stack outline: review the stack chapter by chapter"},
	},
	{
		Layer: L2, Run: "H", Action: "rounds", Cmd: "jj evolog -r <rev>",
		Hint: keyhint.Hint{Key: "H", What: "rounds: the versions of the change, to show one in the dock"},
	},
	{
		Layer: L2, Run: "[r", Action: "round-older", Cmd: "jj interdiff --from <previous> --to <version>",
		Hint: keyhint.Hint{Key: "[r", What: "dock the change's previous version, as an interdiff"},
	},
	{
		Layer: L2, Run: "]r", Action: "round-newer", Cmd: "jj interdiff --from <previous> --to <version>",
		Hint: keyhint.Hint{Key: "]r", What: "dock the next version, back to the whole change after the newest"},
	},
	{Layer: L2, Run: "u", Action: "undo", Cmd: "jj undo", Hint: keyhint.Hint{Key: "u", What: "undo"}},
	{
		Layer: L2, Run: "ctrl+r", Action: "redo", Cmd: "jj redo",
		Hint: keyhint.Hint{Key: "Ctrl+r", What: "redo"},
	},
	{
		Layer: L2, Run: "!", Action: "last-failure",
		Hint: keyhint.Hint{Key: "!", What: "full text of the last failure"},
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
	{Layer: L2, Run: spaceKey, Action: "check", Hint: keyhint.Hint{Key: spaceKey, What: "check for a batch action"}},

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
	{
		Layer: L3, Run: "x", Action: "abandon-divergent", Cmd: "jj converge [-r change_id(<rev>)]",
		Hint: keyhint.Hint{Key: "x", What: "converge divergent changes"},
	},
	{Layer: L3, Hint: keyhint.Hint{What: "edit the revset aliases"}},

	// The staging keys live in the file list of @ (l on the working copy) rather than the log, so they
	// carry no Run: the legend teaches them and the list dispatches them.
	{Layer: L3, Hint: keyhint.Hint{What: "Staging, in the files of @", Head: true}},
	{
		Layer: L3, Cmd: "jj squash --from @ --into @- <path>",
		Hint: keyhint.Hint{Key: spaceKey, What: "stage or unstage the file"},
	},
	{
		Layer: L3, Cmd: "jj describe @- && jj new -A @- -B @",
		Hint: keyhint.Hint{Key: "C", What: "commit what is staged"},
	},

	// The docked diff's own keys, read while it has focus (Tab), and in the outline beside it.
	{Layer: L2, Hint: keyhint.Hint{What: "In the docked diff", Head: true}},
	{Layer: L2, Hint: keyhint.Hint{Key: "]h / [h", What: "next / previous hunk"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "]f / [f", What: "next / previous file"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "n / N", What: "repeat the last ] or [ motion forward / back"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "]r / [r", What: "next / previous version of the change (rounds)"}},
	{Layer: L2, Hint: keyhint.Hint{Key: "]] / [[", What: "next / previous chapter, while the outline is open"}},

	// The picker's keys toggle the flags of the command it is building. The provisional letters follow
	// jjui's rebase mode, and each carries an Action so the keymap pass can move them in config.
	{Layer: L2, Mode: ModePick, Hint: keyhint.Hint{What: "Picking where it goes (r R s D n)", Head: true}},
	{
		Layer: L2, Mode: ModePick,
		Hint: keyhint.Hint{Key: moveRows, What: "move the destination; the graph redraws as it will be"},
	},
	{
		Layer: L2, Mode: ModePick, Run: enterKey, Action: "pick-apply",
		Hint: keyhint.Hint{Key: enterLabel, What: "run it"},
	},
	{
		Layer: L2, Mode: ModePick, Run: escKey, Action: "pick-cancel",
		Hint: keyhint.Hint{Key: escLabel, What: "cancel, nothing ran"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "tab", Action: "pick-compare",
		Hint: keyhint.Hint{Key: tabLabel, What: "show the graph before / after the command"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "o", Action: "pick-onto", Cmd: "--onto",
		Hint: keyhint.Hint{Key: "o", What: "onto"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "a", Action: "pick-after", Cmd: "--insert-after",
		Hint: keyhint.Hint{Key: "a", What: "after"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "b", Action: "pick-before", Cmd: "--insert-before",
		Hint: keyhint.Hint{Key: "b", What: "before"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "h", Action: "pick-here", Cmd: "jj duplicate <rev>",
		Hint: keyhint.Hint{Key: "h", What: "duplicate in place"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "r", Action: "pick-revisions", Cmd: "jj rebase -r",
		Hint: keyhint.Hint{Key: "r", What: "move the revisions alone"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "s", Action: "pick-source", Cmd: "jj rebase -s",
		Hint: keyhint.Hint{Key: "s", What: "move them with descendants"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "B", Action: "pick-branch", Cmd: "jj rebase -b",
		Hint: keyhint.Hint{Key: "B", What: "move the whole branch"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "e", Action: "pick-emptied", Cmd: "--skip-emptied / --keep-emptied",
		Hint: keyhint.Hint{Key: "e", What: "skip (rebase) or keep (squash) emptied revisions"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "m", Action: "pick-message", Cmd: "--use-destination-message",
		Hint: keyhint.Hint{Key: "m", What: "squash: keep the destination's message, or combine in $EDITOR"},
	},
	{
		Layer: L2, Mode: ModePick, Run: "w", Action: "pick-no-edit", Cmd: "jj new --no-edit",
		Hint: keyhint.Hint{Key: "w", What: "new: leave @ where it is"},
	},

	// The outline takes the log's place, with the docked diff following the chapter under the cursor.
	{Layer: L2, Mode: ModeOutline, Hint: keyhint.Hint{What: "Stack outline (I)", Head: true}},
	{
		Layer: L2, Mode: ModeOutline,
		Hint: keyhint.Hint{Key: moveRows, What: "move a chapter; the docked diff follows"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "]]", Action: "chapter-next",
		Hint: keyhint.Hint{Key: "]]", What: "chapter"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "[[", Action: "chapter-prev",
		Hint: keyhint.Hint{Key: "[[", What: "back a chapter"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: enterKey, Action: "chapter-read",
		Hint: keyhint.Hint{Key: enterLabel, What: "read it in the dock"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "]h", Action: "chapter-hunk-next",
		Hint: keyhint.Hint{Key: "]h", What: "hunk"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "[h", Action: "chapter-hunk-prev",
		Hint: keyhint.Hint{Key: "[h", What: "back a hunk"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "]r", Action: "chapter-round-newer", Cmd: "jj interdiff",
		Hint: keyhint.Hint{Key: "]r", What: "newer version"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "[r", Action: "chapter-round-older", Cmd: "jj interdiff",
		Hint: keyhint.Hint{Key: "[r", What: "older version"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: "H", Action: "chapter-rounds", Cmd: "jj evolog",
		Hint: keyhint.Hint{Key: "H", What: "rounds"},
	},
	{
		Layer: L2, Mode: ModeOutline, Run: escKey, Action: "outline-close",
		Hint: keyhint.Hint{Key: escLabel, What: "log"},
	},

	// Time travel is read-only: these are the only keys it answers, and a write verb says why not.
	{Layer: L2, Mode: ModeTravel, Hint: keyhint.Hint{What: "Viewing an earlier operation ([o o)", Head: true}},
	{
		Layer: L2, Mode: ModeTravel, Run: "r", Action: "travel-restore", Cmd: "jj op restore <op>",
		Hint: keyhint.Hint{Key: "r", What: "restore the repository to it"},
	},
	{
		Layer: L2, Mode: ModeTravel, Run: escKey, Action: "travel-exit",
		Hint: keyhint.Hint{Key: escLabel, What: "back to now"},
	},
}

// Keymap is the effective keymap: the default table with the user's rebinds applied. Build it once
// and pass it to whatever renders or dispatches a key, so the legend, the palette, the footer, and
// the handlers can never disagree about which key runs what.
type Keymap struct {
	// toDefault is one translation per mode, "" being the log's.
	toDefault map[string]map[string]string
	level     func(key string) int
	bindings  []Binding
}

// fixedKeys are the hints whose key the dispatcher handles by name rather than through a binding.
var fixedKeys = map[string]string{"q": "q", "?": "?", ":": ":", "Tab": "tab"}

// WithLevel grades every hint that stands for one key by how long level says that key has gone
// unpressed. Level takes the key as msg.String() spells it.
func (k Keymap) WithLevel(level func(key string) int) Keymap {
	k.level = level

	return k
}

// Level is how unpracticed key is, 0 when nothing is known about it.
func (k Keymap) Level(key string) int {
	if k.level == nil {
		return 0
	}

	return k.level(key)
}

func (k Keymap) hint(b *Binding) keyhint.Hint {
	h := b.Hint

	switch key := fixedKeys[h.Key]; {
	case b.Run != "":
		h.Fresh = k.Level(b.Run)
	case key != "":
		h.Fresh = k.Level(key)
	}

	return h
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

	for i := range bindings {
		if bindings[i].Action != "" {
			actions[bindings[i].Action] = i
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

// translation maps each effective key back to the key the handlers switch on, one map per mode,
// which is what makes a rebind one lookup at the door rather than a rename through every switch. A
// key whose action moved away maps to unbound, so a rebind moves a key rather than adding one.
func translation(bindings []Binding) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}

	for _, mode := range modes(bindings) {
		one, err := modeTranslation(bindings, mode)
		if err != nil {
			return nil, err
		}

		out[mode] = one
	}

	return out, nil
}

func modes(bindings []Binding) []string {
	var out []string

	for i := range bindings {
		if !slices.Contains(out, bindings[i].Mode) {
			out = append(out, bindings[i].Mode)
		}
	}

	return out
}

func modeTranslation(bindings []Binding, mode string) (map[string]string, error) {
	fromDefault := map[string]string{}

	for i := range defaultBindings {
		b := &defaultBindings[i]
		if b.Action != "" && b.Mode == mode {
			fromDefault[b.Action] = b.Run
		}
	}

	held := map[string]string{}

	for i := range bindings {
		b := &bindings[i]
		if b.Action == "" || b.Mode != mode {
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

// Resolve turns a key pressed in the log into the key the handlers know. A key nothing rebinds is
// its own answer, so the common case costs one map miss.
func (k Keymap) Resolve(pressed string) string { return k.ResolveIn("", pressed) }

// ResolveIn is Resolve for a key pressed while the log is in mode.
func (k Keymap) ResolveIn(mode, pressed string) string {
	if key, ok := k.toDefault[mode][pressed]; ok {
		return key
	}

	return pressed
}

// ModeHints is the hint of each key live in mode, keyed by its action, as the user has bound it.
func (k Keymap) ModeHints(mode string) map[string]keyhint.Hint {
	out := map[string]keyhint.Hint{}

	bindings := k.ModeBindings(mode)
	for i := range bindings {
		out[bindings[i].Action] = k.hint(&bindings[i])
	}

	return out
}

// ModeBindings is the bindings live in mode, in table order.
func (k Keymap) ModeBindings(mode string) []Binding {
	var out []Binding

	for i := range k.bindings {
		if b := k.bindings[i]; b.Mode == mode && b.Run != "" {
			out = append(out, b)
		}
	}

	return out
}

// Hints returns the actionable hints (headings excluded) for the given
// layers, in table order. No layers returns every layer.
func (k Keymap) Hints(layers ...Layer) []keyhint.Hint {
	out := make([]keyhint.Hint, 0, len(k.bindings))

	for i := range k.bindings {
		b := &k.bindings[i]
		if b.Head || b.Mode != "" {
			continue
		}

		if len(layers) > 0 && !slices.Contains(layers, b.Layer) {
			continue
		}

		out = append(out, k.hint(b))
	}

	return out
}

// AllHints returns every entry, headings included, in the shape the legend
// renders.
func (k Keymap) AllHints() []keyhint.Hint {
	out := make([]keyhint.Hint, len(k.bindings))
	for i := range k.bindings {
		out[i] = k.hint(&k.bindings[i])
	}

	return out
}

// Runnable returns the bindings the command palette can dispatch from the log, in table order.
func (k Keymap) Runnable() []Binding {
	out := make([]Binding, 0, len(k.bindings))

	for i := range k.bindings {
		if k.bindings[i].Run != "" && k.bindings[i].Mode == "" {
			out = append(out, k.bindings[i])
		}
	}

	return out
}
