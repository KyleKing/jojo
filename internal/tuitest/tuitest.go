// Package tuitest drives a Bubble Tea v2 model in-process and asserts its
// rendered frames against golden files, without a real terminal.
package tuitest

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/kyleking/aragonite/tui/framecheck"
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
// "alt+p", or a single printable rune such as "j".
func Key(name string) tea.KeyPressMsg {
	mod := tea.KeyMod(0)
	if rest, ok := strings.CutPrefix(name, "ctrl+"); ok && rest != "" {
		mod |= tea.ModCtrl
		name = rest
	}

	if rest, ok := strings.CutPrefix(name, "alt+"); ok && rest != "" {
		mod |= tea.ModAlt
		name = rest
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
//
// The frame is held to framecheck first: a golden compares bytes, so a frame drawn a cell off when it
// was recorded would otherwise pass forever.
func AssertGolden(tb testing.TB, frame string) {
	tb.Helper()
	framecheck.Require(tb, tb.Name(), StripANSI(frame))
	golden.RequireEqual(tb, StripANSI(frame))
}

// AssertGraphGolden is AssertGolden for a frame that draws jj's graph, whose branch arms meet a node
// glyph rather than a line, which framecheck's corner rule would read as an arm with nothing to join.
// Every other rule still holds.
func AssertGraphGolden(tb testing.TB, frame string) {
	tb.Helper()

	for _, f := range framecheck.Check(StripANSI(frame)) {
		if f.Rule != framecheck.Corner {
			tb.Errorf("%s: %s", tb.Name(), f)
		}
	}

	golden.RequireEqual(tb, StripANSI(frame))
}

// sgr matches one SGR sequence and captures its parameters.
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// ColorsIn is every SGR parameter in line that sets a foreground or background color, which a
// reverse-video selected row must not carry: reversing a colored cell draws it as a block of that
// color inside the bar.
func ColorsIn(line string) []string {
	var out []string

	for _, m := range sgr.FindAllStringSubmatch(line, -1) {
		params := strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ':' })
		for i := 0; i < len(params); i++ {
			n, err := strconv.Atoi(params[i])
			if err != nil {
				continue
			}

			switch {
			case n == 38 || n == 48: // extended fore- and background
				out = append(out, m[0])
				i = len(params)
			case (n >= 30 && n <= 37) || (n >= 40 && n <= 47) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107):
				out = append(out, m[0])
			}
		}
	}

	return out
}

// AttrsIn is every SGR attribute in line that is not a color: 1 bold, 2 faint, 3 italic, 4 underline,
// 7 reverse, 9 strikethrough. A selected row that keeps these keeps what each cell's face says
// beyond its color.
func AttrsIn(line string) map[int]bool {
	out := map[int]bool{}

	for _, m := range sgr.FindAllStringSubmatch(line, -1) {
		params := strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ':' })
		for i := 0; i < len(params); i++ {
			n, err := strconv.Atoi(params[i])
			if err != nil {
				continue
			}

			switch {
			case n == 38 || n == 48 || n == 58: // an extended color's own parameters follow
				i = len(params)
			case n >= 1 && n <= 9:
				out[n] = true
			}
		}
	}

	return out
}
