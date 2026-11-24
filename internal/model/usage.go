package model

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/keyusage"

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
	ctxEvolog      = "evolog"
	ctxFiles       = "files"
	ctxFind        = "find"
	ctxForge       = "forge"
	ctxHelp        = "help"
	ctxLog         = "log"
	ctxOpLog       = "oplog"
	ctxPalette     = "palette"
	ctxOutput      = "output"
	ctxPrompt      = "prompt"
	ctxSpaces      = "spaces"
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

func bind(what string, keys ...string) keyusage.Binding {
	return keyusage.Binding{Keys: keys, What: what}
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

// UsageKeymap is every context a key is logged under, with the log's keys as the user has bound them.
func UsageKeymap(keys help.Keymap) keyusage.Keymap {
	logKeys := []keyusage.Binding{
		bind("quit", "q", "ctrl+c"),
		bind("help", "?"),
		bind("command palette", ":"),
		bind("switch pane", "tab"),
	}

	runnable := keys.Runnable()
	for i := range runnable {
		logKeys = append(logKeys, bind(runnable[i].What, runnable[i].Run))
	}

	return keyusage.Keymap{
		ctxLog: append(logKeys, listMoves()...),
		ctxForge: {
			bind("refresh", "r"),
			bind("back to the log", "esc"),
			bind("switch pane", "tab"),
		},
		ctxHelp: {
			bind("close", "esc", "q", "?"),
			bind("first", "g", "home"),
			bind("last", "G", "end"),
		},
		ctxConfirm: {bind("yes", "y"), bind("no", "n", "esc", "q")},
		ctxBookmarks: listModal(
			bind("set here", "s"), bind("advance", "a"), bind("create", "c"), bind("rename", "r"),
			bind("delete", "d"), bind("forget", "f"), bind("track", "t"), bind("push", "p"),
			bind("resolve", "x"), bind("close", "esc", "q", "b"),
		),
		ctxDiff: listModal(
			bind("previous file", "["), bind("next file", "]"), bind("side by side", "s"), bind("close", "esc", "q"),
		),
		ctxDock: listModal(
			bind("previous file", "["), bind("next file", "]"), bind("side by side", "s"),
			bind("back to the log", "esc", "q"), bind("switch pane", "tab"), bind("close", dockKey),
		),
		ctxFiles: listModal(
			bind("diff the file", "enter"), bind("restore", "r"), bind("move", "m"),
			bind("resolve", "c"), bind("stage or unstage", "space"), bind("commit", "C"),
			bind("close", "esc", "q", "l"),
		),
		ctxOpLog: listModal(
			bind("restore", "enter", "r"), bind("show", "d"), bind("revert", "v"), bind("close", "esc", "q", "o"),
		),
		ctxEvolog: listModal(
			bind("show", "enter"), bind("mark", "space"), bind("interdiff", "d"),
			bind("restore", "r"), bind("close", "esc", "q"),
		),
		ctxSpaces: listModal(bind("add", "a"), bind("forget", "f"), bind("close", "esc", "q", "w")),
		ctxPalette: {
			bind("run", "enter"),
			bind("close", "esc"),
			bind("page up", "pgup"),
			bind("page down", "pgdown"),
			bind("first", "home"),
			bind("last", "end"),
			bind("next", "ctrl+n"),
			bind("previous", "ctrl+p"),
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
