package bookmarkview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/components/bookmarkview"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

const testWidth, testHeight = 100, 12

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

func local(name, changeID, description string, conflict bool) jj.Bookmark {
	return jj.Bookmark{Name: name, ChangeID: changeID, Description: description, Conflict: conflict}
}

func remote(name, changeID string, ahead int, tracked bool) jj.Bookmark {
	return jj.Bookmark{
		Name:     name,
		Remote:   "origin",
		ChangeID: changeID,
		Ahead:    ahead,
		Tracked:  tracked,
	}
}

func TestActionsOnCursorRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		want tea.Msg
		name string
		key  string
	}{
		{name: "s emits SetMsg", key: "s", want: bookmarkview.SetMsg{Name: "main", Target: "zzz"}},
		{name: "a emits AdvanceMsg", key: "a", want: bookmarkview.AdvanceMsg{Name: "main", Target: "zzz"}},
		{name: "c emits CreateRequestedMsg", key: "c", want: bookmarkview.CreateRequestedMsg{Target: "zzz"}},
		{name: "r emits RenameRequestedMsg", key: "r", want: bookmarkview.RenameRequestedMsg{Name: "main"}},
		{name: "d emits DeleteMsg", key: "d", want: bookmarkview.DeleteMsg{Name: "main"}},
		{name: "f emits ForgetMsg", key: "f", want: bookmarkview.ForgetMsg{Name: "main"}},
		{name: "p emits PushMsg", key: "p", want: bookmarkview.PushMsg{Name: "main"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open([]jj.Bookmark{local("main", "aaa", "one", false)}, "zzz")

			cmd := m.Update(tuitest.Key(tc.key))
			if cmd == nil {
				t.Fatalf("Update(%s) did not return a command", tc.key)
			}

			if got := cmd(); got != tc.want {
				t.Errorf("Update(%s) = %#v, want %#v", tc.key, got, tc.want)
			}

			if m.IsOpen() {
				t.Errorf("Update(%s) did not close the panel", tc.key)
			}
		})
	}
}

func TestCancelKeysEmitCancelled(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"esc", "q", "b"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open([]jj.Bookmark{local("main", "aaa", "one", false)}, "zzz")

			if _, ok := drainMsg[bookmarkview.CancelledMsg](m.Update(tuitest.Key(key))); !ok {
				t.Fatalf("Update(%s) did not emit CancelledMsg", key)
			}

			if m.IsOpen() {
				t.Errorf("Update(%s) did not close the panel", key)
			}
		})
	}
}

func TestTrackTogglesFromRowState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want string
		mark jj.Bookmark
	}{
		{name: "untracked remote row tracks", mark: remote("main", "aaa", 0, false), want: "track"},
		{name: "tracked remote row untracks", mark: remote("main", "aaa", 0, true), want: "untrack"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open([]jj.Bookmark{tc.mark}, "zzz")

			cmd := m.Update(tuitest.Key("t"))

			switch tc.want {
			case "track":
				msg, ok := drainMsg[bookmarkview.TrackMsg](cmd)
				if !ok {
					t.Fatal("Update(t) did not emit TrackMsg")
				}

				if msg.Ref != "main@origin" {
					t.Errorf("TrackMsg.Ref = %q, want %q", msg.Ref, "main@origin")
				}
			case "untrack":
				msg, ok := drainMsg[bookmarkview.UntrackMsg](cmd)
				if !ok {
					t.Fatal("Update(t) did not emit UntrackMsg")
				}

				if msg.Ref != "main@origin" {
					t.Errorf("UntrackMsg.Ref = %q, want %q", msg.Ref, "main@origin")
				}
			}
		})
	}
}

func TestGuardsRefuseAndWarn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		want string
		mark jj.Bookmark
	}{
		{
			name: "s on a remote row",
			key:  "s",
			mark: remote("main", "aaa", 0, true),
			want: "that is a remote bookmark, act on the local one",
		},
		{
			name: "d on a remote row",
			key:  "d",
			mark: remote("main", "aaa", 0, true),
			want: "that is a remote bookmark, act on the local one",
		},
		{
			name: "r on a remote row",
			key:  "r",
			mark: remote("main", "aaa", 0, true),
			want: "that is a remote bookmark, act on the local one",
		},
		{
			name: "t on a local row",
			key:  "t",
			mark: local("main", "aaa", "one", false),
			want: "tracking only applies to a remote bookmark",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open([]jj.Bookmark{tc.mark}, "zzz")
			m.Update(tuitest.Key(tc.key))

			if !m.IsOpen() {
				t.Fatalf("Update(%s) closed the panel on a guarded row", tc.key)
			}

			if !strings.Contains(m.View(testWidth, testHeight), tc.want) {
				t.Errorf("View() does not contain warning %q", tc.want)
			}
		})
	}
}

