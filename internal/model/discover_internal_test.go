package model

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/trail"
	"github.com/kyleking/aragonite/tui/palette"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

// flatten runs cmd and every command a batch of it holds, answering the messages they send.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}

	var out []tea.Msg
	for _, c := range batch {
		out = append(out, flatten(c)...)
	}

	return out
}

// said is every status line msgs put up.
func said(msgs []tea.Msg) string {
	var lines []string

	for _, msg := range msgs {
		if m, ok := msg.(messageMsg); ok {
			lines = append(lines, m.text)
		}
	}

	return strings.Join(lines, "\n")
}

// pressOne sends one key and answers the model and what its commands sent.
func pressOne(t *testing.T, m Model, key string) (Model, []tea.Msg) {
	t.Helper()

	next, cmd := m.Update(tuitest.Key(key))

	return asModel(t, next), flatten(cmd)
}

// stackRows is a log with something for every rule: @ with changes and no description, a conflicted
// revision under it, a bookmark ahead of its remote, and an immutable trunk.
func stackRows() []jj.LogRow {
	return []jj.LogRow{
		{Revision: &jj.Revision{
			ChangeID: "wkwyyxvq", WorkingCopy: true, Files: 2, Added: 14, Removed: 3, CommitID: "7f3a91c2",
		}},
		{Revision: &jj.Revision{ChangeID: "rnzxmskv", Description: "parse: read the new header format", Files: 6}},
		{Revision: &jj.Revision{ChangeID: "koopqtrk", Description: "refactor: split the reader", Conflict: true}},
		{Revision: &jj.Revision{
			ChangeID: "twnsvwmr", Description: "test: cover the fallbacks", Bookmarks: []string{"parse*"},
			LocalBookmarks: []string{"parse"},
		}},
		{Revision: &jj.Revision{ChangeID: "nptwozlo", Description: "release 0.13.0", Immutable: true}},
	}
}

// A key with nothing to act on under the cursor says why in the status line rather than doing
// nothing, and the footer draws it flat.
func TestGate_ARefusedKeySaysWhy(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ key, want string }{
		{"x", "x  aaaa has no conflict"},
		{"|", "|  needs two or more checked"},
		{"c", "c  commits @, the working copy"},
		{"B", "B  needs exactly two checked"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))
			after, msgs := pressOne(t, m, tc.key)

			if got := said(msgs); !strings.Contains(got, tc.want) {
				t.Errorf("pressing %s said %q, want %q", tc.key, got, tc.want)
			}

			if after.pick.active || after.files.IsOpen() || after.diff.IsOpen() {
				t.Errorf("a refused %s still opened something", tc.key)
			}
		})
	}

	m := testModel(t, twoRows())
	if h := m.keys.HintsAt(m.gate, help.ActionResolve); len(h) != 1 || !h[0].Off || h[0].Why == "" {
		t.Errorf("the footer's resolve hint is %+v, want it off with its reason", h)
	}
}

// `:` searches the actions by their words, git's included, and Enter runs the action by name: amend
// finds squash, which opens its picker, and the status line teaches the key that would have done it.
func TestPalette_RunsTheActionAndTeachesItsKey(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))
	m, _ = pressOne(t, m, ":")

	if !m.pal.IsOpen() || m.pal.Mode() != palette.Run {
		t.Fatal(": did not open the command palette")
	}

	for _, k := range []string{"a", "m", "e", "n", "d"} {
		m, _ = pressOne(t, m, k)
	}

	if sel, ok := m.pal.Selected(); !ok || sel.Action != help.ActionSquash {
		t.Fatalf("amend selects %+v, want squash", sel)
	}

	m, msgs := pressOne(t, m, "enter")
	if m.pal.IsOpen() {
		t.Error("the palette stayed open after running")
	}

	var run palette.RunMsg

	for _, msg := range msgs {
		if r, ok := msg.(palette.RunMsg); ok {
			run = r
		}
	}

	next, cmd := m.Update(run)
	m = asModel(t, next)

	if !m.pick.active || m.pick.verb != pickSquash {
		t.Fatal("running squash from the palette did not open its picker")
	}

	if got := said(flatten(cmd)); !strings.Contains(got, "next time press s") {
		t.Errorf("the run said %q, want the key that would have done it", got)
	}
}

