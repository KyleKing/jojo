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

// A prefix waits with its which-key panel up, the frame keeps the terminal's height, and Esc drops
// the prefix and the panel together, at every size.
func TestWhichKey_ShowsWhileAPrefixWaits(t *testing.T) {
	t.Parallel()

	for _, size := range []struct{ w, h int }{{100, 30}, {80, 24}} {
		t.Run(strconv.Itoa(size.w)+"x"+strconv.Itoa(size.h), func(t *testing.T) {
			t.Parallel()

			m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: size.w, Height: size.h}))
			m = asModel(t, update(m, tuitest.Key("g")))

			frame := tuitest.StripANSI(m.render())
			if !strings.Contains(frame, "forge pane") || !strings.Contains(frame, "g…") {
				t.Fatalf("g waits without its panel:\n%s", frame)
			}

			if got := len(strings.Split(frame, "\n")); got != size.h {
				t.Errorf("the panel made the frame %d rows, want %d", got, size.h)
			}

			m = asModel(t, update(m, tuitest.Key("esc")))
			if strings.Contains(tuitest.StripANSI(m.render()), "forge pane") || m.matcher.Pending() != nil {
				t.Error("esc left the panel or the prefix up")
			}
		})
	}
}

// The picker's - panel leaves both footer rows in view: the live command line is what the flags
// change, so covering it while choosing one hides the answer.
func TestWhichKey_KeepsThePickerCommandLine(t *testing.T) {
	t.Parallel()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: 100, Height: 30}))
	m = asModel(t, pressKeys(t, m, "r", "-"))

	lines := strings.Split(tuitest.StripANSI(m.render()), "\n")
	if cmd := lines[len(lines)-2]; !strings.Contains(cmd, "jj rebase -r aaaa") {
		t.Errorf("the command line is covered: %q", cmd)
	}

	if !strings.Contains(strings.Join(lines, "\n"), "the whole branch (-b)") {
		t.Error("the - panel does not list the rebase flags")
	}
}

// A [keys] table in old names still starts, and says once in the footer what to rename.
func TestDeprecatedKeysWarnOnceAtStart(t *testing.T) {
	t.Parallel()

	km, err := help.NewKeymap(map[string]string{"dock-diff": "ctrl+t"})
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}

	opts := testOptions(t)
	opts.Keys = km
	m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", opts)

	var said []string

	for _, msg := range drain(m.Init()) {
		if shown, ok := msg.(messageMsg); ok {
			said = append(said, shown.text)
		}
	}

	if len(said) != 1 || !strings.Contains(said[0], "dock-diff is preview now") {
		t.Errorf("Init said %q, want one warning naming dock-diff and preview", said)
	}
}

// ]f in the focused dock reaches the pane through the shared keymap.
func TestDock_MotionsComeFromTheKeymap(t *testing.T) {
	t.Parallel()

	m := dockedModel(t, 120, 20)
	m = asModel(t, pressKeys(t, m, "tab", "]", "f"))

	if got := m.dock.CurrentPath(); got != "second.txt" {
		t.Errorf("]f in the dock left it in %q, want second.txt", got)
	}

	m = asModel(t, pressKeys(t, m, "[", "f"))
	if got := m.dock.CurrentPath(); got != "first.txt" {
		t.Errorf("[f in the dock left it in %q, want first.txt", got)
	}
}

// Every footer hint the log draws is a key the keymap binds, so the footer cannot offer a key that
// does nothing.
func TestFooter_OffersOnlyBoundKeys(t *testing.T) {
	t.Parallel()

	m := testModel(t, twoRows())
	bound := map[string]bool{}

	for _, b := range m.keys.Keys().Bindings() {
		if len(b.Keys) > 0 {
			bound[b.Label()] = true
		}
	}

	for _, h := range m.footerHints() {
		if !bound[h.Key] {
			t.Errorf("the footer offers %q (%s), which nothing binds", h.Key, h.What)
		}
	}
}
