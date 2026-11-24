package model

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/tuitest"
)

const fetchFailure = "jj git fetch --all-remotes\n\nFetching from Git remotes: origin\n" +
	"Error: Could not find repository at '/nonexistent/remote.git'"

func failed(t *testing.T, m Model) Model {
	t.Helper()

	return asModel(t, update(m, messageMsg{
		text:   "jj git fetch: Could not find repository at '/nonexistent/remote.git'",
		output: fetchFailure,
	}))
}

// An overlay used to take the whole frame, footer included, so a failure raised while one was open
// never showed. The failure keeps the bottom row at every size.
func TestFailure_ShowsUnderAnOpenOverlay(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ width, height int }{{120, 35}, {80, 24}, {50, 12}} {
		t.Run(strconv.Itoa(size.width), func(t *testing.T) {
			t.Parallel()

			m, _ := deliver(testModel(t, twoRows()), operations(), nil, showOpLog)
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size.width, Height: size.height}))
			m = failed(t, m)

			lines := strings.Split(tuitest.StripANSI(m.render()), "\n")
			if len(lines) != size.height {
				t.Fatalf("frame is %d rows, want %d", len(lines), size.height)
			}

			if last := lines[len(lines)-1]; !strings.Contains(last, "jj git fetch") {
				t.Errorf("the failure is not on the bottom row: %q", last)
			}
		})
	}
}

// The footer keeps one line of a failure, and "!" shows all of it, from the log and over a panel.
func TestBang_ShowsTheFullTextOfTheLastFailure(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))

	next, _ := m.Update(tuitest.Key("!"))
	if asModel(t, next).output.IsOpen() {
		t.Fatal("! opened an empty failure")
	}

	m = failed(t, m)
	if !strings.Contains(tuitest.StripANSI(m.render()), "! full error") {
		t.Error("the footer does not offer the full error")
	}

	m = asModel(t, update(m, tuitest.Key("!")))
	if !m.output.IsOpen() {
		t.Fatal("! did not open the failure")
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, "Error: Could not find repository") {
		t.Errorf("the full text lacks jj's error line:\n%s", frame)
	}

	m = asModel(t, update(m, tuitest.Key("esc")))
	if m.output.IsOpen() {
		t.Error("esc left the failure open")
	}
}
