package model

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kyleking/jojo/internal/components/oplogview"
	"github.com/kyleking/jojo/internal/tuitest"
)

// traveling is the app looking at the second of operations(), with the log as it stood there.
func traveling(t *testing.T, width, height int, withList bool) Model {
	t.Helper()

	m := asModel(t, update(testModel(t, threeRows()), tea.WindowSizeMsg{Width: width, Height: height}))
	if withList {
		showOpLog(&m, operations())
	} else {
		m.startTravel(operations(), 1)
	}

	m.travelTo(1)
	m.landTravel(travelLogMsg{op: operations()[1].ID, rows: twoRows()})

	return m
}

// The past keeps the terminal's shape at every size, with and without the op log over it, and the
// banner names the operation and the way back.
func TestTravel_FitsTheTerminalAtEverySize(t *testing.T) {
	t.Parallel()

	sizes := []struct{ width, height int }{{100, 30}, {80, 24}, {60, 20}, {40, 12}}

	for _, withList := range []bool{false, true} {
		for _, size := range sizes {
			name := strconv.FormatBool(withList) + "/" + strconv.Itoa(size.width) + "x" + strconv.Itoa(size.height)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				frame := tuitest.StripANSI(traveling(t, size.width, size.height, withList).render())
				assertFits(t, frame, size.width, size.height, 0)

				if !strings.Contains(frame, "bbb222") && size.height >= minTravelSplit {
					t.Errorf("the banner does not name the operation:\n%s", frame)
				}
			})
		}
	}
}

// assertFits checks frame is height lines, none of them from line from on wider than width.
func assertFits(t *testing.T, frame string, width, height, from int) {
	t.Helper()

	lines := strings.Split(frame, "\n")
	if len(lines) != height {
		t.Errorf("the frame is %d lines, want %d", len(lines), height)
	}

	for i := from; i < len(lines); i++ {
		if w := lipgloss.Width(lines[i]); w > width {
			t.Errorf("line %d is %d wide, over %d: %q", i, w, width, lines[i])
		}
	}
}

// Leaving the op log leaves the past it showed: the log is the present again.
func TestTravel_ClosingTheOpLogReturnsToNow(t *testing.T) {
	t.Parallel()

	m := traveling(t, 100, 30, true)
	m = asModel(t, update(m, oplogview.CancelledMsg{}))

	if m.travel.active {
		t.Error("closing the op log left the log in the past")
	}
}

// Enter on an operation in the list asks first, with the operation's summary in the question, and a
// no comes back to the list rather than closing it.
func TestTravel_RestoreAsksFirst(t *testing.T) {
	t.Parallel()

	m := traveling(t, 100, 30, true)
	m = asModel(t, update(m, oplogview.RestoreMsg{Operation: operations()[1]}))

	if !m.confirm.IsOpen() {
		t.Fatal("restoring did not ask first")
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, "bbb222") {
		t.Errorf("the question does not name the operation:\n%s", frame)
	}

	m = asModel(t, update(m, tuitest.Key("n")))
	if !m.opLog.IsOpen() || !m.travel.active {
		t.Error("answering no left the op log")
	}
}
