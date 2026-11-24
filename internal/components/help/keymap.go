package help

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
)

// ErrUnknownAction marks a rebind naming an action the keymap does not have.
var ErrUnknownAction = keys.ErrUnknownAction

// ErrDuplicateKey marks a rebind putting two actions on one key on one screen, or onto a key that
// starts a sequence (g, b, ], [, \, -), which could then never complete.
var ErrDuplicateKey = keys.ErrDuplicateKey

// Deprecated maps an action name the keymap used to have to the one that does its job now. A [keys]
// entry naming an old action still rebinds, onto the new action, with a warning in the footer, for
// one minor release; then the old names go.
//
//nolint:gochecknoglobals // a fixed table, read only.
var Deprecated = map[string]string{
	"abandon-divergent":   ActionConverge,
	"last-failure":        keys.ActionMessages,
	"diff-range":          ActionCompare,
	"dock-diff":           ActionPreview,
	"move-hunks":          ActionSquash,
	"squash-interactive":  ActionSquash,
	"evolog":              keys.ActionRounds,
	"detail":              keys.ActionInspect,
	"round-older":         keys.PrevAction(keys.ObjRound),
	"round-newer":         keys.NextAction(keys.ObjRound),
	"chapter-hunk-next":   keys.NextAction(keys.ObjHunk),
	"chapter-hunk-prev":   keys.PrevAction(keys.ObjHunk),
	"chapter-round-newer": keys.NextAction(keys.ObjRound),
	"chapter-round-older": keys.PrevAction(keys.ObjRound),
	"chapter-rounds":      keys.ActionRounds,
	"outline-close":       keys.ActionBack,
	"pick-cancel":         keys.ActionBack,
	"travel-exit":         keys.ActionBack,
	"pick-here":           ActionPickClear,
}

// deprecatedFlags are the picker's old per-letter toggles, which are flags spelled as jj spells them
// now (- then the letter). A rebind of one is dropped with a warning, since a flag's key is its jj
// spelling and is not rebound.
//
//nolint:gochecknoglobals // a fixed table, read only.
var deprecatedFlags = map[string]string{
	"pick-onto": "-o", "pick-after": "-A", "pick-before": "-B", "pick-revisions": "-r",
	"pick-source": "-s", "pick-branch": "-b", "pick-emptied": "-e or -k", "pick-message": "-u",
	"pick-no-edit": "-n",
}

// Keymap is the effective keymap: aragonite's shared table and jojo's verbs, merged, with the user's
// [keys] rebinds applied. Build it once and hand it to whatever dispatches or draws a key.
type Keymap struct {
	km    *keys.Keymap
	level func(key string) int
	// warnings are what the footer says once about a [keys] table using old names.
	warnings []string
}

// NewKeymap merges the tables and applies overrides, keyed by action ("describe" = "M", or a
// sequence such as "g d"; "" unbinds). An unknown action or a collision is an error, because a rebind
// that silently does nothing is worse than one that refuses. An action renamed by the keymap rework
// still resolves through Deprecated and is reported by Warnings.
func NewKeymap(overrides map[string]string) (Keymap, error) {
	resolved, warnings := resolveDeprecated(overrides)

	km, err := keys.Merge(sharedBindings(), toolBindings(), resolved)
	if err != nil {
		return Keymap{}, fmt.Errorf("merging the keymap: %w", err)
	}

	return Keymap{km: km, warnings: warnings}, nil
}

