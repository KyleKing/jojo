package workspaceview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/workspaceview"
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

func ws(name, changeID, description, root string, current bool) jj.Workspace {
	return jj.Workspace{
		Name:        name,
		ChangeID:    changeID,
		Description: description,
		Root:        root,
		Current:     current,
	}
}

// cursorName reads which workspace the cursor sits on by pressing f: on a
// non-current row that emits ForgetMsg with the name, and on the current row
// (which never emits) it falls back to the View's marked "@" line.
func cursorName(t *testing.T, m *workspaceview.Model) string {
	t.Helper()

	if forget, ok := drainMsg[workspaceview.ForgetMsg](m.Update(tuitest.Key("f"))); ok {
		return forget.Name
	}

	for _, line := range strings.Split(tuitest.StripANSI(m.View(testWidth, testHeight)), "\n") {
		if idx := strings.Index(line, "@ "); idx != -1 {
			return strings.SplitN(strings.TrimSpace(line[idx+len("@ "):]), " ", 2)[0]
		}
	}

	return ""
}

func TestOpenPlacesCursorOnCurrentWorkspace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		want   string
		spaces []jj.Workspace
	}{
		{
			name: "cursor starts on the current workspace",
			spaces: []jj.Workspace{
				ws("default", "aaa", "one", "/repo", false),
				ws("feature", "bbb", "two", "/repo-feature", true),
			},
			want: "feature",
		},
		{
			name: "cursor starts on the first entry when none is current",
			spaces: []jj.Workspace{
				ws("default", "aaa", "one", "/repo", false),
				ws("feature", "bbb", "two", "/repo-feature", false),
			},
			want: "default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := workspaceview.New(testSkin())
			m.Open(tc.spaces)

			if got := cursorName(t, &m); got != tc.want {
				t.Errorf("cursor starts on %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMoveCursorClampsAtBothEnds(t *testing.T) {
	t.Parallel()

	spaces := []jj.Workspace{
		ws("a", "aaa", "one", "/a", false),
		ws("b", "bbb", "two", "/b", false),
		ws("c", "ccc", "three", "/c", false),
	}

	t.Run("up at the first entry stays put", func(t *testing.T) {
		t.Parallel()

		m := workspaceview.New(testSkin())
		m.Open(spaces)
		m.Update(tuitest.Key("up"))

		if got := cursorName(t, &m); got != "a" {
			t.Errorf("cursor after up at top = %q, want %q", got, "a")
		}
	})

	t.Run("down at the last entry stays put", func(t *testing.T) {
		t.Parallel()

		m := workspaceview.New(testSkin())
		m.Open(spaces)
		m.Update(tuitest.Key("G"))
		m.Update(tuitest.Key("down"))

		if got := cursorName(t, &m); got != "c" {
			t.Errorf("cursor after down at bottom = %q, want %q", got, "c")
		}
	})
}

func TestJumpToEnds(t *testing.T) {
	t.Parallel()

	spaces := []jj.Workspace{
		ws("a", "aaa", "one", "/a", false),
		ws("b", "bbb", "two", "/b", false),
		ws("c", "ccc", "three", "/c", false),
	}

	cases := []struct {
		name string
		key  string
		want string
	}{
		{"home jumps to first", "home", "a"},
		{"g jumps to first", "g", "a"},
		{"end jumps to last", "end", "c"},
		{"G jumps to last", "G", "c"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := workspaceview.New(testSkin())
			m.Open(spaces)
			m.Update(tuitest.Key(tc.key))

			forget, ok := drainMsg[workspaceview.ForgetMsg](m.Update(tuitest.Key("f")))
			if !ok {
				t.Fatal("Update(f) did not emit ForgetMsg")
			}

			if forget.Name != tc.want {
				t.Errorf("cursor after %q = %q, want %q", tc.key, forget.Name, tc.want)
			}
		})
	}
}

func TestForgetOnCurrentWarnsWithoutEmitting(t *testing.T) {
	t.Parallel()

	m := workspaceview.New(testSkin())
	m.Open([]jj.Workspace{ws("default", "aaa", "one", "/repo", true)})

	if _, ok := drainMsg[workspaceview.ForgetMsg](m.Update(tuitest.Key("f"))); ok {
		t.Fatal("Update(f) on the current workspace emitted ForgetMsg")
	}

	if !strings.Contains(m.View(testWidth, testHeight), "cannot forget the workspace you are in") {
		t.Error("View() does not explain why forgetting the current workspace was refused")
	}
}

func TestWarningClearsOnMove(t *testing.T) {
	t.Parallel()

	m := workspaceview.New(testSkin())
	m.Open([]jj.Workspace{
		ws("default", "aaa", "one", "/repo", true),
		ws("feature", "bbb", "two", "/repo-feature", false),
	})

	m.Update(tuitest.Key("f"))

	if !strings.Contains(m.View(testWidth, testHeight), "cannot forget") {
		t.Fatal("warning was not set after forgetting the current workspace")
	}

	m.Update(tuitest.Key("down"))

	if strings.Contains(m.View(testWidth, testHeight), "cannot forget") {
		t.Error("warning was not cleared after moving the cursor")
	}
}

func TestAddEmitsAddRequested(t *testing.T) {
	t.Parallel()

	m := workspaceview.New(testSkin())
	m.Open([]jj.Workspace{ws("default", "aaa", "one", "/repo", true)})

	if _, ok := drainMsg[workspaceview.AddRequestedMsg](m.Update(tuitest.Key("a"))); !ok {
		t.Fatal("Update(a) did not emit AddRequestedMsg")
	}
}

func TestEscCancelsAndCloses(t *testing.T) {
	t.Parallel()

	m := workspaceview.New(testSkin())
	m.Open([]jj.Workspace{ws("default", "aaa", "one", "/repo", true)})

	if _, ok := drainMsg[workspaceview.CancelledMsg](m.Update(tuitest.Key("esc"))); !ok {
		t.Fatal("Update(esc) did not emit CancelledMsg")
	}

	if m.IsOpen() {
		t.Error("IsOpen() is true after Update(esc)")
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := workspaceview.New(testSkin())
	m.Open([]jj.Workspace{
		ws("default", "aaa", "trunk work", "/home/kyle/repo", true),
		ws("feature", "bbb", "", "/home/kyle/repo-feature", false),
		ws("hotfix", "ccc", "urgent fix", "/home/kyle/repo-hotfix", false),
	})
	m.Update(tuitest.Key("down"))

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}
