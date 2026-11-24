package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/seen"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// readsRepo is a real repository whose change "two" has three hunks: two far apart in a.txt and one
// in b.txt, on top of "base", with an empty @ above it.
func readsRepo(t *testing.T) string {
	t.Helper()

	dir := liveRepo(t)

	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}

	write := func(name, text string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("a.txt", strings.Join(lines, "\n")+"\n")
	write("b.txt", "bee\n")
	jjRun(t, dir, "commit", "-m", "base")

	lines[1], lines[24] = "line two", "line twenty-five"
	write("a.txt", strings.Join(lines, "\n")+"\n")
	write("b.txt", "bee, changed\n")
	jjRun(t, dir, "commit", "-m", "two")

	return dir
}

// readsModel is liveModel with a read-mark store kept in a directory of the test's own.
func readsModel(t *testing.T, dir, cursorOn string, store *seen.Store) Model {
	t.Helper()

	opts := testOptions(t)
	opts.Ticker = instantTicks
	opts.Reads = store

	m := New(t.Context(), jj.NewClient(dir), ui.New(theme.Macchiato()), dir, "all()", opts)
	m = asModel(t, update(m, tea.WindowSizeMsg{Width: 100, Height: 30}))
	m = pump(t, m, m.loadLog("all()"))

	if !m.log.Focus(changeFor(t, dir, cursorOn)) {
		t.Fatalf("no row described %q", cursorOn)
	}

	return m
}

// refsOf is a change's hunks as seen names them.
func refsOf(t *testing.T, dir, change string) []seen.Ref {
	t.Helper()

	return seen.Parse([]byte(jjRun(t, dir, "diff", "--git", "-r", change)))
}

// readPanes are the two diffs the read keys work in, each opened on the log's cursor: the dock beside
// the log (p, then tab to focus it) and the full-screen diff over it (enter). The read tests run in
// both, since the two share one code path and must answer every key the same way.
var readPanes = []struct {
	view func(*Model) readView
	name string
	open []string
}{
	{name: "dock", open: []string{"p", "tab"}, view: (*Model).dockReads},
	{name: "full screen", open: []string{"enter"}, view: (*Model).fullReads},
}

// Space on a hunk marks it read and on the next press unread; on a file's heading it marks the
// file. Every mark is on disk before the next key, where another process would find it.
func TestReads_SpaceMarksTheHunkThenTheFile(t *testing.T) {
	t.Parallel()

	for _, pane := range readPanes {
		t.Run(pane.name, func(t *testing.T) {
			t.Parallel()

			dir := readsRepo(t)
			store := seen.Open(t.TempDir())
			change := changeFor(t, dir, "two")
			refs := refsOf(t, dir, change)

			if len(refs) != 3 {
				t.Fatalf("two has %d hunks, want 3", len(refs))
			}

			m := press(t, readsModel(t, dir, "two", store), append(pane.open, "]", "h", "space")...)
			assertSpaceMarks(t, m, store, change, refs)
		})
	}
}

func assertSpaceMarks(t *testing.T, m Model, store *seen.Store, change string, refs []seen.Ref) {
	t.Helper()

	if !strings.HasPrefix(m.message, "hunk read · 2 hunks left") {
		t.Errorf("space said %q", m.message)
	}

	if seen.Open(store.Dir()).Unread(change, string(refs[0].ID)) {
		t.Error("the first hunk is not marked read on disk")
	}

	if s := m.strips[change]; s.Read != 1 || s.Total != 3 {
		t.Errorf("strip %+v after one mark, want 1 of 3", s)
	}

	if !strings.Contains(tuitest.StripANSI(m.render()), "✓ ") {
		t.Error("the read hunk's label is not drawn read")
	}

	m = press(t, m, "space")
	if !store.Unread(change, string(refs[0].ID)) || !strings.HasPrefix(m.message, "hunk unread") {
		t.Errorf("a second space left the hunk read, said %q", m.message)
	}

	// The file's heading takes both of a.txt's hunks at once, and b.txt's stays unread.
	m = press(t, m, "[", "f", "space")

	if store.Unread(change, string(refs[0].ID)) || store.Unread(change, string(refs[1].ID)) {
		t.Error("space on a.txt's heading did not mark both its hunks")
	}

	if !store.Unread(change, string(refs[2].ID)) || !strings.HasPrefix(m.message, "a.txt read") {
		t.Errorf("space on a.txt reached b.txt too, or said %q", m.message)
	}

	press(t, m, "space")

	if !store.Unread(change, string(refs[0].ID)) {
		t.Error("space on a file already read did not mark it unread")
	}
}

