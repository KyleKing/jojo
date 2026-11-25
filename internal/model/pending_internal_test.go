package model

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

func testOptions(t *testing.T) Options {
	t.Helper()

	keys, err := help.NewKeymap(nil)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	return Options{Keys: keys}
}

func testModel(t *testing.T, rows []jj.LogRow) Model {
	t.Helper()

	m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", testOptions(t))
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: rows})

	return m
}

func twoRows() []jj.LogRow {
	return []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa", Description: "source"}},
		{Revision: &jj.Revision{ChangeID: "bbbb", Description: "destination"}},
	}
}

// Every verb that picks a destination opens the in-log picker over the log, and Enter runs what it
// built and leaves picking mode, so a second Enter cannot run it again.
func TestPicker_EveryVerbOpensItAndEnterLeavesIt(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"s", "r", "R", "y", "N"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			opened := asModel(t, update(testModel(t, twoRows()), tuitest.Key(key)))
			if !opened.pick.active {
				t.Fatal("the picker did not open")
			}

			if opened.destPicker.IsOpen() {
				t.Error("the old destination overlay opened as well")
			}

			done, cmd := opened.applyPick()
			if cmd == nil {
				t.Error("enter ran nothing")
			}

			if done.pick.active {
				t.Error("the picker stayed open after running, so the next enter would run it again")
			}
		})
	}
}

// The picker builds the exact command the footer shows, from the verb, the flags, and the row under
// the cursor. Each case is the key sequence a reader would press and the command it must leave.
func TestPicker_BuildsTheCommandTheFlagsName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		keys []string
	}{
		{name: "rebase lands on the row below", keys: []string{"r"}, want: "jj rebase -r aaaa --onto bbbb"},
		{name: "-s moves descendants", keys: []string{"r", "-", "s"}, want: "jj rebase -s aaaa --onto bbbb"},
		{name: "a bare s is -s", keys: []string{"r", "s"}, want: "jj rebase -s aaaa --onto bbbb"},
		{name: "a bare r is -r again", keys: []string{"r", "s", "r"}, want: "jj rebase -r aaaa --onto bbbb"},
		{name: "-B is before", keys: []string{"r", "-", "B"}, want: "jj rebase -r aaaa --insert-before bbbb"},
		{
			name: "-b is the branch", keys: []string{"r", "-", "b", "-", "A"},
			want: "jj rebase -b aaaa --insert-after bbbb",
		},
		{name: "-e skips emptied", keys: []string{"r", "-", "e"}, want: "jj rebase -r aaaa --onto bbbb --skip-emptied"},
		{
			name: "-d and -p are jj's own toggles", keys: []string{"r", "-", "d", "-", "p"},
			want: "jj rebase -r aaaa --onto bbbb --keep-divergent --simplify-parents",
		},
		{
			name: "squash keeps the destination message by default", keys: []string{"s"},
			want: "jj squash --from aaaa --into bbbb --use-destination-message",
		},
		{
			name: "squash -k keeps the emptied source, -u off combines messages",
			keys: []string{"s", "-", "k", "-", "u"},
			want: "jj squash --from aaaa --into bbbb --keep-emptied",
		},
		{
			name: "squash -i chooses hunks", keys: []string{"s", "-", "i"},
			want: "jj squash --from aaaa --into bbbb --use-destination-message --interactive",
		},
		{
			name: "squash -A squashes into a new commit after", keys: []string{"s", "-", "A"},
			want: "jj squash --from aaaa --insert-after bbbb",
		},
		{
			name: "squash back to -t goes into it again", keys: []string{"s", "-", "o", "-", "t"},
			want: "jj squash --from aaaa --into bbbb --use-destination-message",
		},
		{name: "duplicate starts in place", keys: []string{"y"}, want: "jj duplicate aaaa"},
		{name: "moving a duplicate places it", keys: []string{"y", "j"}, want: "jj duplicate aaaa --onto bbbb"},
		{name: "duplicate after", keys: []string{"y", "j", "-", "A"}, want: "jj duplicate aaaa --insert-after bbbb"},
		{name: "backspace puts it back in place", keys: []string{"y", "j", "backspace"}, want: "jj duplicate aaaa"},
		{name: "new on the cursor", keys: []string{"N"}, want: "jj new aaaa"},
		{
			name: "new before, no edit", keys: []string{"N", "-", "B", "-", "n"},
			want: "jj new --insert-before aaaa --no-edit",
		},
		{name: "new elsewhere", keys: []string{"N", "j"}, want: "jj new bbbb"},
		{name: "restore starts on the parent", keys: []string{"R"}, want: "jj restore --from bbbb --into aaaa"},
		{
			name: "restore -d keeps descendants", keys: []string{"R", "-", "d"},
			want: "jj restore --from bbbb --into aaaa --restore-descendants",
		},
		{
			name: "a flag another verb has does nothing", keys: []string{"R", "-", "o"},
			want: "jj restore --from bbbb --into aaaa",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var model tea.Model = testModel(t, twoRows())
			for _, key := range tc.keys {
				model = update(asModel(t, model), tuitest.Key(key))
			}

			if got := commandOf(asModel(t, model)); got != tc.want {
				t.Errorf("command = %q, want %q", got, tc.want)
			}
		})
	}
}

