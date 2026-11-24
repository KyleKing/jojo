package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/rounds"
	"github.com/kyleking/jojo/internal/tuitest"
)

// TestOutline_ReadsTheStackParentFirstAndTheDockFollows covers the review loop: I opens the stack at
// the cursor as chapters, parent first, docks the diff on the working copy's chapter, and ]] and
// [[ walk the chapters with the diff following.
func TestOutline_ReadsTheStackParentFirstAndTheDockFollows(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	m = press(t, m, "I")

	if !m.outline.active || !m.docked {
		t.Fatalf("I left the outline %v and the dock %v, want both open", m.outline.active, m.docked)
	}

	var got []string
	for _, c := range m.outline.view.Chapters() {
		got = append(got, c.Rev.Description)
	}

	// The working copy is an empty change on top, so it is the last chapter; side is not in the stack.
	if want := []string{"bottom", "middle", "top", ""}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("chapters %q, want %q: the stack under top, parent first, and nothing from side", got, want)
	}

	if got := m.outline.view.Selected().Description; got != "top" {
		t.Errorf("cursor on %q, want the chapter the log cursor was on", got)
	}

	if !strings.Contains(m.dock.Title(), changeFor(t, dir, "top")) {
		t.Errorf("dock shows %q, want top's diff", m.dock.Title())
	}

	m = press(t, m, "[", "[")

	if got := m.outline.view.Selected().Description; got != "middle" {
		t.Fatalf("[[ moved to %q, want middle", got)
	}

	if !strings.Contains(m.dock.Title(), changeFor(t, dir, "middle")) {
		t.Errorf("after [[ the dock shows %q, want middle's diff", m.dock.Title())
	}

	if c := m.outline.view.Chapters()[1]; c.Rev.Files != 1 || c.Rev.Added != 1 {
		t.Errorf("middle's size is %d files +%d, want what it adds", c.Rev.Files, c.Rev.Added)
	}

	if !m.outline.view.Chapters()[1].Read.Known() {
		t.Error("the chapter the dock read has no read strip")
	}

	frame := tuitest.StripANSI(m.render())
	if !strings.Contains(frame, "Stack · ") || !strings.Contains(frame, "]] chapter") {
		t.Errorf("the frame does not draw the outline and its keys:\n%s", frame)
	}
}

// Esc gives the log back on the chapter the outline was on, and takes away the dock it opened.
func TestOutline_EscReturnsToTheLogOnTheChapter(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "top"), "I", "k", "k", "esc")

	if m.outline.active || m.docked {
		t.Fatalf("after Esc the outline is %v and the dock %v, want both closed", m.outline.active, m.docked)
	}

	if got := m.log.Selected().Description; got != "bottom" {
		t.Errorf("the log cursor is on %q, want bottom, where the outline was", got)
	}
}

// The outline reads; a write verb pressed out of habit does nothing and says where the verbs are.
func TestOutline_WriteVerbsDoNothing(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "top"), "I")
	before := opCount(t, dir)
	m = press(t, m, "a")

	if opCount(t, dir) != before || !strings.Contains(m.message, "Esc returns to the log") {
		t.Errorf("a in the outline wrote %d operations, said %q", opCount(t, dir)-before, m.message)
	}
}

