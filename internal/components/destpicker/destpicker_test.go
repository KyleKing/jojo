package destpicker_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/destpicker"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 60, 12

func testSkin() ui.Skin {
	return ui.New(theme.Macchiato())
}

//nolint:ireturn // T is concrete at every call site, only the signature is generic.
func drainMsg[T any](cmd tea.Cmd) (T, bool) {
	var zero T

	if cmd == nil {
		return zero, false
	}

	msg, ok := cmd().(T)

	return msg, ok
}

func rev(changeID, description string, opts ...func(*jj.Revision)) jj.LogRow {
	r := &jj.Revision{ChangeID: changeID, Description: description}
	for _, opt := range opts {
		opt(r)
	}

	return jj.LogRow{Revision: r}
}

func immutable(r *jj.Revision) { r.Immutable = true }

func bookmarks(names ...string) func(*jj.Revision) {
	return func(r *jj.Revision) { r.Bookmarks = names }
}

func graphLine() jj.LogRow {
	return jj.LogRow{Graph: &jj.GraphLine{Graph: "│"}}
}

func TestOpenSkipsGraphLinesAndExcludedChange(t *testing.T) {
	t.Parallel()

	m := destpicker.New(testSkin())
	m.Open("Squash into", []jj.LogRow{
		rev("aaa", "one"),
		graphLine(),
		rev("bbb", "two"),
		rev("ccc", "three"),
	}, "bbb")

	picked, ok := drainMsg[destpicker.PickedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit destpicker.PickedMsg")
	}

	if picked.Revision.ChangeID != "aaa" {
		t.Errorf("first pickable revision = %q, want %q", picked.Revision.ChangeID, "aaa")
	}
}

func TestEnterOnImmutableRevisionRefusesAndWarns(t *testing.T) {
	t.Parallel()

	m := destpicker.New(testSkin())
	m.Open("Squash into", []jj.LogRow{rev("aaa", "trunk", immutable)}, "")

	if _, ok := drainMsg[destpicker.PickedMsg](m.Update(tuitest.Key("enter"))); ok {
		t.Fatal("Update(enter) on an immutable revision emitted destpicker.PickedMsg")
	}

	if !strings.Contains(m.View(testWidth, testHeight), "immutable") {
		t.Error("View() does not explain why the immutable revision was refused")
	}
}

func TestEscCancelsWithoutPicking(t *testing.T) {
	t.Parallel()

	m := destpicker.New(testSkin())
	m.Open("Squash into", []jj.LogRow{rev("aaa", "one")}, "")

	if _, ok := drainMsg[destpicker.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("Update(esc) did not emit destpicker.CancelledMsg")
	}
}

func TestFilterNarrowsSmartCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"change id substring", "bb", "bbb"},
		{"bookmark substring", "feat", "ccc"},
		{"description substring", "fix the", "ddd"},
		{"lowercase query matches any case", "trunk", "eee"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := destpicker.New(testSkin())
			m.Open("Squash into", []jj.LogRow{
				rev("aaa", "one"),
				rev("bbb", "two"),
				rev("ccc", "three", bookmarks("feature-x")),
				rev("ddd", "fix the bug"),
				rev("eee", "TRUNK marker"),
			}, "")

			for _, r := range tc.query {
				m.Update(tuitest.Key(string(r)))
			}

			picked, ok := drainMsg[destpicker.PickedMsg](m.Update(tuitest.Key("enter")))
			if !ok {
				t.Fatalf("Update(enter) after filtering %q did not emit destpicker.PickedMsg", tc.query)
			}

			if picked.Revision.ChangeID != tc.want {
				t.Errorf("filter %q picked %q, want %q", tc.query, picked.Revision.ChangeID, tc.want)
			}
		})
	}
}

func TestUppercaseQueryIsCaseSensitive(t *testing.T) {
	t.Parallel()

	m := destpicker.New(testSkin())
	m.Open("Squash into", []jj.LogRow{
		rev("aaa", "TRUNK marker"),
		rev("bbb", "trunk marker"),
	}, "")

	for _, r := range "TRUNK" {
		m.Update(tuitest.Key(string(r)))
	}

	picked, ok := drainMsg[destpicker.PickedMsg](m.Update(tuitest.Key("enter")))
	if !ok {
		t.Fatal("Update(enter) did not emit destpicker.PickedMsg")
	}

	if picked.Revision.ChangeID != "aaa" {
		t.Errorf("uppercase query picked %q, want %q (case-sensitive match only)", picked.Revision.ChangeID, "aaa")
	}
}

func TestListReportsScrollPositionWhenClipped(t *testing.T) {
	t.Parallel()

	rows := make([]jj.LogRow, 0, 20)
	for i := range 20 {
		rows = append(rows, rev(fmt.Sprintf("c%02d", i), "row"))
	}

	m := destpicker.New(testSkin())
	m.Open("Squash into", rows, "")

	view := m.View(testWidth, testHeight)
	if !strings.Contains(view, "of 20") {
		t.Errorf("View() = %q, want a clipped list to report its position out of 20", view)
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := destpicker.New(testSkin())
	m.Open("Squash into", []jj.LogRow{
		rev("aaa", "one", bookmarks("main")),
		rev("bbb", "two", immutable),
		rev("ccc", "three"),
	}, "")
	m.Update(tuitest.Key("down"))

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
