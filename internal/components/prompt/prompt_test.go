package prompt_test

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/kyleking/aragonite/tui/theme"
	"github.com/kyleking/aragonite/tui/vimtext"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 60, 20

// errSpaces is the refusal the name check in these tests answers.
var errSpaces = errors.New("a bookmark name has no spaces")

func testSkin() ui.Skin {
	return ui.New(theme.Macchiato())
}

//nolint:ireturn // T is concrete at every call site, only the signature is generic.
func drainMsg[T any](cmd tea.Cmd) (T, bool) {
	var zero T

	if cmd == nil {
		return zero, false
	}

	msg, ok := cmd().(T)

	return msg, ok
}

// typeKeys presses each key in turn, a key name per element.
func typeKeys(m *prompt.Model, keys ...string) {
	for _, k := range keys {
		m.Update(tuitest.Key(k))
	}
}

// typeText types each character of text.
func typeText(m *prompt.Model, text string) {
	for _, r := range text {
		m.Update(tuitest.Key(string(r)))
	}
}

func TestEnterSubmitsEmptyValue(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "")

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit prompt.SubmittedMsg")
	}

	if submitted.Value != "" || submitted.Title != "Describe" {
		t.Errorf("SubmittedMsg = %+v, want an empty value under its title", submitted)
	}

	if m.IsOpen() {
		t.Error("prompt still open after submitting")
	}
}

func TestEnterSubmitsTypedValue(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "")
	typeText(&m, "wip")

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit prompt.SubmittedMsg")
	}

	if submitted.Value != "wip" {
		t.Errorf("SubmittedMsg.Value = %q, want %q", submitted.Value, "wip")
	}
}

// Esc backs out one level and never two: from insert mode it goes to normal mode, keeping the prompt
// and its text, and only esc in normal mode cancels.
func TestEsc_GoesToNormalModeThenCancels(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe", "existing")

	if cmd := m.Update(tuitest.Key("esc")); cmd != nil {
		if _, canceled := drainMsg[prompt.CancelledMsg](cmd); canceled {
			t.Fatal("the first esc canceled the prompt")
		}
	}

	if !m.IsOpen() || m.Mode() != vimtext.Normal || m.Value() != "existing" {
		t.Fatalf("after one esc: open %v, mode %v, value %q; want open in normal mode", m.IsOpen(), m.Mode(), m.Value())
	}

	if _, ok := drainMsg[prompt.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("esc in normal mode did not emit prompt.CancelledMsg")
	}

	if m.IsOpen() {
		t.Error("prompt still open after canceling")
	}
}

// With vim = false there is no normal mode to go to, so esc cancels at once.
func TestEsc_CancelsAtOnceWithTheVimLayerOff(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin(), prompt.WithVim(false))
	m.OpenText("describe abc", "existing")

	if _, ok := drainMsg[prompt.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("esc with the vim layer off did not cancel")
	}

	if m.IsOpen() {
		t.Error("prompt still open after canceling")
	}
}

// The field has the keyboard, so vim's keys edit the text: u undoes what was typed and a appends,
// where on the log u is jj undo and a abandons. The root model holds those keys away from the log;
// this pins that the field itself acts on them.
func TestNormalMode_UndoesAndAppendsInTheText(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenText("describe abc", "feat: x")
	typeText(&m, " more")
	typeKeys(&m, "esc", "u")

	if got := m.Value(); got != "feat: x" {
		t.Fatalf("u in normal mode left %q, want the typing undone", got)
	}

	typeKeys(&m, "A")
	typeText(&m, "!")

	if got := m.Value(); got != "feat: x!" || m.Mode() != vimtext.Insert {
		t.Errorf("A then ! gave %q in %v, want an append in insert mode", got, m.Mode())
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("Describe change", "wip: ")

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}

// A description's body and trailers are part of it. The single-line field dropped every line after
// the first, so Enter on an unedited prompt rewrote the description without them.
func TestOpenText_KeepsEveryLineThroughSubmit(t *testing.T) {
	t.Parallel()

	full := "fix: side branch\n\nLong body paragraph explaining why.\nSigned-off-by: X"

	m := prompt.New(testSkin())
	m.OpenText("describe abc", full)

	if got := m.Value(); got != full {
		t.Fatalf("seeded %q, want %q", got, full)
	}

	m.Update(tuitest.Key("alt+enter"))

	submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("enter did not submit")
	}

	if submitted.Value != full+"\n" {
		t.Errorf("submitted %q, want every original line plus the line break typed at the end", submitted.Value)
	}
}

// ctrl+e hands the draft to $EDITOR and takes back what the editor wrote, keeping the prompt open on
// it, rather than leaving the prompt for jj's editor and losing what was typed.
func TestOpenText_CtrlETakesTheEditorsTextBack(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenText("describe abc", "subject")

	if m.Update(tuitest.Key("ctrl+e")) == nil {
		t.Fatal("ctrl+e did not hand the draft to the editor")
	}

	m.Update(vimtext.EditedMsg{ID: "describe abc", Text: "subject\n\nbody from the editor"})

	if got := m.Value(); got != "subject\n\nbody from the editor" || !m.IsOpen() {
		t.Errorf("after the editor: %q (open %v), want its text in an open prompt", got, m.IsOpen())
	}

	m.Update(vimtext.EditedMsg{ID: "another prompt", Text: "stray"})

	if strings.Contains(m.Value(), "stray") {
		t.Error("an editor's answer for another field landed in this one")
	}
}

// The one-line prompts (bookmark names, workspace names) have no editor to hand off to.
func TestOpen_CtrlEDoesNothingOnASingleLinePrompt(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.Open("new bookmark", "")

	if m.Update(tuitest.Key("ctrl+e")) != nil {
		t.Error("a single-line prompt offered the editor")
	}
}

