package model

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// ]x and [x step the conflicted revisions, ]b and [b the bookmarked ones, and running out says so.
func TestMotions_RowObjects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		from, want, said string
		keys             []string
	}{
		{from: "wkwyyxvq", keys: []string{"]", "x"}, want: "koopqtrk"},
		{from: "nptwozlo", keys: []string{"[", "x"}, want: "koopqtrk"},
		{from: "koopqtrk", keys: []string{"]", "x"}, want: "koopqtrk", said: "no conflicted revision below"},
		{from: "wkwyyxvq", keys: []string{"]", "b"}, want: "twnsvwmr"},
		{from: "nptwozlo", keys: []string{"[", "b"}, want: "twnsvwmr"},
		{from: "twnsvwmr", keys: []string{"[", "b"}, want: "twnsvwmr", said: "no bookmarked revision above"},
	} {
		t.Run(tc.from+strings.Join(tc.keys, ""), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, [2]int{100, 30}, tc.from)
			m, _ = pressOne(t, m, tc.keys[0])
			m, msgs := pressOne(t, m, tc.keys[1])

			if got := m.log.Selected().ChangeID; got != tc.want {
				t.Errorf("the cursor is on %s, want %s", got, tc.want)
			}

			if tc.said != "" && !strings.Contains(said(msgs), tc.said) {
				t.Errorf("said %q, want %q", said(msgs), tc.said)
			}
		})
	}
}

// In a picker ]b moves the destination, not the log's cursor.
func TestMotions_BookmarkInThePicker(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "wkwyyxvq")
	m, _ = pressOne(t, m, "y")
	m, _ = pressOne(t, m, "]")
	m, _ = pressOne(t, m, "b")

	if m.pick.dest != "twnsvwmr" || m.log.Selected().ChangeID != "wkwyyxvq" {
		t.Errorf("]b in the picker left the destination on %s and the cursor on %s", m.pick.dest,
			m.log.Selected().ChangeID)
	}
}

// [[ steps to the first parent and ]] back up to a child, in the log and as a picker's destination.
func TestMotionsLive_GraphEdges(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "middle"), "[", "[")

	if got := m.log.Selected().Description; got != "bottom" {
		t.Fatalf("[[ from middle went to %q, want bottom", got)
	}

	m = press(t, m, "]", "]")
	if got := m.log.Selected().Description; got != "middle" && got != "side" {
		t.Errorf("]] from bottom went to %q, want one of its children", got)
	}

	m = press(t, liveModel(t, dir, "top"), "y", "[", "[")
	if m.pick.dest != changeFor(t, dir, "middle") {
		t.Errorf("[[ in the duplicate picker chose %q, want middle", m.pick.dest)
	}
}

// The repeat key with nothing written says so; after a squash into the parent it squashes the next revision into
// its own parent, and after a duplicate onto a chosen revision it copies the next one there too.
func TestRepeatLive(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "")
	if _, msgs := pressOne(t, m, "."); !strings.Contains(said(msgs), "nothing to repeat yet") {
		t.Errorf(". before any write said %q", said(msgs))
	}

	dir := liveRepo(t)
	m = press(t, liveModel(t, dir, "top"), "s", "enter")

	if changeFor(t, dir, "top") != "" {
		t.Fatal("squashing top into its parent left it")
	}

	if !m.log.Focus(changeFor(t, dir, "side")) {
		t.Fatal("no side row")
	}

	press(t, m, ".")

	if changeFor(t, dir, "side") != "" {
		t.Error(". did not squash side into its own parent")
	}

	dir = liveRepo(t)
	m = press(t, liveModel(t, dir, "top"), "y")
	m.pick.dest, m.pick.inPlace = changeFor(t, dir, "side"), false
	m = pump(t, m, m.changed())
	m = press(t, m, "enter")

	if !m.log.Focus(changeFor(t, dir, "middle")) {
		t.Fatal("no middle row")
	}

	press(t, m, ".")

	onSide := jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r",
		"children(description(exact:'side\n'))", "-T", "description.first_line() ++ \"\\n\"")
	for _, want := range []string{"top", "middle"} {
		if !strings.Contains(onSide, want) {
			t.Errorf("side's children are %q, want a copy of %s", onSide, want)
		}
	}
}

// The log after ]x, which is how the motions read on screen: the cursor on the conflict, and the
// coach naming x.
func TestMotions_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, size, "wkwyyxvq")
			m, _ = pressOne(t, m, "]")
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
			assertVerbFrame(t, m.render(), size)

			m, _ = pressOne(t, m, "x")
			if m.log.Selected().ChangeID != "koopqtrk" {
				t.Errorf("]x landed on %s", m.log.Selected().ChangeID)
			}
		})
	}
}