// What moves cannot be its own destination, so the cursor steps over it in either direction, and it
// stays put rather than landing on a source at either end.
func TestPicker_CursorStepsOverWhatMoves(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa"}},
		{Revision: &jj.Revision{ChangeID: "bbbb"}},
		{Graph: &jj.GraphLine{Graph: "│"}},
		{Revision: &jj.Revision{ChangeID: "cccc"}},
	}

	var model tea.Model = testModel(t, rows)
	model = update(asModel(t, model), tuitest.Key("j"))
	model = update(asModel(t, model), tuitest.Key("r"))

	if got := asModel(t, model).pick.dest; got != "cccc" {
		t.Fatalf("rebasing bbbb started on %q, want the next row cccc", got)
	}

	model = update(asModel(t, model), tuitest.Key("k"))
	if got := asModel(t, model).pick.dest; got != "aaaa" {
		t.Errorf("k from cccc landed on %q, want aaaa past the source", got)
	}

	model = update(asModel(t, model), tuitest.Key("k"))
	if got := asModel(t, model).pick.dest; got != "aaaa" {
		t.Errorf("k at the top moved to %q", got)
	}
}

// Esc leaves picking mode without running anything, and says so.
func TestPicker_EscRunsNothing(t *testing.T) {
	t.Parallel()

	var model tea.Model = testModel(t, twoRows())
	model = update(asModel(t, model), tuitest.Key("r"))

	next, cmd := asModel(t, model).Update(tuitest.Key("esc"))
	if asModel(t, next).pick.active {
		t.Fatal("esc left the picker open")
	}

	if cmd == nil {
		t.Fatal("esc said nothing")
	}

	for _, msg := range drain(cmd) {
		if shown, ok := msg.(messageMsg); ok && shown.wrote {
			t.Errorf("esc reported a write: %q", shown.text)
		}
	}
}

// Rebasing a stack onto trunk() is the core fetch-rebase-push loop, and trunk() is immutable once it
// tracks a remote, so the picker lets the cursor land there and leaves the verdict to jj.
func TestRebaseOntoTrunkPicksAnImmutableDestination(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{
		{Revision: &jj.Revision{ChangeID: "aaaa", Description: "mine"}},
		{Revision: &jj.Revision{ChangeID: "zzzz", Description: "trunk", Immutable: true}},
	}

	next := update(testModel(t, rows), tuitest.Key("r"))
	if got := commandOf(asModel(t, next)); got != "jj rebase -r aaaa --onto zzzz" {
		t.Errorf("rebase onto trunk built %q", got)
	}
}

