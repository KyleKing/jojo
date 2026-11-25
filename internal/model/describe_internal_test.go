package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/framecheck"
	"github.com/kyleking/aragonite/tui/vimtext"

	"github.com/kyleking/jojo/internal/components/prompt"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

const fullDescription = "fix: side branch\n\nLong body paragraph explaining why.\nSigned-off-by: X"

func describing(t *testing.T) Model {
	t.Helper()

	m, _ := deliver(testModel(t, twoRows()),
		jj.Detail{ChangeID: "aaaa", Description: fullDescription}, nil, openDescribe)

	if !m.prompt.IsOpen() {
		t.Fatal("describe did not open the prompt")
	}

	return m
}

// The log row carries the first line only, and seeding the prompt with it dropped the body and the
// trailers on Enter. The prompt starts from the whole description.
func TestDescribe_SeedsThePromptWithTheBodyAndTrailers(t *testing.T) {
	t.Parallel()

	if got := describing(t).prompt.Value(); got != fullDescription {
		t.Errorf("prompt holds %q, want the full description", got)
	}
}

// Enter on an unedited description used to rewrite the revision, an operation to undo for nothing.
func TestDescribe_EnterOnAnUnchangedDescriptionWritesNothing(t *testing.T) {
	t.Parallel()

	m := describing(t)

	_, cmd := m.handleSubmitted(prompt.SubmittedMsg{Value: fullDescription + "\n"})
	if cmd == nil {
		t.Fatal("submitting produced nothing to say")
	}

	msg, ok := cmd().(messageMsg)
	if !ok || msg.text != "description of aaaa unchanged" {
		t.Errorf("unchanged submit produced %#v, want the unchanged message and no write", cmd())
	}
}

// ctrl+e hands the draft to $EDITOR and takes the text back into the prompt, so a describe started
// in the prompt finishes there rather than in a second jj describe that forgets what was typed.
func TestDescribe_CtrlEEditsTheDraftAndKeepsThePrompt(t *testing.T) {
	t.Parallel()

	m := describing(t)

	next, cmd := m.Update(tuitest.Key("ctrl+e"))
	if cmd == nil {
		t.Fatal("ctrl+e produced no command")
	}

	done, _ := asModel(t, next).Update(vimtext.EditedMsg{ID: "describe aaaa", Text: "from the editor"})

	model := asModel(t, done)
	if !model.prompt.IsOpen() || model.prompt.Value() != "from the editor" || model.onSubmit == nil {
		t.Errorf("after the editor: open %v, value %q; want the editor's text in the describe prompt",
			model.prompt.IsOpen(), model.prompt.Value())
	}
}

// Describe docks its prompt under the log rather than blanking the screen for a centered box, so the
// change being described stays in view, and the footer says the prompt's keys.
func TestDescribe_KeepsTheLogInView(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{80, 24}, {120, 35}} {
		dir := liveRepo(t)
		m := liveModel(t, dir, "middle")
		m = asModel(t, update(m, tea.WindowSizeMsg{Width: size.width, Height: size.height}))
		m = press(t, m, "d")

		if !m.prompt.IsOpen() {
			t.Fatalf("%dx%d: d did not open the prompt", size.width, size.height)
		}

		frame := tuitest.StripANSI(m.render())
		lines := strings.Split(frame, "\n")

		if len(lines) != size.height {
			t.Errorf("%dx%d: the frame is %d rows", size.width, size.height, len(lines))
		}

		wants := []string{"bottom", "middle", "top", "─ describe ", "⏎ save", "ctrl+j new line", "esc normal", "INSERT"}
		for _, want := range wants {
			if !strings.Contains(frame, want) {
				t.Errorf("%dx%d: the frame lacks %q:\n%s", size.width, size.height, want, frame)
			}
		}

		if footer := lines[len(lines)-1]; !strings.Contains(footer, "esc normal") {
			t.Errorf("%dx%d: the last row is %q, want the prompt's keys", size.width, size.height, footer)
		}
	}
}

// Inside an open prompt the field has the keyboard, and vim's keys mean what they mean in vim: u in
// normal mode undoes the typing and never runs jj undo, and a appends and never abandons. The one key
// the field leaves alone is ctrl+c, so it still quits.
func TestDescribe_VimKeysStayInTheField(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "middle"), "d")

	if !m.prompt.IsOpen() {
		t.Fatal("d did not open the prompt")
	}

	ops, before := opCount(t, dir), m.prompt.Value()

	m = typeText(t, m, " more")
	m = press(t, m, "esc", "u")

	if got := m.prompt.Value(); got != before || !m.prompt.IsOpen() {
		t.Errorf("u in normal mode: value %q (open %v), want %q back in the open prompt",
			got, m.prompt.IsOpen(), before)
	}

	m = press(t, m, "a")
	m = typeText(t, m, "!")

	if got := m.prompt.Value(); !strings.HasSuffix(got, "!") || m.prompt.Mode() != vimtext.Insert {
		t.Errorf("a then !: value %q in %v, want an append in insert mode", got, m.prompt.Mode())
	}

	if got := opCount(t, dir) - ops; got != 0 {
		t.Errorf("vim keys in the prompt ran %d jj operations, want none", got)
	}

	if changeFor(t, dir, "middle") == "" {
		t.Error("a in the prompt abandoned the change being described")
	}

	next, cmd := m.Update(tuitest.Key("ctrl+c"))
	if _, quit := runCmd(t, cmd)[0].(tea.QuitMsg); !quit || !asModel(t, next).quitting {
		t.Error("ctrl+c in the prompt did not quit")
	}
}

// The describe prompt docked under the log at 80 columns, in insert mode as it opens and in normal
// mode after esc: the badge names the mode and the keys follow it.
func TestDescribe_Frames(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		keys []string
	}{
		{"insert", nil},
		{"normal", []string{"esc", "k"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 80, Height: 24}))
			m, _ = deliver(m, jj.Detail{ChangeID: "aaaa", Description: fullDescription}, nil, openDescribe)
			// The program lays the screen out again after the update that opened the prompt.
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: 80, Height: 24}))

			for _, k := range tc.keys {
				m, _ = pressOne(t, m, k)
			}

			frame := m.render()
			assertFits(t, frame, 80, 24, 0)
			framecheck.Require(t, tc.name, frame, framecheck.MaxWidth(80), framecheck.Graph())
			tuitest.AssertGolden(t, frame)

			if !strings.Contains(tuitest.StripANSI(frame), strings.ToUpper(tc.name)) {
				t.Errorf("the footer does not name %s mode:\n%s", tc.name, tuitest.StripANSI(frame))
			}
		})
	}
}
