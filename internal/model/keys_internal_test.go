package model

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func TestLogPagesWhenTheCursorRunsOutOfRows(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())
	m.logWindow = len(m.rows)
	m.logFull = true

	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	got := asModel(t, next)

	if got.logWindow != 2*len(m.rows) {
		t.Errorf("logWindow = %d, want %d after reaching the bottom", got.logWindow, 2*len(m.rows))
	}
}

func TestLogStaysPutWhenThePageIsNotFull(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())

	next, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	got := asModel(t, next)

	if got.logWindow != logLimit {
		t.Errorf("logWindow = %d, want it unchanged when the log is shorter than one page", got.logWindow)
	}
}

// errJJExploded is a log read that failed for a reason that is not the revset.
var errJJExploded = errors.New("jj exploded")

// launching is a model whose first log read has not come back, as every session starts.
func launching(t *testing.T) Model {
	t.Helper()

	return New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", testOptions(t))
}

// TestLaunch_KeyBeforeTheLogActsOnceItLands pins a key pressed before the first log read is back:
// it waits for the log rather than moving a cursor over no rows, and keys keep their order. A read
// that fails releases them too, since the failure is what they may be asking about.
func TestLaunch_KeyBeforeTheLogActsOnceItLands(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		landed logLoadedMsg
		name   string
		want   string
		keys   []string
	}{
		{logLoadedMsg{revset: "@", rows: threeRows()}, "one j", "bbbb", []string{"j"}},
		{logLoadedMsg{revset: "@", rows: threeRows()}, "in order", "bbbb", []string{"j", "j", "k"}},
		{logLoadedMsg{revset: "@", err: errJJExploded}, "a failed read", "", []string{"j"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := launching(t)
			for _, key := range tc.keys {
				m = asModel(t, update(m, tuitest.Key(key)))
			}

			if m.early.Ready() {
				t.Fatalf("keys released before the log landed")
			}

			m = asModel(t, update(m, tc.landed))

			if !m.early.Ready() {
				t.Errorf("keys still held after the log landed")
			}

			got := ""
			if rev := m.log.Selected(); rev != nil {
				got = rev.ChangeID
			}

			if got != tc.want {
				t.Errorf("cursor on %q after the log landed, want %q", got, tc.want)
			}
		})
	}
}

// TestLaunch_CtrlCIsNeverHeld pins the way out of a jj that never answers: ctrl+c acts at once.
func TestLaunch_CtrlCIsNeverHeld(t *testing.T) {
	t.Parallel()

	m := launching(t)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	early := asModel(t, next).early
	if held := early.Release(); len(held) != 0 {
		t.Errorf("ctrl+c was held for the log: %v", held)
	}
}

// Esc backs out one level and never quits: at the log's root, with nothing checked, it does nothing,
// so esc pressed to dismiss something already gone cannot end the session, as it did in second-look.
func TestEsc_AtTheRootNeverQuits(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))

	for range 3 {
		next, cmd := m.Update(tuitest.Key("esc"))
		m = asModel(t, next)

		if m.quitting {
			t.Fatal("esc at the root quit")
		}

		for _, msg := range flatten(cmd) {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("esc at the root sent a quit")
			}
		}
	}
}
