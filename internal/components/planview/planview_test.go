package planview_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/planview"
	"github.com/kyleking/jojo/internal/stack"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

var errNoTrunk = errors.New("trunk() is on no remote bookmark")

func plan() stack.Plan {
	changes := make([]stack.Change, 0, 6)
	for _, id := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc", "dddddddd", "eeeeeeee", "ffffffff"} {
		changes = append(changes, stack.Change{ID: id, LongID: id + "0000", Subject: "change " + id})
	}

	return stack.Build(stack.Input{Trunk: "main", Changes: changes, Draft: true})
}

func TestView_FillsItsRectInEveryState(t *testing.T) {
	t.Parallel()

	m := planview.New(ui.New(theme.Macchiato()))
	m.SetSize(60, 12)

	// In order: the plan last, which is what the view is left showing.
	states := []struct {
		set  func()
		name string
	}{
		{name: "loading", set: m.SetLoading},
		{name: "error", set: func() { m.SetLoading(); m.SetErr(errNoTrunk) }},
		{name: "plan", set: func() { m.SetPlan(plan()) }},
	}

	for _, st := range states {
		st.set()

		lines := strings.Split(tuitest.StripANSI(m.View()), "\n")
		if len(lines) != 12 {
			t.Errorf("%s: %d lines, want 12", st.name, len(lines))
		}
	}

	if m.Plan() == nil || m.Loading() || m.Err() != nil {
		t.Fatal("SetPlan left the view without a plan")
	}
}

// A plan longer than the view scrolls, and the title says how much is below.
func TestView_ScrollsAndSaysWhatIsBelow(t *testing.T) {
	t.Parallel()

	m := planview.New(ui.New(theme.Macchiato()))
	m.SetSize(70, 8)
	m.SetPlan(plan())

	if !strings.Contains(tuitest.StripANSI(m.View()), "more ↓") {
		t.Errorf("a clipped plan does not say so:\n%s", tuitest.StripANSI(m.View()))
	}

	if !m.Scroll(3) || !m.Scroll(-10) || m.Scroll(-1) {
		t.Error("Scroll moved wrong: down 3, back to the top, then no further")
	}

	m.ScrollTo(-1)

	view := tuitest.StripANSI(m.View())
	if !strings.Contains(view, "gh pr edit push-ffffffff0000") || strings.Contains(view, "more ↓") {
		t.Errorf("the bottom does not show the last command:\n%s", view)
	}

	m.SetRunning(true)

	if !m.Running() {
		t.Error("SetRunning did not stick")
	}
}
