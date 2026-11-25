package help_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
	"github.com/kyleking/aragonite/tui/keyhint"
	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/palette"

	"github.com/kyleking/jojo/internal/components/help"
)

// TestKeymap_AgreesWithShared is what keeps one key meaning one thing across jojo, jj-diff, and
// second-look: no key jojo binds may be a key the shared table gives another action on a screen the
// two share, and the merged table must have no collision on any screen.
func TestKeymap_AgreesWithShared(t *testing.T) {
	t.Parallel()

	if got := keys.Disagreements(help.Shared(), help.Tool()); len(got) != 0 {
		t.Errorf("jojo's keys disagree with the shared keymap:\n%s", strings.Join(got, "\n"))
	}

	km, err := help.NewKeymap(nil)
	if err != nil {
		t.Fatalf("the default keymap does not merge: %v", err)
	}

	keys.RequireReserved(t, km.Keys())
}

// screens is every screen jojo reports to the matcher, with the name its golden is filed under.
//
//nolint:gochecknoglobals // a fixture
var screens = []struct {
	name string
	ctx  keys.Context
}{
	{"log", keys.Log},
	{"dock", keys.View},
	{"files", keys.List},
	{"outline", help.Outline},
	{"travel", help.Travel},
	{"pick_rebase", keys.Picker | help.PickRebase},
	{"pick_squash", keys.Picker | help.PickSquash},
	{"pick_duplicate", keys.Picker | help.PickDuplicate},
	{"pick_new", keys.Picker | help.PickNew},
	{"pick_restore", keys.Picker | help.PickRestore},
	{"pick_bookmark", keys.Picker | help.PickBookmark},
	{"submit", help.Submit},
}

func defaultKeymap(t *testing.T) help.Keymap {
	t.Helper()

	km, err := help.NewKeymap(nil)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	return km
}

// TestHelp_Golden pins the legend each screen's `?` lists, which the keymap generates, with what the
// palette teaches about each key, so a change to a binding shows up as a reviewed diff of what the
// reader sees.
func TestHelp_Golden(t *testing.T) {
	t.Parallel()

	for _, screen := range screens {
		t.Run(screen.name, func(t *testing.T) {
			t.Parallel()

			km := defaultKeymap(t)
			entries := palette.FromKeymap(km.Keys(), screen.ctx, nil, help.About(km))

			var b strings.Builder

			for _, section := range palette.Groups(entries) {
				fmt.Fprintf(&b, "── %s\n", section.Name)

				for _, hit := range section.Hits {
					fmt.Fprintf(&b, "%-8s %-58s %s\n", hit.Key, hit.What, hit.Cmd)
				}
			}

			golden.RequireEqual(t, []byte(b.String()))
		})
	}
}

// Every action the keymap binds has something to teach in the palette, a git word maps to the jj
// verb a git user means, and an action no key presses is still reachable there.
func TestAbout_TeachesTheVerbsAndTheirGitWords(t *testing.T) {
	t.Parallel()

	km := defaultKeymap(t)
	about := help.About(km)

	for word, action := range map[string]string{
		"amend": help.ActionSquash, "fixup": help.ActionSquash, "stash": help.ActionNew,
		"checkout": help.ActionEdit, "branch": help.ActionBookmarkCreate, "commit": help.ActionCommit,
		"stage": help.ActionSplit, "reset": help.ActionRestore, "revert": keys.ActionUndo,
		"cherry-pick": help.ActionDuplicate, "reflog": help.ActionOpLog, "pull": help.ActionFetch,
	} {
		if !slices.Contains(about[action].Also, word) {
			t.Errorf("%q does not find %s", word, action)
		}
	}

	for _, action := range []string{help.ActionSquash, help.ActionRebase, help.ActionNew, keys.ActionUndo} {
		if about[action].Cmd == "" || len(about[action].Prose) == 0 {
			t.Errorf("%s has no command or prose to teach", action)
		}
	}

	if !strings.Contains(about[help.ActionSquash].Flags, "-i") {
		t.Errorf("squash's flags are %q, want its picker's", about[help.ActionSquash].Flags)
	}

	only := km.PaletteOnly()
	if !slices.ContainsFunc(only, func(b keys.Binding) bool { return b.Action == help.ActionArrange }) {
		t.Errorf("palette-only verbs are %v, want arrange among them", only)
	}
}

