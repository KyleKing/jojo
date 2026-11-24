package model

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

// historyRepo is notes.txt added, edited, renamed to guide.txt, and edited again, with an unrelated
// change between each, and a side branch off the first change that edits the file too.
func historyRepo(t *testing.T) string {
	t.Helper()

	dir := liveRepo(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	lines := "one\ntwo\nthree\nfour\nfive\nsix\n"
	jjRun(t, dir, "new", "description(exact:'top\n')")
	write("notes.txt", lines)
	jjRun(t, dir, "commit", "-m", "add notes")
	write("unrelated.txt", "x\n")
	jjRun(t, dir, "commit", "-m", "unrelated")
	write("notes.txt", lines+"seven\n")
	jjRun(t, dir, "commit", "-m", "edit notes")

	if err := os.Rename(filepath.Join(dir, "notes.txt"), filepath.Join(dir, "guide.txt")); err != nil {
		t.Fatal(err)
	}

	jjRun(t, dir, "commit", "-m", "rename to guide")
	write("guide.txt", lines+"seven\neight\n")
	jjRun(t, dir, "commit", "-m", "edit guide")

	return dir
}

func described(m Model) []string {
	revs := revisionsOf(m.rows)
	out := make([]string, 0, len(revs))

	for _, rev := range revs {
		out = append(out, rev.Description)
	}

	return out
}

// * on a file in the file list narrows the log to the changes that touched it, under its old name as
// well; the dock opens on that file alone and follows the cursor; esc puts back the revset, the row,
// and the dock as they were.
func TestHistory_FromTheFileListFollowsARenameAndEscReturns(t *testing.T) {
	t.Parallel()

	dir := historyRepo(t)
	m := liveModel(t, dir, "edit guide")

	m = press(t, m, "l")
	if !m.files.IsOpen() {
		t.Fatalf("l opened no file list: %q", m.message)
	}

	m = press(t, m, "*")

	want := []string{"edit guide", "rename to guide", "edit notes", "add notes"}
	if !m.history.active || !slices.Equal(described(m), want) {
		t.Fatalf("the history is %v (active %v, %q), want %v", described(m), m.history.active, m.message, want)
	}

	if m.files.IsOpen() || !m.docked || m.log.Selected().Description != "edit guide" {
		t.Errorf("files open %v, docked %v, cursor on %q", m.files.IsOpen(), m.docked, m.log.Selected().Description)
	}

	frame := tuitest.StripANSI(m.render())
	if !strings.Contains(frame, "history of guide.txt (was notes.txt) · 4 changes") {
		t.Errorf("no banner naming the file and its old name:\n%s", frame)
	}

	m = press(t, m, "j")
	m = press(t, m, "j")

	if title := m.dock.Title(); !strings.Contains(title, "· guide.txt") {
		t.Errorf("the dock reads %q, want one file's diff", title)
	}

	if text := m.dock.CurrentPath(); text != "notes.txt" && text != "guide.txt" {
		t.Errorf("the dock on edit notes is in %q", text)
	}

	m = press(t, m, "esc")

	if m.history.active || m.docked || m.revset != "all()" || m.log.Selected().Description != "edit guide" {
		t.Errorf("esc left history %v, docked %v, revset %q, cursor on %q",
			m.history.active, m.docked, m.revset, m.log.Selected().Description)
	}
}

// * reads the file under the cursor in the dock, and in the full-screen diff, and a dock that was
// open before stays open after esc.
func TestHistory_FromTheDockAndTheFullScreenDiff(t *testing.T) {
	t.Parallel()

	dir := historyRepo(t)

	for _, tc := range []struct {
		name string
		keys []string
	}{
		{"dock", []string{"p", "tab"}},
		{"log with the dock open", []string{"p"}},
		{"full-screen diff", []string{"enter"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := liveModel(t, dir, "rename to guide")
			for _, k := range tc.keys {
				m = press(t, m, k)
			}

			m = press(t, m, "*")

			want := []string{"edit guide", "rename to guide", "edit notes", "add notes"}
			if !slices.Equal(described(m), want) {
				t.Fatalf("the history is %v (%q), want %v", described(m), m.message, want)
			}

			if m.diff.IsOpen() || m.log.Selected().Description != "rename to guide" {
				t.Errorf("diff open %v, cursor on %q", m.diff.IsOpen(), m.log.Selected().Description)
			}

			m = press(t, m, "esc")
			if m.history.active || m.docked != (tc.name != "full-screen diff") {
				t.Errorf("after esc history %v, docked %v", m.history.active, m.docked)
			}
		})
	}
}

// Without a file under the cursor * says where to find one.
func TestHistory_RefusesWithoutAFile(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "rnzxmskv")
	if _, msgs := pressOne(t, m, "*"); !strings.Contains(said(msgs), "needs a file") {
		t.Errorf("* in the log said %q", said(msgs))
	}
}

// A file's history over the log at both sizes, the dock beside or under it showing that file alone.
func TestHistory_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, size, "rnzxmskv")
			m.history = historyState{
				active: true, back: historyReturn{revset: "@"},
				file: jj.FileHistory{Names: []string{"main.go", "cmd.go"}, Revset: "files(...)"},
			}
			m.docked = true
			m.resize()
			m.dockSeq++
			loaded := dockLoadedMsg{revision: "rnzxmskv", text: dockGoDiff, seq: m.dockSeq, path: "main.go"}
			m = asModel(t, update(m, loaded))
			m = asModel(t, update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}))

			assertVerbFrame(t, m.render(), size)
		})
	}
}
