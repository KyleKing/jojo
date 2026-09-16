package tuitest_test

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/tuitest"
)

// counterModel increments on "j" and quits on "q", enough to exercise Run
// against real Update/View calls without pulling in app code.
type counterModel struct {
	n     int
	width int
}

func (counterModel) Init() tea.Cmd { return nil }

func (m counterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j":
			m.n++
		case "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m counterModel) View() tea.View {
	return tea.NewView(fmt.Sprintf("count=%d width=%d", m.n, m.width))
}

func TestRun_FeedsKeysInOrder(t *testing.T) {
	t.Parallel()

	frame := tuitest.Run(t, counterModel{}, 40, 10, tuitest.Keys("j", "j", "j")...)
	const want = "count=3 width=40"
	if frame != want {
		t.Fatalf("got %q, want %q", frame, want)
	}
}

func TestAssertGolden_MatchesStoredFrame(t *testing.T) {
	t.Parallel()

	frame := tuitest.Run(t, counterModel{}, 40, 10, tuitest.Keys("j", "j")...)
	tuitest.AssertGolden(t, frame)
}