// A palette command typed by its name ranks first, over a key whose description says the same word.
func TestPalette_ACommandByNameRanksFirst(t *testing.T) {
	t.Parallel()

	m, _ := pressOne(t, testModel(t, twoRows()), ":")
	for _, k := range []string{"n", "e", "x", "t"} {
		m, _ = pressOne(t, m, k)
	}

	if sel, ok := m.pal.Selected(); !ok || sel.Action != cmdNext {
		t.Errorf("next selects %+v, want :next", sel)
	}
}

// Flags typed after a picker verb open its picker with them on: "squash -i" chooses hunks.
func TestPalette_FlagsPresetThePicker(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())
	next, _ := m.Update(palette.RunMsg{Action: help.ActionSquash, Key: "s", Flags: []string{"-i"}, From: palette.Run})
	m = asModel(t, next)

	if !m.pick.active || !m.pick.flags.On("i") || !m.pick.flags.On("u") {
		t.Errorf("squash -i opened with -i %v and -u %v, want both on", m.pick.flags.On("i"), m.pick.flags.On("u"))
	}
}

// `?` is the legend of the screen it opened on: a key pressed there explains itself rather than
// running, and the refused keys carry their reasons.
func TestHelp_AKeyExplainsItselfInsteadOfRunning(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 120, Height: 35}))
	m, _ = pressOne(t, m, "?")

	if !m.pal.IsOpen() || m.pal.Mode() != palette.Browse {
		t.Fatal("? did not open the legend")
	}

	m, _ = pressOne(t, m, "s")
	if m.pick.active {
		t.Fatal("s in the legend ran squash")
	}

	if e, ok := m.pal.Explained(); !ok || e.Action != help.ActionSquash || !strings.Contains(e.Cmd, "jj squash") {
		t.Errorf("s explained %+v, want squash and its jj command", e)
	}

	frame := tuitest.StripANSI(m.render())
	if !strings.Contains(frame, "jj squash --from <rev> --into <dest>") {
		t.Errorf("the explainer does not show the jj command:\n%s", frame)
	}

	entries := m.entries(palette.Browse)

	i := slices.IndexFunc(entries, func(e palette.Entry) bool { return e.Action == help.ActionResolve })
	if i < 0 || !entries[i].Off || !strings.Contains(entries[i].Why, "no conflict") {
		t.Errorf("resolve in the legend is %+v, want it off with its reason", entries)
	}
}

// The coach's next step takes the line over the footer while nothing on the row was cut, and its key
// is brightened in the footer; :next moves it into a band of every step instead.
func TestCoach_NextStepInTheInspectorAndTheBand(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, stackRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))

	frame := tuitest.StripANSI(m.render())
	if !strings.Contains(frame, "▸ next  @ has changes but no description · d describe") {
		t.Errorf("the inspector does not name the next step:\n%s", frame)
	}

	next, _ := m.Update(palette.RunMsg{Action: cmdNext, From: palette.Run})
	m = asModel(t, next)

	frame = tuitest.StripANSI(m.render())
	if !strings.Contains(frame, "── next") || strings.Contains(frame, "▸ next  @ has changes") {
		t.Errorf("the band did not replace the inline step:\n%s", frame)
	}

	if lines := strings.Split(frame, "\n"); len(lines) != 30 {
		t.Errorf("the frame with the band is %d rows, want 30", len(lines))
	}
}