// resolveDeprecated renames each old action to its new one. A new name the user also set wins over
// the old one, since it is the one they wrote most recently.
func resolveDeprecated(overrides map[string]string) (map[string]string, []string) {
	out := make(map[string]string, len(overrides))

	var warnings []string

	for _, action := range slices.Sorted(maps.Keys(overrides)) {
		seq := overrides[action]

		if flag, ok := deprecatedFlags[action]; ok {
			warnings = append(warnings,
				fmt.Sprintf("[keys] %s is a picker flag now (%s) and is not rebound", action, flag))

			continue
		}

		renamed, ok := Deprecated[action]
		if !ok {
			out[action] = seq

			continue
		}

		if _, set := overrides[renamed]; set {
			warnings = append(warnings, fmt.Sprintf("[keys] %s is %s now, which you also set; %s is ignored",
				action, renamed, action))

			continue
		}

		warnings = append(warnings, fmt.Sprintf("[keys] %s is %s now; rename it", action, renamed))
		out[renamed] = seq
	}

	sort.Strings(warnings)

	return out, warnings
}

// Warnings is what the [keys] table said in names the keymap no longer uses, for the footer to say
// once.
func (k Keymap) Warnings() []string { return slices.Clone(k.warnings) }

// Keys is the merged keymap, for a Matcher and the legend.
func (k Keymap) Keys() *keys.Keymap {
	if k.km == nil {
		km, err := NewKeymap(nil)
		if err != nil {
			panic(err) // the default table is checked by TestKeymap_AgreesWithShared
		}

		return km.km
	}

	return k.km
}

// WithLevel grades every hint that stands for one key by how long level says that key has gone
// unpressed. Level takes the key the way the usage log records it: a sequence's keys joined by
// spaces.
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

// Binding is action's binding as the user has it.
func (k Keymap) Binding(action string) (keys.Binding, bool) { return k.Keys().Binding(action) }

// Hint is action's footer hint, graded by practice, false when the action is unbound.
func (k Keymap) Hint(action string) (keyhint.Hint, bool) {
	b, ok := k.Binding(action)
	if !ok || len(b.Keys) == 0 {
		return keyhint.Hint{}, false
	}

	h := b.Hint()
	h.Fresh = k.Level(b.Keys[0])

	return h, true
}

// Hints is the footer hints for actions in order, skipping any the user unbound.
func (k Keymap) Hints(actions ...string) []keyhint.Hint {
	out := make([]keyhint.Hint, 0, len(actions))

	for _, a := range actions {
		if h, ok := k.Hint(a); ok {
			out = append(out, h)
		}
	}

	return out
}

// HintsAt is Hints with every hint gate refuses marked Off with its reason, so a footer draws it
// flat rather than offering a key that would do nothing.
func (k Keymap) HintsAt(gate keys.Gate, actions ...string) []keyhint.Hint {
	out := make([]keyhint.Hint, 0, len(actions))

	for _, a := range actions {
		h, ok := k.Hint(a)
		if !ok {
			continue
		}

		if av := gate(a); !av.OK {
			h.Off, h.Why = true, av.Why
		}

		out = append(out, h)
	}

	return out
}

// Label is action's first key as a reader types it ("gg", "]o"), "" when unbound.
func (k Keymap) Label(action string) string {
	b, ok := k.Binding(action)
	if !ok {
		return ""
	}

	return b.Label()
}

// Legend is the help for one screen, grouped as aragonite groups it.
func (k Keymap) Legend(ctx keys.Context) []keyhint.Hint {
	return k.Keys().Legend(ctx)
}

// paletteSkips are the log's bindings the palette leaves out of its palette-only rows: movement, and
// the keys that only make sense typed (help, the palette itself, backing out).
//
//nolint:gochecknoglobals // a fixed table, read only.
var paletteSkips = []string{
	keys.ActionHelp, keys.ActionPalette, keys.ActionBack, keys.ActionCancel, keys.ActionQuit,
	keys.ActionLineDown, keys.ActionLineUp, keys.ActionHalfPageDown, keys.ActionHalfPageUp,
	keys.ActionTop, keys.ActionBottom, keys.ActionSwitchPane, ActionCheck,
}

// Bound is every binding with a key on screen ctx, for the usage log.
func (k Keymap) Bound(ctx keys.Context) []keys.Binding { return k.Keys().Bound(ctx) }