// A mark follows the hunk's text: rebasing the change elsewhere keeps the hunk read, and changing a
// character of it brings it back unread.
func TestReads_AMarkSurvivesARebaseWhenTheHunkIsUnchanged(t *testing.T) {
	t.Parallel()

	dir := readsRepo(t)
	store := seen.Open(t.TempDir())
	change := changeFor(t, dir, "top")

	m := press(t, readsModel(t, dir, "top", store), "p", "tab", "]", "h", "space")
	if store.Unread(change, string(refsOf(t, dir, change)[0].ID)) {
		t.Fatal("top's hunk was not marked read")
	}

	// top moves off middle onto bottom: its one hunk adds the same line to the same file.
	jjRun(t, dir, "rebase", "-r", change, "-o", "description(exact:'bottom\n')")
	m = press(t, m, "ctrl+l")

	if after := refsOf(t, dir, change); store.Unread(change, string(after[0].ID)) {
		t.Error("the rebase took the mark off a hunk whose text did not change")
	}

	if s := m.strips[change]; s.Read != 1 || s.Total != 1 {
		t.Errorf("after the rebase top's strip is %+v, want it read", s)
	}

	jjRun(t, dir, "edit", change)

	if err := os.WriteFile(filepath.Join(dir, "top.txt"), []byte("tops\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	jjRun(t, dir, "status")

	if after := refsOf(t, dir, change); !store.Unread(change, string(after[0].ID)) {
		t.Error("a hunk whose text changed kept its mark")
	}
}

// Marks are the repository's, not the workspace's: a second workspace opens the same store and a
// model over it sees what the first one marked.
func TestReads_ASecondWorkspaceSeesTheSameMarks(t *testing.T) {
	t.Parallel()

	dir := readsRepo(t)
	second := filepath.Join(t.TempDir(), "second")
	jjRun(t, dir, "workspace", "add", second)

	state := t.TempDir()
	open := func(root string) *seen.Store {
		t.Helper()

		repo, err := jj.RepoStore(root)
		if err != nil {
			t.Fatalf("RepoStore(%s): %v", root, err)
		}

		return seen.Open(filepath.Join(state, seen.RepoKey(repo)))
	}

	first, other := open(dir), open(second)
	if first.Dir() != other.Dir() {
		t.Fatalf("the workspaces keep marks apart: %s and %s", first.Dir(), other.Dir())
	}

	change := changeFor(t, dir, "two")
	press(t, readsModel(t, dir, "two", first), "I", "space")

	m := press(t, readsModel(t, second, "two", other), "I")
	if s := m.strips[change]; s.Read != 3 || s.Total != 3 {
		t.Errorf("the second workspace counts two as %+v, want all 3 read", s)
	}
}

// U draws only what is unread. A hunk marked read under it stays drawn until the cursor leaves it,
// ]u steps the unread ones and says when it runs out, and with nothing left the pane says the change
// is read.
func TestReads_OnlyUnreadHoldsAMarkUntilTheCursorLeaves(t *testing.T) {
	t.Parallel()

	for _, pane := range readPanes {
		t.Run(pane.name, func(t *testing.T) {
			t.Parallel()

			dir := readsRepo(t)
			store := seen.Open(t.TempDir())
			m := press(t, readsModel(t, dir, "two", store), append(pane.open, "U")...)

			assertOnlyUnread(t, m, pane.view, changeFor(t, dir, "two"))
		})
	}
}

func assertOnlyUnread(t *testing.T, m Model, view func(*Model) readView, change string) {
	t.Helper()

	shown := func() int { return len(view(&m).state.shown) }

	if shown() != 3 || !strings.HasPrefix(m.message, "only unread: 3 of 3 hunks") {
		t.Fatalf("U shows %d hunks, said %q", shown(), m.message)
	}

	m = press(t, m, "]", "u", "space")
	if shown() != 3 {
		t.Errorf("the hunk just marked was taken away under the cursor: %d shown", shown())
	}

	m = press(t, m, "]", "u")
	if shown() != 2 {
		t.Errorf("after ]u left the read hunk %d are shown, want 2", shown())
	}

	if path, k, ok := view(&m).pane.HunkAt(); !ok || path != "a.txt" || k != 0 {
		t.Errorf("]u landed on %s hunk %d, want a.txt's second hunk, now its first", path, k)
	}

	m = press(t, m, "]", "u", "]", "u")
	if !strings.Contains(m.message, "no unread hunk below") {
		t.Errorf("]u past the last unread said %q", m.message)
	}

	m = press(t, m, "space", "[", "u", "space", "k")
	if m.unreadIn(change) != 0 || shown() != 0 {
		t.Fatalf("%d left unread, %d shown", m.unreadIn(change), shown())
	}

	if frame := tuitest.StripANSI(m.render()); !strings.Contains(frame, change+" is read") {
		t.Errorf("an all-read change under U draws no card:\n%s", frame)
	}

	m = press(t, m, "]", "u")
	if !strings.Contains(m.message, "all 3 hunks read") {
		t.Errorf("]u with nothing unread said %q", m.message)
	}
}

// A mark made in the full-screen diff is the dock's too: esc back to the log and the dock draws the
// hunk read and the log row counts it.
func TestReads_TheFullScreenDiffAndTheDockShareMarks(t *testing.T) {
	t.Parallel()

	dir := readsRepo(t)
	store := seen.Open(t.TempDir())
	change := changeFor(t, dir, "two")

	m := press(t, readsModel(t, dir, "two", store), "enter", "]", "h", "space", "esc")
	if m.diff.IsOpen() || m.marks.full.change != "" {
		t.Fatal("esc left the full-screen diff, or its change, behind")
	}

	if s := m.strips[change]; s.Read != 1 {
		t.Errorf("the log counts %+v after a mark in the full-screen diff", s)
	}

	m = press(t, m, "p")
	if !strings.Contains(tuitest.StripANSI(m.render()), "✓ ") {
		t.Error("the dock does not draw the hunk marked in the full-screen diff")
	}
}

// In the outline space marks the chapter whole, reading its diff first when the dock has not, and ]u
// steps the chapters with something left; the log's rows carry the count and ]u steps them too.
func TestReads_OutlineAndLogStepUnreadRevisions(t *testing.T) {
	t.Parallel()

	dir := readsRepo(t)
	store := seen.Open(t.TempDir())

	m := press(t, readsModel(t, dir, "two", store), "I")

	for _, c := range m.outline.view.Chapters() {
		if !c.Rev.Empty && !c.Read.Known() {
			t.Errorf("opening the outline left %q uncounted", c.Rev.Description)
		}
	}

	m = press(t, m, "space")
	two := changeFor(t, dir, "two")

	if m.unreadIn(two) != 0 || !strings.HasPrefix(m.message, two+" read") {
		t.Fatalf("space on the chapter left %d unread, said %q", m.unreadIn(two), m.message)
	}

	m = press(t, m, "[", "u")
	if got := m.outline.view.Selected().Description; got != "base" {
		t.Errorf("[u from two moved to %q, want base, its parent", got)
	}

	m = press(t, m, "esc")
	if !strings.Contains(tuitest.StripANSI(m.render()), "◌1") {
		t.Error("the log rows do not carry their unread counts")
	}

	m = press(t, m, "g", "g", "]", "u")
	if got := m.log.Selected().Description; got != "base" {
		t.Errorf("]u in the log moved to %q, want base, stepping over two, which is read", got)
	}
}

// Every read key says why it does nothing where it cannot: without a store, on a round, and with
// nothing counted.
func TestReads_GatesSayWhy(t *testing.T) {
	t.Parallel()

	dir := readsRepo(t)

	m := press(t, liveModel(t, dir, "two"), "p", "tab", "space")
	if !strings.Contains(m.message, "read marks are off") {
		t.Errorf("space without a store said %q", m.message)
	}

	m = press(t, readsModel(t, dir, "two", seen.Open(t.TempDir())), "]", "u")
	if !strings.Contains(m.message, "no diff here is counted yet") {
		t.Errorf("]u in a log with nothing counted said %q", m.message)
	}

	// A full-screen diff that is not one change's whole diff (a file's, two revisions') keeps none.
	m = asModel(t, update(m, diffLoadedMsg{title: "diff a.txt in two", text: dockFirstDiff}))
	for _, keys := range [][]string{{"space"}, {"]", "u"}, {"U", "U"}} {
		m = press(t, m, keys...)

		want := "read marks are kept on one change's whole diff, and this is diff a.txt in two"
		if keys[0] == "U" {
			want = "every hunk" // U is a mode, not a mark: it toggles anywhere
		}

		if !strings.Contains(m.message, want) {
			t.Errorf("%v in a file's diff said %q", keys, m.message)
		}
	}
}

func TestFilterDiff_KeepsOnlyTheHunksAsked(t *testing.T) {
	t.Parallel()

	text := dockFirstDiff
	refs := seen.Parse([]byte(text))

	got, kept, ok := filterDiff(text, refs, func(r seen.Ref) bool { return r.Path == "second.txt" })
	if !ok || len(kept) != 1 || kept[0].Path != "second.txt" {
		t.Fatalf("kept %+v (ok %v)", kept, ok)
	}

	if strings.Contains(got, "first.txt") || !strings.Contains(got, "+new second") {
		t.Errorf("filtered diff:\n%s", got)
	}

	if again := seen.Parse([]byte(got)); len(again) != 1 || again[0].ID != refs[1].ID {
		t.Errorf("the filtered diff names its hunk %+v, want %s", again, refs[1].ID)
	}

	if _, _, ok := filterDiff(text, refs[:1], func(seen.Ref) bool { return true }); ok {
		t.Error("a hunk list shorter than the diff was taken as matching")
	}
}

// The outline and dock with marks at both sizes: a chapter partly read with its unread count, a
// hunk drawn read, and the read keys in the footer.
func TestReads_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			store := seen.Open(t.TempDir())
			refs := seen.Parse([]byte(dockFirstDiff))

			if err := store.Mark("yqosmzxt", true, refs, refs[0].ID); err != nil {
				t.Fatal(err)
			}

			opts := testOptions(t)
			opts.Reads = store

			m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", opts)
			m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: twoRows()})
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))

			msg := fakeStack()
			msg.seq = m.outline.seq
			m, _ = m.landOutline(msg)
			m = asModel(t, update(m, dockLoadedMsg{
				revision: "yqosmzxt", text: dockFirstDiff, seq: m.dockSeq, hunks: refs,
			}))

			frame := m.render()
			assertFits(t, frame, size[0], size[1], 1)

			plain := tuitest.StripANSI(frame)
			if !strings.Contains(plain, "◌1") || !strings.Contains(plain, "✓ ") {
				t.Errorf("the frame does not draw the unread count and the read hunk:\n%s", plain)
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

// The full-screen diff with marks at both sizes: a hunk drawn read and the read keys in its footer.
func TestReads_FullScreenFrames(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			store := seen.Open(t.TempDir())
			refs := seen.Parse([]byte(dockFirstDiff))

			if err := store.Mark("yqosmzxt", true, refs, refs[0].ID); err != nil {
				t.Fatal(err)
			}

			opts := testOptions(t)
			opts.Reads = store

			m := New(t.Context(), jj.NewClient(t.TempDir()), ui.New(theme.Macchiato()), t.TempDir(), "@", opts)
			m, _ = m.applyLog(logLoadedMsg{revset: "@", rows: twoRows()})
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))
			m = asModel(t, update(m, diffLoadedMsg{
				title: "diff yqosmzxt", text: dockFirstDiff, change: "yqosmzxt", hunks: refs,
			}))

			frame := m.render()
			assertFits(t, frame, size[0], size[1], 0)

			plain := tuitest.StripANSI(frame)
			for _, want := range []string{"✓ ", "[space] read"} {
				if !strings.Contains(plain, want) {
					t.Errorf("the full-screen diff lacks %q:\n%s", want, plain)
				}
			}

			if size[0] >= 100 && !strings.Contains(plain, "only [U]nread") {
				t.Errorf("the full-screen footer does not tell U from ]u:\n%s", plain)
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}
