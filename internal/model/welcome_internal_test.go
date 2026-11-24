package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/coach"
	"github.com/kyleking/aragonite/tui/palette"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// firstRun is the stack fixture at size for a reader whose ledger is new, kept at path.
func firstRun(t *testing.T, size [2]int, path string) Model {
	t.Helper()

	ledger, err := coach.LedgerAt(path)
	if err != nil {
		t.Fatal(err)
	}

	opts := testOptions(t)
	opts.Ledger = ledger

	m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", opts)
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: stackRows()})

	return asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
}

// The card over a first run's log at both sizes.
func TestWelcome_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := firstRun(t, size, filepath.Join(t.TempDir(), "coach.json"))
			frame := m.render()

			for _, want := range []string{"Welcome to jojo", "n", "]u", "I", "P"} {
				if !strings.Contains(tuitest.StripANSI(frame), want) {
					t.Errorf("the card has no %q:\n%s", want, tuitest.StripANSI(frame))
				}
			}

			assertVerbFrame(t, frame, size)
		})
	}
}

// Any key takes the card down and still acts, the help key opening the legend; the dismissal is saved at once,
// so the next launch does not greet again.
func TestWelcome_AnyKeyDismissesActsAndIsSaved(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "coach.json")
	m, _ := pressOne(t, firstRun(t, [2]int{100, 30}, path), "?")

	if m.welcome.Showing() || !m.pal.IsOpen() {
		t.Errorf("? left the card up (%v) or opened no legend (%v)", m.welcome.Showing(), m.pal.IsOpen())
	}

	again, err := coach.LedgerAt(path)
	if err != nil {
		t.Fatal(err)
	}

	if coach.Due(again) {
		t.Error("the dismissal was not saved: a reopened ledger still wants the card")
	}
}

// Esc only dismisses; :welcome brings the card back.
func TestWelcome_EscOnlyDismissesAndTheCommandReopens(t *testing.T) {
	t.Parallel()

	m := firstRun(t, [2]int{100, 30}, filepath.Join(t.TempDir(), "coach.json"))
	m.log.Update(tuitest.Key("space"))

	m, _ = pressOne(t, m, "esc")
	if m.welcome.Showing() || len(m.log.Checked()) != 1 || m.quitting {
		t.Errorf("esc: card %v, checks %v, quitting %v", m.welcome.Showing(), m.log.Checked(), m.quitting)
	}

	next, _ := m.Update(palette.RunMsg{Action: coach.WelcomeAction})
	if !asModel(t, next).welcome.Showing() {
		t.Error(":welcome did not reopen the card")
	}
}

// Below the card's bounds the footer says it in a line; with no ledger it never shows.
func TestWelcome_SmallTerminalAndNoLedger(t *testing.T) {
	t.Parallel()

	m := firstRun(t, [2]int{80, 14}, filepath.Join(t.TempDir(), "coach.json"))
	lines := strings.Split(tuitest.StripANSI(m.render()), "\n")

	if last := lines[len(lines)-1]; !strings.Contains(last, "welcome") {
		t.Errorf("a 80x14 footer has no welcome line: %q", last)
	}

	if testModel(t, stackRows()).welcome.Showing() {
		t.Error("a model with no ledger greeted the reader")
	}
}
