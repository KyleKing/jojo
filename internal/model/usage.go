package model

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/keyusage"
	"github.com/kyleking/aragonite/tui/keys"

	"github.com/kyleking/jojo/internal/components/help"
)

// UsageApp names jojo's key usage log.
const UsageApp = "jojo"

// StaleAfter is how long a key may go unpressed before `jojo keys` lists it as neglected.
const StaleAfter = 28 * 24 * time.Hour

// The contexts a key is logged under: the places where what a key does changes.
const (
	ctxBookmarks   = "bookmarks"
	ctxConfirm     = "confirm"
	ctxDestination = "destination"
	ctxDetail      = "detail"
	ctxDiff        = "diff"
	ctxDock        = "dock"
	ctxFiles       = "files"
	ctxFind        = "find"
	ctxForge       = "forge"
	ctxHelp        = "help"
	ctxLog         = "log"
	ctxOpLog       = "oplog"
	ctxPalette     = "palette"
	ctxPick        = "pick"
	ctxTravel      = "travel"
	ctxOutput      = "output"
	ctxTrail       = "trail"
	ctxPrompt      = "prompt"
	ctxSpaces      = "spaces"
	ctxOutline     = "outline"
	ctxSubmit      = "submit"
	ctxRounds      = "rounds"
)

// recordModalKey logs a key an overlay is about to take. Overlays with a text field log only the
// keys that are not text, because the log answers what was reached for and not what was typed.
func (m *Model) recordModalKey(ctx string, msg tea.KeyPressMsg) {
	switch ctx {
	case ctxPrompt, ctxFind, ctxDetail:
		return
	case ctxPalette, ctxDestination:
		if msg.Text != "" {
			return
		}
	}

	m.usage.Key(ctx, msg.String())
}

// screenKeys is a screen's bindings as the user has bound them, sequences spelled the way the
// dispatcher records them: the keys joined by spaces.
func screenKeys(km help.Keymap, screen keys.Context) []keyusage.Binding {
	bound := km.Bound(screen)
	out := make([]keyusage.Binding, 0, len(bound))

	for _, b := range bound {
		out = append(out, keyusage.Binding{Keys: b.Keys, What: b.What})
	}

	return out
}

func bind(what string, seqs ...string) keyusage.Binding {
	return keyusage.Binding{Keys: seqs, What: what}
}

func listMoves() []keyusage.Binding {
	return []keyusage.Binding{
		bind("page up", "pgup", "ctrl+u"),
		bind("page down", "pgdown", "ctrl+d"),
		bind("first", "g", "home"),
		bind("last", "G", "end"),
	}
}

func listModal(own ...keyusage.Binding) []keyusage.Binding {
	return append(own, listMoves()...)
}

// UsageKeymap is every context a key is logged under, with the keymap's screens as the user has
// bound them.
func UsageKeymap(km help.Keymap) keyusage.Keymap {
	pick := screenKeys(km, keys.Picker)
	for _, screen := range []keys.Context{
		help.PickRebase, help.PickSquash, help.PickDuplicate, help.PickNew,
		help.PickRestore,
	} {
		pick = append(pick, screenKeys(km, screen)...)
	}

	return keyusage.Keymap{
		ctxLog:     screenKeys(km, keys.Log),
		ctxPick:    pick,
		ctxTravel:  screenKeys(km, help.Travel),
		ctxOutline: screenKeys(km, help.Outline),
		ctxDock:    screenKeys(km, keys.View),
		ctxFiles:   screenKeys(km, keys.List),
		ctxRounds: listModal(bind("show in the dock", "enter"), bind("restore to it", "R"),
			bind("close", "esc", "q", "H")),
		ctxForge: {
			bind("refresh", "ctrl+l"),
			bind("back to the log", "esc"),
			bind("switch pane", "tab"),
		},
		ctxHelp: {
			bind("run", "enter"),
			bind("filter", "/"),
			bind("close", "esc"),
			bind("next", "down", "ctrl+n"),
			bind("previous", "up", "ctrl+p"),
		},
		ctxTrail: {
			bind("full stderr", "enter"), bind("copy as shell", "Y"), bind("undo", "u"),
			bind("show reads", "r"), bind("next", "j", "down"), bind("previous", "k", "up"),
			bind("first", "g", "home"), bind("last", "G", "end"), bind("close", "esc", "q"),
		},
		ctxConfirm: {bind("yes", "y"), bind("no", "n", "esc", "q")},
		ctxBookmarks: listModal(
			bind("set here", "s"), bind("advance", "a"), bind("create", "c"), bind("rename", "r"),
			bind("delete", "d"), bind("forget", "f"), bind("track", "t"), bind("push", "p"),
			bind("resolve", "x"), bind("close", "esc", "q", "b"),
		),
		ctxOpLog: listModal(
			bind("restore", "enter"), bind("show", "p"), bind("revert", "u"), bind("close", "esc", "q", "o"),
			bind("label jump", "'"), bind("jump back", "ctrl+o"), bind("jump forward", "ctrl+i", "tab"),
		),
		ctxSpaces: listModal(bind("add", "a"), bind("forget", "f"), bind("close", "esc", "q", "w")),
		ctxPalette: {
			bind("run", "enter"),
			bind("close", "esc"),
			bind("next", "down", "ctrl+n"),
			bind("previous", "up", "ctrl+p"),
			bind("explain", "?"),
		},
		ctxDestination: {
			bind("pick", "enter"),
			bind("insert after", "alt+a"),
			bind("insert before", "alt+b"),
			bind("close", "esc"),
			bind("page up", "pgup"),
			bind("page down", "pgdown"),
			bind("first", "home"),
			bind("last", "end"),
			bind("next", "ctrl+n"),
			bind("previous", "ctrl+p"),
		},
	}
}
