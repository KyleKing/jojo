package model

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/trail"

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

// The footer keeps one line of a failure, and gm opens the trail on it: the exact command, why it
// failed, and enter for all of jj's stderr.
func TestMessages_ShowsTheFullTextOfTheLastFailure(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))

	m = asModel(t, pressKeys(t, m, "g", "m"))
	if !m.trailView.IsOpen() || !strings.Contains(tuitest.StripANSI(m.render()), "nothing has run yet") {
		t.Fatal("gm with nothing run did not open an empty trail that says so")
	}

	m = asModel(t, update(m, tuitest.Key("esc")))

	m.trail.Add(trail.Call{
		Argv: []string{"jj", "git", "fetch", "--all-remotes"}, Write: true, Exit: 1, Start: time.Now(),
		Stderr: "Fetching from Git remotes: origin\nError: Could not find repository at '/nonexistent/remote.git'\n",
	})
	m.trail.Add(trail.Call{Argv: []string{"jj", "describe", "-m", "later"}, Write: true, Start: time.Now()})

	m = failed(t, m)
	if !strings.Contains(tuitest.StripANSI(m.render()), "gm full error") {
		t.Error("the footer does not offer the full error")
	}

	m = asModel(t, pressKeys(t, m, "g", "m"))
	if !m.trailView.IsOpen() {
		t.Fatal("gm did not open the trail")
	}

	frame := tuitest.StripANSI(m.render())
	for _, want := range []string{
		"jj git fetch --all-remotes", "exit 1 · Fetching from Git remotes", "jj describe -m later",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("the trail lacks %q:\n%s", want, frame)
		}
	}

	if c, ok := m.trailView.Selected(); !ok || !c.Failed() {
		t.Errorf("the cursor is on %+v, want the failure", c)
	}

	next, show := m.Update(tuitest.Key("enter"))
	if show == nil {
		t.Fatal("enter on the failure asked for nothing")
	}

	m = asModel(t, update(asModel(t, next), show()))
	if !m.output.IsOpen() {
		t.Fatal("enter did not open the failure's stderr")
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, "Error: Could not find repository") {
		t.Errorf("the full text lacks jj's error line:\n%s", frame)
	}

	m = asModel(t, update(m, tuitest.Key("esc")))
	if m.output.IsOpen() {
		t.Error("esc left the stderr open")
	}
}

// A split canceled in jj-diff wrote nothing, and the trail still counted it as one write. A program
// jojo hands the terminal to is filed as a write only when jj's operation log moved under it.
func TestExec_TrailCountsAWriteOnlyWhenTheOpLogMoved(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")

	for _, tc := range []struct {
		write func()
		name  string
		want  bool
	}{
		{func() {}, "canceled", false},
		{func() { jjRun(t, dir, "describe", "-m", "renamed", "-r", changeFor(t, dir, "top")) }, "wrote", true},
	} {
		before := headOp(t.Context(), m.client)
		tc.write()
		m.client.AcquireQueue()

		msg := execDoneMsg{
			before: before, outcome: func(error, bool) tea.Msg { return nil },
			call: trail.Call{Argv: []string{"jj-diff", "--split", tc.name}},
		}
		m = pump(t, m, m.finishExec(msg))

		var filed, wrote bool

		for _, c := range m.trail.Calls(false) {
			if len(c.Argv) == 3 && c.Argv[2] == tc.name {
				filed, wrote = true, c.Write
			}
		}

		if !filed || wrote != tc.want {
			t.Errorf("%s: filed %v as a write %v, want filed as a write %v", tc.name, filed, wrote, tc.want)
		}
	}
}
