// Package tuitest drives a Bubble Tea v2 model in-process and asserts its
// rendered frames against golden files, without a real terminal.
package tuitest

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// namedKeys maps the key names accepted by Keys to their tea.Key code.
var namedKeys = map[string]rune{
	"backspace": tea.KeyBackspace,
	"down":      tea.KeyDown,
	"end":       tea.KeyEnd,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEscape,
	"home":      tea.KeyHome,
	"left":      tea.KeyLeft,
	"pgdown":    tea.KeyPgDown,
	"pgup":      tea.KeyPgUp,
	"right":     tea.KeyRight,
	"space":     tea.KeySpace,
	"tab":       tea.KeyTab,
	"up":        tea.KeyUp,
}

// Key builds a tea.KeyPressMsg from a name such as "enter", "up", "ctrl+c",
// or a single printable rune such as "j".
func Key(name string) tea.KeyPressMsg {
	mod := tea.KeyMod(0)
	if len(name) > 5 && name[:5] == "ctrl+" {
		mod |= tea.ModCtrl
		name = name[5:]
	}

	if code, ok := namedKeys[name]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}

	r, _ := utf8.DecodeRuneInString(name)
	text := ""
	if mod == 0 {
		text = name
	}

	return tea.KeyPressMsg{Code: r, Text: text, Mod: mod}
}

// Keys builds a tea.Msg for each name, in order, via Key.
func Keys(names ...string) []tea.Msg {
	msgs := make([]tea.Msg, len(names))
	for i, name := range names {
		msgs[i] = Key(name)
	}

	return msgs
}

// Run sizes m at width by height, sends Init's message, then each msg in
// order, and returns the resulting frame. It does not schedule commands
// beyond that first Init message, so a model that depends on a command's
// result arriving asynchronously (an I/O response, a timer) will not see it;
// send that result as one of msgs instead.
func Run(tb testing.TB, m tea.Model, width, height int, msgs ...tea.Msg) string {
	tb.Helper()

	model := m
	if cmd := model.Init(); cmd != nil {
		if msg := cmd(); msg != nil {
			model, _ = model.Update(msg)
		}
	}

	model, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	for _, msg := range msgs {
		model, _ = model.Update(msg)
	}

	return model.View().Content
}

// StripANSI removes SGR and other escape sequences so a golden file stays
// readable and diffable.
func StripANSI(s string) string {
	return ansi.Strip(s)
}

// AssertGolden compares frame against testdata/<TestName>.golden, updating
// it when the test binary was run with -update.
func AssertGolden(tb testing.TB, frame string) {
	tb.Helper()
	golden.RequireEqual(tb, StripANSI(frame))
}