// amendedStack is liveRepo with its middle change amended (a snapshot round, which also rebases
// top) and top rebased away and back (two more versions that change nothing in top's patch).
func amendedStack(t *testing.T) string {
	t.Helper()

	dir := liveRepo(t)
	jjRun(t, dir, "edit", "description(exact:'middle\n')")

	if err := os.WriteFile(filepath.Join(dir, "middle.txt"), []byte("middle, amended\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	jjRun(t, dir, "new", "description(exact:'top\n')")
	jjRun(t, dir, "rebase", "-r", "description(exact:'top\n')", "-o", "description(exact:'bottom\n')")
	jjRun(t, dir, "rebase", "-r", "description(exact:'top\n')", "-o", "description(exact:'middle\n')")

	return dir
}

// [r steps the dock back through the change's versions as interdiffs, labeled by the operation
// that made each, ]r steps forward and past the newest returns to the change as it stands, and the
// rebases that left top's patch alone fold into one round.
func TestRounds_StepThroughVersionsWithInterdiffs(t *testing.T) {
	t.Parallel()

	dir := amendedStack(t)
	m := liveModel(t, dir, "top")
	m = press(t, m, "[", "r")

	if !m.docked {
		t.Fatal("[r did not dock the diff")
	}

	if title := m.dock.Title(); !strings.Contains(title, rounds.Moved) || !strings.HasPrefix(title, "versions ") {
		t.Fatalf("newest round of top is %q, want the folded rebases", title)
	}

	if body := tuitest.StripANSI(strings.Join(m.dock.Lines(), "\n")); !strings.Contains(body, "("+rounds.Moved+")") {
		t.Errorf("a folded round draws %q, want the sentence saying nothing changed", body)
	}

	// Older from there: the commit that described top, the snapshot that wrote top.txt, and the empty
	// change jj started it as, which is where [r stops.
	titles := make([]string, 0, 4)

	for range 4 {
		m = press(t, m, "[", "r")
		titles = append(titles, m.dock.Title())
	}

	wants := []string{"commit, description only", "snapshot", "the first version", "the first version"}
	for i, want := range wants {
		if !strings.Contains(titles[i], want) {
			t.Errorf("round %d back is %q, want %q", i+1, titles[i], want)
		}
	}

	if !strings.Contains(m.message, "nothing before it") {
		t.Errorf("[r at the first version said %q", m.message)
	}

	m = press(t, m, "]", "r", "]", "r", "]", "r", "]", "r")

	m = press(t, m, "]", "r", "]", "r")

	if title := m.dock.Title(); !strings.HasPrefix(title, "diff ") {
		t.Errorf("]r past the newest shows %q, want the change as it stands", title)
	}

	key := versionsKey(m.log.Selected())
	if _, ok := m.versions[key]; !ok {
		t.Error("the evolution was not cached under the commit it was read at")
	}
}

// The amended middle change has a content round whose interdiff is the amendment alone.
func TestRounds_AnAmendmentShowsItsInterdiff(t *testing.T) {
	t.Parallel()

	dir := amendedStack(t)
	m := press(t, liveModel(t, dir, "middle"), "[", "r")

	body := tuitest.StripANSI(strings.Join(m.dock.Lines(), "\n"))
	if !strings.Contains(m.dock.Title(), "snapshot") || !strings.Contains(body, "middle, amended") {
		t.Errorf("middle's newest round is %q with\n%s\nwant the snapshot that amended it", m.dock.Title(), body)
	}
}

// H lists the rounds; Enter on one shows it in the dock.
func TestRounds_HListsThemAndEnterShowsOne(t *testing.T) {
	t.Parallel()

	dir := amendedStack(t)
	m := press(t, liveModel(t, dir, "top"), "H")

	if !m.roundsBox.IsOpen() {
		t.Fatal("H did not open the rounds box")
	}

	m = press(t, m, "G", "enter")

	if m.roundsBox.IsOpen() || !strings.HasPrefix(m.dock.Title(), "version 1 of") {
		t.Errorf("Enter on the oldest round left the box open %v and the dock on %q",
			m.roundsBox.IsOpen(), m.dock.Title())
	}
}

// At a narrow width the row cuts the description and drops the author; the inspector says both, and
// the commit, on the line over the footer.
func TestInspector_SaysWhatTheRowHadNoRoomFor(t *testing.T) {
	t.Parallel()

	rows := []jj.LogRow{{Revision: &jj.Revision{
		ChangeID: "kxqpmnzo", CommitID: "0123456789ab", AuthorEmail: "someone@example.com", Age: "3 hours ago",
		Description: "feat(parse): read the header format",
	}}}

	for _, width := range []int{60, 200} {
		m := asModel(t, update(testModel(t, rows), tea.WindowSizeMsg{Width: width, Height: 20}))
		lines := strings.Split(tuitest.StripANSI(m.render()), "\n")
		inspector := lines[len(lines)-2]

		cut := strings.Contains(inspector, "feat(parse): read the header format")
		if cut != (width == 60) {
			t.Errorf("at %d the inspector reads %q", width, inspector)
		}

		if !strings.Contains(inspector, "commit 0123456789ab") {
			t.Errorf("at %d the inspector does not name the commit: %q", width, inspector)
		}

		if len(lines) != 20 {
			t.Errorf("at %d the frame is %d lines", width, len(lines))
		}
	}
}

// fakeStack is the outline a test lands without jj: four chapters with known ids.
func fakeStack() outlineLoadedMsg {
	return outlineLoadedMsg{
		revset: "(trunk()..@) | (@:: ~ ::trunk())",
		revs: []jj.Revision{
			{
				ChangeID: "kxqpmnzo", CommitID: "a1", Description: "feat(parse): read the new header format",
				Bookmarks: []string{"parse-header"}, LocalBookmarks: []string{"parse-header"},
				AuthorEmail: "a@example.com", Age: "3 hours ago",
			},
			{ChangeID: "vwtrlsqy", CommitID: "b2", Description: "refactor: split the reader", Conflict: true},
			{ChangeID: "nnzolpqr", CommitID: "c3", Empty: true},
			{ChangeID: "yqosmzxt", CommitID: "d4", Description: "test: cover the fallbacks", WorkingCopy: true},
		},
		stats: map[string]jj.Stats{"a1": {Files: 3, Added: 120, Removed: 14}, "b2": {Files: 2, Added: 40, Removed: 38}},
		forge: map[string]string{"kxqpmnzo": "#41 open · CI passing"},
	}
}

// outlineFrame lands the fake stack and a diff for its working-copy chapter at width by height.
func outlineFrame(t *testing.T, width, height int) Model {
	t.Helper()

	m := asModel(t, update(testModel(t, twoRows()), tea.WindowSizeMsg{Width: width, Height: height}))
	msg := fakeStack()
	msg.seq = m.outline.seq
	m, _ = m.landOutline(msg)

	return asModel(t, update(m, dockLoadedMsg{
		revision: "yqosmzxt", text: dockFirstDiff, seq: m.dockSeq,
		hunks: nil,
	}))
}

func TestOutline_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			frame := outlineFrame(t, size[0], size[1]).render()
			assertFits(t, frame, size[0], size[1], 1)
			tuitest.AssertGolden(t, frame)
		})
	}
}