// A tip shows only once the reader is idle past the session's first minute, stays until the next
// key, and any key clears it.
func TestCoach_TipShowsWhenIdleAndAnyKeyClearsIt(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, stackRows()), tea.WindowSizeMsg{Width: 120, Height: 30}))

	if _, ok := m.coach.Tip(time.Now(), false); ok {
		t.Fatal("a tip showed in the session's first minute")
	}

	at := time.Now().Add(2 * time.Minute)
	if _, ok := m.coach.Tip(at, false); !ok {
		t.Fatal("no tip showed once idle past the first minute")
	}

	if frame := tuitest.StripANSI(m.footer()); !strings.Contains(frame, "◇") {
		t.Errorf("the footer does not show the tip: %q", frame)
	}

	m, _ = pressOne(t, m, "j")
	if _, ok := m.coach.Showing(); ok {
		t.Error("a key left the tip showing")
	}
}

// Running an action from the palette three times teaches its key, and repeating edit then new
// teaches that new alone starts a change on top.
func TestCoach_DetectorsTeachTheShortWay(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())
	for range 3 {
		next, _ := m.Update(palette.RunMsg{Action: help.ActionCompare, Key: "=", From: palette.Run})
		m = asModel(t, next)
		m.diff.Close()
		m.destPicker.Close()
	}

	if tip, ok := m.coach.Tip(time.Now().Add(2*time.Minute), false); !ok || tip.Key != "=" {
		t.Errorf("three palette runs of compare tipped %+v, want its key", tip)
	}
}

// An empty revset draws a card saying what matched nothing and the keys out, in place of a blank log.
func TestEmptyLog_DrawsACardWithTheWayOut(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, nil), tea.WindowSizeMsg{Width: 100, Height: 30}))

	frame := tuitest.StripANSI(m.render())
	for _, want := range []string{`revset "@" matched nothing`, "L revset"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the empty log lacks %q:\n%s", want, frame)
		}
	}
}

// The frames the discoverability layer draws, at the two sizes the family is checked at: the log's
// footer and next step, `?`, `:` with a git word typed, the empty log, and gm.
func TestDiscover_Frames(t *testing.T) {
	t.Parallel()

	frames := map[string]struct {
		rows []jj.LogRow
		keys []string
	}{
		"log":     {rows: stackRows()},
		"help":    {rows: stackRows(), keys: []string{"?"}},
		"explain": {rows: stackRows(), keys: []string{"?", "s"}},
		"palette": {rows: stackRows(), keys: []string{":", "s", "t", "a", "s", "h"}},
		"empty":   {},
		"trail":   {rows: stackRows(), keys: []string{"g", "m"}},
	}

	for name, f := range frames {
		for _, size := range [][2]int{{80, 24}, {100, 30}} {
			t.Run(fmt.Sprintf("%s/%dx%d", name, size[0], size[1]), func(t *testing.T) {
				t.Parallel()

				m := asModel(t, update(testModel(t, f.rows), tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
				m.trail.Add(trail.Call{
					Argv: []string{"jj", "squash", "--from", "wkwyyxvq", "--into", "rnzxmskv", "-u"}, Write: true,
					Start: time.Date(2026, 11, 24, 9, 41, 0, 0, time.UTC), Took: 180 * time.Millisecond,
					Op: "3f2a9c01aa11",
				})
				m.trail.Add(trail.Call{
					Argv: []string{"jj", "git", "push", "-r", "twnsvwmr"}, Write: true, Exit: 1,
					Start: time.Date(2026, 11, 24, 9, 42, 0, 0, time.UTC), Took: 1200 * time.Millisecond,
					Stderr: "Error: Refusing to push a bookmark that moved both ways\n",
				})

				for _, k := range f.keys {
					m, _ = pressOne(t, m, k)
				}

				frame := m.render()
				if lines := strings.Split(frame, "\n"); len(lines) != size[1] {
					t.Errorf("the frame is %d rows, want %d", len(lines), size[1])
				}

				tuitest.AssertGolden(t, frame)
			})
		}
	}
}