// A preview that lands after the destination moved on is for a command nobody is looking at, and
// must not be drawn as though it were.
func TestPicker_DropsAPreviewForAnEarlierCommand(t *testing.T) {
	t.Parallel()

	var model tea.Model = testModel(t, threeRows())
	model = update(asModel(t, model), tuitest.Key("r"))
	stale := commandOf(asModel(t, model))
	model = update(asModel(t, model), tuitest.Key("j"))

	session := asModel(t, model).pick.session
	rows := []jj.LogRow{{Revision: &jj.Revision{ChangeID: "bbbb"}}}
	model = update(asModel(t, model), previewMsg{command: stale, session: session, preview: jj.Preview{Rows: rows}})

	if got := asModel(t, model).pick.preview; got != nil {
		t.Errorf("a preview for %q was drawn after the destination moved on", stale)
	}
}

// drain runs cmd and every command a batch or sequence inside it holds, returning the messages that
// come back. Ticks are skipped rather than waited out, since nothing here needs them to fire.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}

		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func commandOf(m Model) string { return m.pick.command() }

// The picker's frame keeps the terminal's shape at every size: as many lines as rows, none wider
// than the terminal, and Enter and Esc still on the footer however many flags it sheds.
func TestPicker_FitsTheTerminalAtEverySize(t *testing.T) {
	t.Parallel()

	sizes := []struct{ width, height int }{{100, 30}, {80, 24}, {60, 20}, {40, 12}}

	for _, verb := range []string{"r", "s", "y", "N", "R"} {
		for _, size := range sizes {
			t.Run(verb+"/"+strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
				t.Parallel()

				m := asModel(t, update(testModel(t, threeRows()),
					tea.WindowSizeMsg{Width: size.width, Height: size.height}))
				m = asModel(t, update(m, tuitest.Key(verb)))

				frame := tuitest.StripANSI(m.render())
				// The revset box's input pads one cell past the width, which the renderer clips; the
				// rows below it are the picker's.
				assertFits(t, frame, size.width, size.height, 1)

				lines := strings.Split(frame, "\n")
				footer := lines[len(lines)-1]
				if !strings.Contains(footer, "enter") || !strings.Contains(footer, "esc") {
					t.Errorf("the footer lost enter or esc: %q", footer)
				}
			})
		}
	}
}

// A chord's first key waits for the second, and the pair is spent once it arrives: [ then x does
// nothing, and does not leave [ waiting to swallow the next key.
func TestChord_SecondKeyEndsIt(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tuitest.Key("[")))
	if got := m.matcher.Pending(); len(got) != 1 || got[0] != "[" {
		t.Fatalf("[ was not held, pending = %q", got)
	}

	m = asModel(t, update(m, tuitest.Key("x")))
	if got := m.matcher.Pending(); got != nil {
		t.Errorf("the chord outlived its second key: %q", got)
	}

	m = asModel(t, update(m, tuitest.Key("j")))
	if got := m.log.Selected().ChangeID; got != "bbbb" {
		t.Errorf("j after a spent chord did not move the cursor, it is on %q", got)
	}
}

// The rebase picker's footer reads "-r revs -s source -b branch", and a bare s did nothing at all. The
// s and r keys now pick their mode; b is the family's prefix, so it says to press - b, and so does any
// flag letter pressed bare, rather than doing nothing silently.
func TestRebasePicker_BareModeLettersAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		want string
		keys []string
	}{
		{"flags start with -: - b is -b, the whole branch", []string{"r", "b"}},
		{"flags start with -: - e is --skip-emptied", []string{"r", "e"}},
	} {
		m := testModel(t, twoRows())
		var msgs []tea.Msg

		for _, key := range tc.keys {
			m, msgs = pressOne(t, m, key)
		}

		if got := said(msgs); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%v said %q, want %q", tc.keys, got, tc.want)
		}

		if got := commandOf(m); got != "jj rebase -r aaaa --onto bbbb" {
			t.Errorf("%v changed the command to %q", tc.keys, got)
		}
	}
}