// A round in the dock at both sizes: the banner names the version, the operation, and the age.
func TestRounds_Frames(t *testing.T) {
	t.Parallel()

	versions := []jj.Version{
		{ChangeID: "yqosmzxt", CommitID: "v1", Operation: "new empty commit", Age: "3 hours ago", Files: 1},
		{
			ChangeID: "yqosmzxt", CommitID: "v2", Operation: "snapshot working copy", Age: "2 hours ago", Files: 2,
			Predecessors: []jj.Predecessor{{ChangeID: "yqosmzxt", CommitID: "v1"}},
		},
		{
			ChangeID: "yqosmzxt", CommitID: "v3", Operation: "rebase commit 1234", Age: "1 hour ago",
			Predecessors: []jj.Predecessor{{ChangeID: "yqosmzxt", CommitID: "v2"}},
		},
	}

	for _, size := range [][2]int{{80, 24}, {120, 35}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := outlineFrame(t, size[0], size[1])
			m.versions["d4"] = versions
			m.interdiffs["v2..v3"] = ""

			m = keysNoPump(t, m, "[", "r")

			if !strings.Contains(m.dock.Title(), "version 3 of 3 · "+rounds.Moved) {
				t.Fatalf("[r showed %q, want the rebase folded", m.dock.Title())
			}

			// The cached rebase drew at once; the snapshot before it needs a read, which lands here.
			m = keysNoPump(t, m, "[", "r")

			m = asModel(t, update(m, dockLoadedMsg{
				revision: "v2", title: m.rounds.list[1].Banner(3), text: dockSecondDiff, seq: m.dockSeq,
				cacheKey: "v1..v2",
			}))

			frame := m.render()
			assertFits(t, frame, size[0], size[1], 1)

			if !strings.Contains(tuitest.StripANSI(frame), "version 2 of 3 · snapshot · 2 hours ago") {
				t.Errorf("the banner is missing:\n%s", tuitest.StripANSI(frame))
			}

			tuitest.AssertGolden(t, frame)
		})
	}
}

// keysNoPump presses keys without running what they start, so a test can land the reads itself.
func keysNoPump(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, key := range keys {
		m = asModel(t, update(m, tuitest.Key(key)))
	}

	return m
}

// Beside the dock the outline takes its own width rather than a fixed share, up to 45% of the
// terminal, stretching to half before it drops a column, and the dock gets the rest.
func TestOutline_TakesItsContentWidthAndTheDockTheRest(t *testing.T) {
	t.Parallel()

	for _, width := range []int{100, 120, 160, 240} {
		m := outlineFrame(t, width, 30)
		dock, _ := m.dockSize()
		outline := width - dock
		content := m.outline.view.ContentWidth()

		if outline > content {
			t.Errorf("at %d the outline takes %d cells for rows %d wide", width, outline, content)
		}

		if outline > width/2 {
			t.Errorf("at %d the outline takes %d cells, past half the terminal", width, outline)
		}

		if outline < content && outline < width*outlineMaxShare/percent {
			t.Errorf("at %d the outline cuts its rows at %d cells with room up to %d",
				width, outline, width*outlineMaxShare/percent)
		}

		for i, line := range strings.Split(ansi.Strip(m.render()), "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("at %d row %d is %d cells", width, i, w)
			}
		}
	}
}