// The cursor starts at the end of a description, so typing adds to it: it started at the beginning,
// and "hello world" typed into "feat: wip work" read "hello worldfeat: wip work".
func TestOpenText_TypingAddsAtTheEnd(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenText("describe abc", "feat: wip work\n\nbody")
	typeText(&m, " more")

	if got := m.Value(); got != "feat: wip work\n\nbody more" {
		t.Errorf("typing into the description gave %q", got)
	}
}

// A name with rules says why it is refused where the keys are, and stays open to be fixed.
func TestOpenName_RefusesWithTheReasonAndStaysOpen(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.OpenName("new bookmark", "", func(name string) error {
		if strings.Contains(name, " ") {
			return errSpaces
		}

		return nil
	})
	typeText(&m, "a b")

	if _, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter"))); ok || !m.IsOpen() {
		t.Fatal("enter submitted a refused name")
	}

	if footer := tuitest.StripANSI(m.Footer(testWidth)); !strings.Contains(footer, errSpaces.Error()) {
		t.Errorf("the footer does not say why: %q", footer)
	}

	typeKeys(&m, "backspace", "backspace")

	if submitted, ok := drainMsg[prompt.SubmittedMsg](m.Update(tuitest.Key("enter"))); !ok || submitted.Value != "a" {
		t.Errorf("a fixed name did not submit: %+v %v", submitted, ok)
	}
}

// Every prompt keeps one contract, and its footer says so in the mode it is in: enter saves, ctrl+j
// breaks the line where the value can hold one, and esc goes to normal mode, where it cancels.
func TestFooter_SaysTheContract(t *testing.T) {
	t.Parallel()

	text := prompt.New(testSkin())
	text.OpenText("describe abc", "")

	line := prompt.New(testSkin())
	line.Open("new bookmark", "")

	normal := prompt.New(testSkin())
	normal.Open("new bookmark", "")
	typeKeys(&normal, "esc")

	plain := prompt.New(testSkin(), prompt.WithVim(false))
	plain.OpenText("describe abc", "")

	for _, tc := range []struct {
		name string
		want string
		m    prompt.Model
	}{
		{"multi-line", "INSERT  ⏎ save · ctrl+j new line · esc normal · ctrl+e $EDITOR", text},
		{"one line", "INSERT  ⏎ save · esc normal", line},
		{"normal mode", "NORMAL  ⏎ save · i insert · esc cancel", normal},
		{"vim off", "⏎ save · ctrl+j new line · esc cancel · ctrl+e $EDITOR", plain},
	} {
		if got := strings.TrimSpace(tuitest.StripANSI(tc.m.Footer(100))); got != tc.want {
			t.Errorf("%s footer %q, want %q", tc.name, got, tc.want)
		}
	}

	if _, ok := drainMsg[prompt.SubmittedMsg](text.Update(tuitest.Key("ctrl+j"))); ok || text.Value() != "\n" {
		t.Errorf("ctrl+j submitted or did not break the line: %q", text.Value())
	}
}

// sgr is one SGR escape, whose parameters say what it sets.
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// backgrounds lists every background an ANSI string sets, skipping the parameters of an extended
// foreground (38;5;n and 38;2;r;g;b), whose numbers would otherwise read as background codes.
func backgrounds(s string) []string {
	var out []string

	for _, match := range sgr.FindAllStringSubmatch(s, -1) {
		params := strings.Split(match[1], ";")
		for i := 0; i < len(params); i++ {
			n, err := strconv.Atoi(params[i])
			if err != nil {
				continue
			}

			switch {
			case n == 38 || n == 58:
				i += extended(params, i)
			case n == 48 || (n >= 40 && n <= 47) || (n >= 100 && n <= 107):
				out = append(out, match[0])
			}
		}
	}

	return out
}

// extended is how many parameters after i an extended color (5;n or 2;r;g;b) takes.
func extended(params []string, i int) int {
	if i+1 < len(params) && params[i+1] == "2" {
		return 4
	}

	return 2
}

// The description's field drew its cursor line on a background, which at 16 colors came out black
// across the whole row. The field draws on the terminal's own background at every depth, and only the
// cursor cell stands out, in reverse video.
func TestDocked_DrawsNoBackgroundAtSixteenColors(t *testing.T) {
	t.Parallel()

	for _, profile := range []colorprofile.Profile{colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
		m := prompt.New(ui.ForProfile(testSkin(), profile))
		m.OpenText("describe abc", "feat: parse the header\n\nbody line one\nbody line two")
		typeKeys(&m, "esc", "k")

		if bg := backgrounds(m.Docked(testWidth, testHeight)); len(bg) > 0 {
			t.Errorf("%v: the docked field sets backgrounds %q", profile, bg)
		}
	}
}

// A description opened on a tall screen keeps its height when the screen shrinks under it, and the
// rows that still fit are the ones around the cursor, so what is being typed stays in view.
func TestDocked_KeepsTheCursorInViewWhenTheScreenShrinks(t *testing.T) {
	t.Parallel()

	m := prompt.New(testSkin())
	m.SetHeight(40)
	m.OpenText("describe abc", "")

	for i := range 12 {
		typeKeys(&m, "ctrl+j")
		typeText(&m, "line "+strconv.Itoa(i))
	}

	docked := tuitest.StripANSI(m.Docked(testWidth, 12))
	if !strings.Contains(docked, "line 11") || strings.Contains(docked, "line 0 ") {
		t.Errorf("the shrunk prompt does not show the rows around the cursor:\n%s", docked)
	}

	if got := len(strings.Split(docked, "\n")); got != m.Rows(testWidth, 12) {
		t.Errorf("the panel is %d rows, want %d", got, m.Rows(testWidth, 12))
	}
}