func TestSetAndAdvanceWarnWithoutTarget(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"s", "a"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open([]jj.Bookmark{local("main", "aaa", "one", false)}, "")
			m.Update(tuitest.Key(key))

			if !m.IsOpen() {
				t.Fatalf("Update(%s) closed the panel with no target", key)
			}

			if !strings.Contains(m.View(testWidth, testHeight), "no revision selected") {
				t.Error("View() does not explain why the action was refused")
			}
		})
	}
}

func TestWarningClearsOnMove(t *testing.T) {
	t.Parallel()

	m := bookmarkview.New(testSkin())
	m.Open([]jj.Bookmark{
		local("main", "aaa", "one", false),
		remote("main", "aaa", 0, false),
	}, "zzz")
	m.Update(tuitest.Key("down"))
	m.Update(tuitest.Key("s"))

	if !strings.Contains(m.View(testWidth, testHeight), "act on the local one") {
		t.Fatal("warning was not set after acting on a remote row")
	}

	m.Update(tuitest.Key("up"))

	if strings.Contains(m.View(testWidth, testHeight), "act on the local one") {
		t.Error("warning was not cleared after moving the cursor")
	}
}

func TestCursorClampsAtBothEnds(t *testing.T) {
	t.Parallel()

	marks := []jj.Bookmark{
		local("a", "aaa", "one", false),
		local("b", "bbb", "two", false),
		local("c", "ccc", "three", false),
	}

	t.Run("up at the first entry stays put", func(t *testing.T) {
		t.Parallel()

		m := bookmarkview.New(testSkin())
		m.Open(marks, "zzz")
		m.Update(tuitest.Key("up"))

		msg, ok := drainMsg[bookmarkview.ForgetMsg](m.Update(tuitest.Key("f")))
		if !ok || msg.Name != "a" {
			t.Errorf("cursor after up at top = %+v, ok=%v, want a", msg, ok)
		}
	})

	t.Run("down at the last entry stays put", func(t *testing.T) {
		t.Parallel()

		m := bookmarkview.New(testSkin())
		m.Open(marks, "zzz")
		m.Update(tuitest.Key("G"))
		m.Update(tuitest.Key("down"))

		msg, ok := drainMsg[bookmarkview.ForgetMsg](m.Update(tuitest.Key("f")))
		if !ok || msg.Name != "c" {
			t.Errorf("cursor after down at bottom = %+v, ok=%v, want c", msg, ok)
		}
	})
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := bookmarkview.New(testSkin())
	m.Open([]jj.Bookmark{
		local("main", "aaa111", "trunk work", false),
		remote("feature", "bbb222", 2, true),
		local("hotfix", "ccc333", "urgent fix", true),
	}, "zzz999")
	m.Update(tuitest.Key("down"))

	tuitest.AssertGolden(t, m.View(testWidth, testHeight))
}

func TestResolveTargetsTheRemoteSide(t *testing.T) {
	t.Parallel()

	m := bookmarkview.New(testSkin())
	m.Open([]jj.Bookmark{
		local("main", "aaa", "local side", true),
		remote("main", "bbb", 0, true),
	}, "zzz")

	msg, ok := drainMsg[bookmarkview.ResolveMsg](m.Update(tuitest.Key("x")))
	if !ok {
		t.Fatal("Update(x) did not emit ResolveMsg")
	}

	if msg.Name != "main" || msg.Ref != "main@origin" {
		t.Errorf("ResolveMsg = %#v, want Name main resolving to main@origin", msg)
	}

	if m.IsOpen() {
		t.Error("Update(x) did not close the panel")
	}
}

func TestResolveRefusesRowsWithNothingToResolve(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		marks []jj.Bookmark
	}{
		{name: "clean bookmark", marks: []jj.Bookmark{local("main", "aaa", "one", false)}},
		{name: "conflicted without a remote side", marks: []jj.Bookmark{local("main", "aaa", "one", true)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := bookmarkview.New(testSkin())
			m.Open(tc.marks, "zzz")

			if _, ok := drainMsg[bookmarkview.ResolveMsg](m.Update(tuitest.Key("x"))); ok {
				t.Error("Update(x) emitted ResolveMsg with nothing valid to resolve to")
			}
		})
	}
}