// Every bound action shows in the legend of some screen: a key the legend misses is a key nobody
// finds.
func TestHelp_ListsEveryBoundAction(t *testing.T) {
	t.Parallel()

	km := defaultKeymap(t)
	listed := map[string]bool{}

	for _, screen := range screens {
		for _, h := range km.Legend(screen.ctx) {
			listed[h.Key+" "+h.What] = true
		}
	}

	for _, b := range km.Keys().Bindings() {
		if len(b.Keys) > 0 && !listed[b.Labels()+" "+b.What] {
			t.Errorf("%s (%s) is bound but in no legend", b.Action, b.Labels())
		}
	}
}

// A rebind moves the action, and the legend and the footer hint follow it, because both read the
// same table the dispatcher does.
func TestRebindMovesTheActionAndTheLegendTogether(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{help.ActionSquash: "M", help.ActionForge: "g p"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	for action, want := range map[string]string{help.ActionSquash: "M", help.ActionForge: "gp"} {
		if got := km.Label(action); got != want {
			t.Errorf("%s is on %q, want %q", action, got, want)
		}
	}

	legend := km.Legend(keys.Log)
	if !slices.ContainsFunc(legend, func(h keyhint.Hint) bool { return h.Key == "M" }) {
		t.Error("the legend does not show squash on M")
	}

	m := keys.NewMatcher(km.Keys())
	if got := m.Feed(keys.Log, "s"); got.Action != "" {
		t.Errorf("s still runs %q after squash moved", got.Action)
	}
}

// A rebind that cannot be honored refuses rather than silently doing nothing: an unknown action, two
// actions on one key, or a key that starts a sequence, which could never complete.
func TestRebindRefusesWhatItCannotHonor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		overrides map[string]string
		want      error
	}{
		"unknown action":      {map[string]string{"teleport": "T"}, help.ErrUnknownAction},
		"two on one key":      {map[string]string{help.ActionSquash: "r"}, help.ErrDuplicateKey},
		"onto the g prefix":   {map[string]string{help.ActionSquash: "g"}, help.ErrDuplicateKey},
		"onto the b prefix":   {map[string]string{help.ActionFetch: "b"}, help.ErrDuplicateKey},
		"onto a shared key":   {map[string]string{help.ActionFetch: "?"}, help.ErrDuplicateKey},
		"onto the ] prefix":   {map[string]string{help.ActionFetch: "]"}, help.ErrDuplicateKey},
		"flag onto a verb's":  {map[string]string{keys.FlagAction(keys.VerbRebase, "o"): "- s"}, help.ErrDuplicateKey},
		"a sequence clashing": {map[string]string{help.ActionWorkspaces: "g h"}, help.ErrDuplicateKey},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := help.NewKeymap(tc.overrides); !errors.Is(err, tc.want) {
				t.Errorf("NewKeymap(%v) = %v, want %v", tc.overrides, err, tc.want)
			}
		})
	}
}

// A swap is two rebinds that are each a collision alone and fine together.
func TestRebindAllowsASwap(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{help.ActionRebase: "s", help.ActionSquash: "r"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	if km.Label(help.ActionRebase) != "s" || km.Label(help.ActionSquash) != "r" {
		t.Errorf("swap gave rebase %q and squash %q", km.Label(help.ActionRebase), km.Label(help.ActionSquash))
	}
}

// "" unbinds, and an unbound action drops out of the footer rather than drawing a key that does
// nothing.
func TestRebindToNothingUnbinds(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{help.ActionAbsorb: ""})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	if _, ok := km.Hint(help.ActionAbsorb); ok {
		t.Error("an unbound action still has a footer hint")
	}

	if got := keys.NewMatcher(km.Keys()).Feed(keys.Log, "A"); got.Action != "" {
		t.Errorf("A still runs %q", got.Action)
	}
}

