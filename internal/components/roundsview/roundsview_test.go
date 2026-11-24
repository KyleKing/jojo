package roundsview_test

import (
	"fmt"
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/roundsview"
	"github.com/kyleking/jojo/internal/rounds"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func list() []rounds.Round {
	return []rounds.Round{
		{To: "v1", Verb: "new", Age: "3 hours ago", First: 1, Last: 1, Files: 1, Added: 12},
		{From: "v1", To: "v2", Verb: "snapshot", Age: "2 hours ago", First: 2, Last: 2, Files: 2, Added: 4, Removed: 1},
		{From: "v2", To: "v4", Verb: "rebase", Age: "1 hour ago", First: 3, Last: 4, Moved: true},
		{From: "v4", To: "v5", Verb: "describe", Age: "5 minutes ago", First: 5, Last: 5, Before: "a", After: "b"},
	}
}

func TestView_Sizes(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := roundsview.New(ui.New(theme.Macchiato()))
			m.Open("kxqpmnzo", list(), 5, 1)
			tuitest.AssertGolden(t, m.View(size[0], size[1]))
		})
	}
}

// The rows run newest first under the change as it stands, and Enter names the round by its
// index from the oldest, so the box and the dock's ]r agree on which round is which.
func TestEnter_PicksByIndexFromTheOldest(t *testing.T) {
	t.Parallel()

	m := roundsview.New(ui.New(theme.Macchiato()))
	m.Open("kxqpmnzo", list(), 5, -1)

	m.Update(tuitest.Key("j"))
	m.Update(tuitest.Key("j"))

	cmd := m.Update(tuitest.Key("enter"))
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}

	if got, ok := cmd().(roundsview.PickedMsg); !ok || got.Index != 2 {
		t.Errorf("enter on the second round down sent %#v, want index 2", cmd())
	}

	m.Open("kxqpmnzo", list(), 5, -1)

	if _, ok := m.Update(tuitest.Key("enter"))().(roundsview.WholeMsg); !ok {
		t.Error("enter on the top row did not ask for the whole change")
	}
}
