package launch_test

import (
	"image/color"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/kyleking/jojo/internal/launch"
)

// recorder is the program a launch builds: it notes how it was built and every message it is given.
type recorder struct {
	got  []string
	dark bool
}

func (recorder) Init() tea.Cmd { return nil }

func (r recorder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		r.got = append(r.got, key.String())
	}

	return r, nil
}

func (recorder) View() tea.View { return tea.NewView("") }

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// TestLaunch_KeysTypedBeforeTheAnswerReachTheProgram pins what a launch is for: keys that arrive
// while the terminal is being asked are held, not lost, and reach the program in the order typed,
// built for the background the terminal answered.
func TestLaunch_KeysTypedBeforeTheAnswerReachTheProgram(t *testing.T) {
	t.Parallel()

	light := tea.BackgroundColorMsg{Color: color.RGBA{R: 0xef, G: 0xf1, B: 0xf5, A: 0xff}}
	dark := tea.BackgroundColorMsg{Color: color.RGBA{R: 0x24, G: 0x27, B: 0x3a, A: 0xff}}

	for _, tc := range []struct {
		answer   tea.Msg
		name     string
		wantDark bool
	}{
		{light, "a light background", false},
		{dark, "a dark background", true},
		{uv.PrimaryDeviceAttributesEvent{62, 22}, "device attributes with no background", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var m tea.Model = launch.New(func(dark bool) tea.Model { return recorder{dark: dark} })
			for _, r := range "jjk" {
				m, _ = m.Update(key(r))
			}

			m, _ = m.Update(tc.answer)

			got, ok := m.(recorder)
			if !ok {
				t.Fatalf("the answer did not build the program, the model is %T", m)
			}

			if got.dark != tc.wantDark {
				t.Errorf("built dark=%t, want %t", got.dark, tc.wantDark)
			}

			if want := []string{"j", "j", "k"}; !slices.Equal(got.got, want) {
				t.Errorf("the program was given %q, want %q", got.got, want)
			}
		})
	}
}

// TestLaunch_SilentTerminalStartsDarkAfterTheWait pins a terminal that answers nothing: the wait
// runs out and the program starts dark with what was typed.
func TestLaunch_SilentTerminalStartsDarkAfterTheWait(t *testing.T) {
	t.Parallel()

	m := launch.New(func(dark bool) tea.Model { return recorder{dark: dark} }).WithSettle(0)
	if m.Init() == nil {
		t.Fatal("Init asked nothing")
	}

	var next tea.Model = m

	next, _ = next.Update(key('j'))

	// The tick Init schedules; with no wait it fires at once.
	for _, msg := range drain(m.Init()) {
		next, _ = next.Update(msg)
	}

	got, ok := next.(recorder)
	if !ok {
		t.Fatalf("the wait running out did not build the program, the model is %T", next)
	}

	if !got.dark || !slices.Equal(got.got, []string{"j"}) {
		t.Errorf("built dark=%t given %q, want dark given [j]", got.dark, got.got)
	}
}

// drain runs cmd and any batch it returns, collecting the messages that are not terminal writes.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}

		return out
	case tea.RawMsg:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// TestLaunch_ViewIsBlankOnTheAltScreen pins the screen while the terminal is asked.
func TestLaunch_ViewIsBlankOnTheAltScreen(t *testing.T) {
	t.Parallel()

	view := launch.New(nil).View()
	if !view.AltScreen {
		t.Error("the launch drew on the main screen, which flashes before the program's alt screen")
	}
}