// An old action name still rebinds, onto the action that does its job now, and says so; it never
// fails startup.
func TestDeprecatedActionsResolveWithAWarning(t *testing.T) {
	t.Parallel()

	for old, renamed := range help.Deprecated {
		t.Run(old, func(t *testing.T) {
			t.Parallel()

			// A key nothing else uses on any screen jojo has.
			km, err := help.NewKeymap(map[string]string{old: "ctrl+t"})
			if err != nil {
				t.Fatalf("NewKeymap(%s): %v", old, err)
			}

			if got := km.Label(renamed); got != "ctrl+t" {
				t.Errorf("%s did not move %s: it is on %q", old, renamed, got)
			}

			if w := km.Warnings(); len(w) != 1 || !strings.Contains(w[0], old) || !strings.Contains(w[0], renamed) {
				t.Errorf("warnings = %q, want one naming %s and %s", w, old, renamed)
			}
		})
	}
}

// A [keys] rebind of the retired staging key lands on the file list's check, and the warning says what
// replaced staging, since check never writes where stage did.
func TestFileStage_RebindsCheckAndSaysStagingIsRetired(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{"file-stage": "ctrl+t"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	if got := km.Label(keys.ActionCheck); got != "ctrl+t" {
		t.Errorf("check is on %q, want the old file-stage key", got)
	}

	if w := km.Warnings(); len(w) != 1 || !strings.Contains(w[0], "staging is retired") {
		t.Errorf("warnings = %q, want the staging notice", w)
	}
}

// The new name wins over an old one that names the same action, and the old one is reported.
func TestDeprecatedActionLosesToItsNewName(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{"dock-diff": "ctrl+t", help.ActionPreview: "ctrl+g"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	if got := km.Label(help.ActionPreview); got != "ctrl+g" {
		t.Errorf("preview is on %q, want ctrl+g", got)
	}

	if w := km.Warnings(); len(w) != 1 || !strings.Contains(w[0], "ignored") {
		t.Errorf("warnings = %q", w)
	}
}

// The picker's old per-letter toggles are flags spelled as jj spells them now, which are not rebound:
// naming one warns and changes nothing.
func TestOldPickerTogglesWarnAndChangeNothing(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{"pick-onto": "O"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	if got := km.Label(keys.FlagAction(keys.VerbRebase, "o")); got != "-o" {
		t.Errorf("-o moved to %q", got)
	}

	if len(km.Warnings()) != 1 {
		t.Errorf("warnings = %q", km.Warnings())
	}
}

// The keys that moved in the rework land where keymap.md says, so a regression in the table is a
// failing name rather than a golden diff someone waves through.
func TestKeymap_MovedKeys(t *testing.T) {
	t.Parallel()

	km := defaultKeymap(t)
	want := map[string]string{
		keys.ActionTop: "gg", help.ActionDuplicate: "y", keys.ActionDescribeEditor: "D",
		help.ActionResolve: "x", help.ActionConverge: "X", help.ActionArrange: "", keys.ActionRounds: "H",
		keys.ActionInspect: "K", help.ActionCompare: "=", help.ActionPreview: "p", help.ActionForge: "gh",
		help.ActionWorkspaces: "gw", keys.ActionMessages: "gm", keys.ActionShell: "!", help.ActionBookmarks: "bb",
		help.ActionRestore: "R", help.ActionNew: "n", help.ActionNewPlaced: "N", help.ActionCommit: "c",
		help.ActionDiffedit: "E", keys.ActionYank: "Y", help.ActionRebaseSubtree: "",
		keys.FlagAction(keys.VerbSquash, "i"): "-i", keys.FlagAction(keys.VerbRebase, "b"): "-b",
		keys.FlagAction(keys.VerbRebase, "B"): "-B",
	}

	for action, label := range want {
		if got := km.Label(action); got != label {
			t.Errorf("%s is on %q, want %q", action, got, label)
		}
	}

	for _, gone := range []string{"alt+i", "alt+d", "alt+p", "alt+e", "i"} {
		if got := keys.NewMatcher(km.Keys()).Feed(keys.Log, gone); got.Action != "" {
			t.Errorf("%s still runs %s from the log", gone, got.Action)
		}
	}
}
