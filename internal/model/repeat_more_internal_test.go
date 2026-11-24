package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
)

// A description written in the prompt is the write the repeat key runs again: the next revision gets the same
// message without the prompt. A bookmark deleted through the b menu repeats on the next revision's.
func TestRepeatLive_DescribeAndBookmarkDelete(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "top"), "d")

	if !m.prompt.IsOpen() {
		t.Fatalf("d opened no prompt: %q", m.message)
	}

	m = submitPrompt(t, m, "wip: retitle")

	if !m.log.Focus(changeFor(t, dir, "middle")) {
		t.Fatal("no middle row")
	}

	press(t, m, ".")

	if got := strings.Count(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r", "all()",
		"-T", "description.first_line() ++ \"\\n\""), "wip: retitle"); got != 2 {
		t.Errorf(". wrote the description on %d revisions, want 2", got)
	}

	dir = liveRepo(t)
	jjRun(t, dir, "bookmark", "create", "one", "-r", "description(exact:'top\n')")
	jjRun(t, dir, "bookmark", "create", "two", "-r", "description(exact:'middle\n')")

	m = press(t, liveModel(t, dir, "top"), "b", "d")
	if !m.log.Focus(changeFor(t, dir, "middle")) {
		t.Fatal("no middle row")
	}

	press(t, m, ".")

	if left := strings.TrimSpace(jjRun(t, dir, "bookmark", "list", "--ignore-working-copy",
		"-T", "name ++ \"\\n\"")); left != "" {
		t.Errorf("bookmarks left after b d and .: %q", left)
	}
}

// submitPrompt submits text as what the prompt holds.
func submitPrompt(t *testing.T, m Model, text string) Model {
	t.Helper()

	onSubmit := m.onSubmit
	m.prompt.Close()
	m.onSubmit = nil

	next, cmd := onSubmit(m, text)

	return pump(t, next, cmd)
}

// ' labels the files on screen and a label lands there; G and the label leave jumps ctrl+o walks
// back and ctrl+i forward.
func TestFileList_LabelJumpAndJumpList(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "rnzxmskv")
	m.files.Open("rnzxmskv", []jj.FileChange{
		{Status: "M", Path: "a.go"}, {Status: "M", Path: "b.go"}, {Status: "A", Path: "c.go"},
	})
	_ = m.render()

	m, _ = pressOne(t, m, "'")

	if frame := tuitest.StripANSI(m.render()); !m.files.Labeling() || !strings.Contains(frame, "d  ") {
		t.Fatalf("' labeled no files:\n%s", frame)
	}

	m, _ = pressOne(t, m, "d")
	if _, path, _ := m.files.Selected(); path != "c.go" {
		t.Errorf("the label d landed on %s, want c.go", path)
	}

	for _, tc := range []struct{ key, want string }{
		{"ctrl+o", "a.go"}, {"ctrl+i", "c.go"}, {"g", ""}, {"ctrl+o", ""},
	} {
		if tc.key == "g" {
			m, _ = pressOne(t, m, "g")
			m, _ = pressOne(t, m, "g")

			continue
		}

		m, _ = pressOne(t, m, tc.key)
		if _, path, _ := m.files.Selected(); tc.want != "" && path != tc.want {
			t.Errorf("%s landed on %s, want %s", tc.key, path, tc.want)
		}
	}

	if _, path, _ := m.files.Selected(); path != "c.go" {
		t.Errorf("ctrl+o after gg went to %s, want c.go", path)
	}
}

// The file list's labels at both sizes.
func TestFileList_LabelFrames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()

			m := verbFrame(t, size, "rnzxmskv")
			m.files.Open("rnzxmskv", []jj.FileChange{
				{Status: "M", Path: "parse/header.go"},
				{Status: "M", Path: "parse/reader.go"},
				{Status: "A", Path: "parse/reader_test.go"},
			})
			_ = m.render()
			m, _ = pressOne(t, m, "'")

			assertVerbFrame(t, m.render(), size)
		})
	}
}
