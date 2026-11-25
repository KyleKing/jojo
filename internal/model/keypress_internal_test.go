package model

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/logview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// benchRows is a log the size of a working stack over some history: a graph line between each
// revision, a bookmark every few rows, and descriptions long enough to be cut at 140 columns.
func benchRows(n int) []jj.LogRow {
	rows := make([]jj.LogRow, 0, 2*n)

	for i := range n {
		r := &jj.Revision{
			ChangeID:     fmt.Sprintf("%c%c%c%cqzsm", 'k'+i%10, 'l'+i%7, 'm'+i%5, 'n'+i%3),
			FullChangeID: fmt.Sprintf("full%04dqzsmyzxkqrnkttwlvulq", i),
			CommitID:     fmt.Sprintf("%08x", 0xabc000+i),
			Description:  fmt.Sprintf("feat(dock): change number %d reads the diff once per commit id", i),
			AuthorEmail:  "dev@example.com",
			Age:          fmt.Sprintf("%d hours ago", i),
			Graph:        "○",
			Added:        i % 40, Removed: i % 13, Files: 1 + i%5,
			Immutable: i > n/2,
		}
		if i == 0 {
			r.WorkingCopy, r.Graph = true, "@"
		}

		if i%6 == 3 {
			r.LocalBookmarks = []string{fmt.Sprintf("push-%d", i)}
			r.Bookmarks = r.LocalBookmarks
		}

		rows = append(rows, jj.LogRow{Revision: r}, jj.LogRow{Graph: &jj.GraphLine{Graph: "│"}})
	}

	return rows
}

// benchDiff is a docked diff of a few files and hunks, what a typical change shows.
func benchDiff(rev string) string {
	var out strings.Builder

	for f := range 4 {
		fmt.Fprintf(&out, "diff --git a/%s/file%d.go b/%s/file%d.go\nindex 0000000..1111111 100644\n"+
			"--- a/%s/file%d.go\n+++ b/%s/file%d.go\n@@ -1,6 +1,7 @@\n", rev, f, rev, f, rev, f, rev, f)
		for l := range 6 {
			fmt.Fprintf(&out, " context line %d of the file that did not change\n", l)
		}

		out.WriteString("-func old() int { return 1 }\n+func new() int { return 2 }\n+// and one line more\n")
	}

	return out.String()
}

// keyModel is the app at 140x40 over a 60-revision log, docked or not, as a user sees it after launch.
func keyModel(tb testing.TB, docked bool) Model {
	tb.Helper()

	keys, err := help.NewKeymap(nil)
	if err != nil {
		tb.Fatalf("NewKeymap: %v", err)
	}

	m := New(tb.Context(), jj.NewClient(tb.TempDir()), ui.New(theme.Macchiato()), tb.TempDir(), "@",
		Options{Keys: keys})
	m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: benchRows(60)})
	m = benchUpdate(m, tea.WindowSizeMsg{Width: 140, Height: 40})

	if docked {
		m = benchUpdate(m, tuitest.Key("p"))
		m = benchUpdate(m, dockLoadedMsg{revision: m.dockTarget().ChangeID, text: benchDiff("a"), seq: m.dockSeq})
	}

	return m
}

func benchUpdate(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	model, _ := next.(Model) //nolint:errcheck // the root always answers with a Model.

	return model
}

// keyStep is one j or k as the program runs it: the key, the cursor move it reports, a frame after
// each, and with the dock open the rested cursor's diff landing, drawn too. No command runs, since
// those are jj's time rather than jojo's.
func keyStep(m Model, key string, docked bool, diff string) Model {
	m = benchUpdate(m, tuitest.Key(key))
	_ = m.View()

	m = benchUpdate(m, logview.SelectedMsg{Revision: m.log.Selected()})
	_ = m.View()

	if docked {
		m = benchUpdate(m, dockLoadedMsg{revision: m.dockTarget().ChangeID, text: diff, seq: m.dockSeq})
		_ = m.View()
	}

	return m
}

// benchKeys runs j and k in turn, alternating two diffs so the dock never draws the one it holds.
func benchKeys(b *testing.B, docked bool) {
	b.Helper()

	m := keyModel(b, docked)
	diffs := [2]string{benchDiff("a"), benchDiff("b")}
	keys := [2]string{"j", "k"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		m = keyStep(m, keys[i%2], docked, diffs[i%2])
	}
}

// BenchmarkModel_KeyJ is the whole app's cost for one cursor move: what each j or k spends in
// Update and View with the log alone.
func BenchmarkModel_KeyJ(b *testing.B) { benchKeys(b, false) }

// BenchmarkModel_KeyJDocked is the same move with the diff docked beside the log.
func BenchmarkModel_KeyJDocked(b *testing.B) { benchKeys(b, true) }

// keyBudget is what one cursor move may cost. The model was 700 KB by value once, copied whole into
// every handler and every footer hint, and a keypress cost 86 ms of CPU; these bounds are a few times
// what a move costs now, so a regression of that kind fails here rather than in a user's terminal.
//
//nolint:gochecknoglobals // a fixed table, read only.
var keyBudget = []struct {
	name   string
	docked bool
	allocs float64
	bytes  uint64
}{
	{name: "log", allocs: 10_000, bytes: 2 << 20},
	{name: "docked", docked: true, allocs: 45_000, bytes: 6 << 20},
}

// modelBudget bounds the root model by value, which Bubble Tea copies on every message.
const modelBudget = 16 << 10

func TestModel_StaysSmallByValue(t *testing.T) {
	t.Parallel()

	if size := unsafe.Sizeof(Model{}); size > modelBudget {
		t.Errorf("Model is %d bytes by value, over the %d budget: hold large fields by pointer", size, modelBudget)
	}
}

//nolint:paralleltest // testing.AllocsPerRun refuses to run beside a parallel test.
func TestModel_KeyPressBudget(t *testing.T) {
	for _, tc := range keyBudget {
		t.Run(tc.name, func(t *testing.T) {
			m := keyModel(t, tc.docked)
			diffs := [2]string{benchDiff("a"), benchDiff("b")}
			i := 0

			step := func() {
				m = keyStep(m, [2]string{"j", "k"}[i%2], tc.docked, diffs[i%2])
				i++
			}

			if allocs := testing.AllocsPerRun(20, step); allocs > tc.allocs {
				t.Errorf("a cursor move made %.0f allocations, over the %.0f budget", allocs, tc.allocs)
			}

			var before, after runtime.MemStats

			const runs = 20

			runtime.ReadMemStats(&before)

			for range runs {
				step()
			}

			runtime.ReadMemStats(&after)

			if per := (after.TotalAlloc - before.TotalAlloc) / runs; per > tc.bytes {
				t.Errorf("a cursor move allocated %d bytes, over the %d budget", per, tc.bytes)
			}
		})
	}
}
