package revsetbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kyleking/jojo/internal/components/revsetbox"
	"github.com/kyleking/jojo/internal/tuitest"
)

const testWidth = 60

var (
	errAliasFetchFailed = errors.New("jj config list failed")
	errBogusFunction    = errors.New("function `bogus` doesn't exist")
)

func typeText(m *revsetbox.Model, s string) {
	for _, r := range s {
		m.Update(tuitest.Key(string(r)))
	}
}

// drainMsgs runs cmd and, if it produced a tea.BatchMsg, recursively runs every command
// in the batch, so a test can find one specific message among unrelated cmds (such as
// the virtual cursor's blink) without knowing bubbletea's exact batching shape.
func drainMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, 0, len(batch))
		for _, sub := range batch {
			msgs = append(msgs, drainMsgs(sub)...)
		}

		return msgs
	}

	return []tea.Msg{msg}
}

func TestCompletionCompletesIdentifierUnderCursorNotWholeLine(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("")
	m.Focus()
	typeText(&m, "ancestors(mi")

	if !strings.Contains(m.View(testWidth), "mine") {
		t.Fatalf("View() = %q, want it to list the mine candidate", m.View(testWidth))
	}

	m.Update(tuitest.Key("tab"))

	const want = "ancestors(mine()"
	if got := m.Value(); got != want {
		t.Errorf("Value() = %q, want %q", got, want)
	}
}

func TestAcceptCandidateLeavesCursorInsideNewCallParens(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("")
	m.Focus()
	typeText(&m, "ancestors(mi")
	m.Update(tuitest.Key("tab"))
	typeText(&m, "x")

	const want = "ancestors(mine(x)"
	if got := m.Value(); got != want {
		t.Errorf("Value() = %q, want %q (typing after accept must land inside the new parens)", got, want)
	}
}

func TestAliasSourceFailureDegradesToBuiltins(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("", revsetbox.WithAliases(func(_ context.Context) ([]string, error) {
		return []string{"myalias"}, errAliasFetchFailed
	}))

	cmd := m.Focus()
	for _, msg := range drainMsgs(cmd) {
		m.Update(msg)
	}

	typeText(&m, "tru")
	if !strings.Contains(m.View(testWidth), "trunk") {
		t.Errorf("View() = %q, want the builtin trunk candidate despite the alias fetch failing", m.View(testWidth))
	}

	m.SetValue("")
	typeText(&m, "myal")

	if strings.Contains(m.View(testWidth), "myalias") {
		t.Error("View() lists an alias from a source that errored; the failure should have been discarded")
	}
}

func TestAliasSourceSuccessAddsCandidates(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("", revsetbox.WithAliases(func(_ context.Context) ([]string, error) {
		return []string{"myalias"}, nil
	}))

	cmd := m.Focus()
	for _, msg := range drainMsgs(cmd) {
		m.Update(msg)
	}

	typeText(&m, "myal")
	if !strings.Contains(m.View(testWidth), "myalias") {
		t.Errorf("View() = %q, want the fetched alias as a candidate", m.View(testWidth))
	}
}

func TestDebounceEmitsOnePerSettle(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("")
	m.Focus()

	cmdA := m.Update(tuitest.Key("a"))
	cmdB := m.Update(tuitest.Key("b"))

	staleTicks := drainMsgs(cmdA)
	settledTicks := drainMsgs(cmdB)

	for _, msg := range staleTicks {
		if cmd := m.Update(msg); cmd != nil {
			if _, ok := drainChangedMsg(cmd); ok {
				t.Error("a stale debounce tick emitted ChangedMsg; only the settled tick should")
			}
		}
	}

	var (
		changed revsetbox.ChangedMsg
		found   bool
	)

	for _, msg := range settledTicks {
		if cmd := m.Update(msg); cmd != nil {
			if got, ok := drainChangedMsg(cmd); ok {
				changed, found = got, true
			}
		}
	}

	if !found {
		t.Fatal("the settled debounce tick did not emit ChangedMsg")
	}

	if changed.Revset != "ab" {
		t.Errorf("ChangedMsg.Revset = %q, want %q", changed.Revset, "ab")
	}
}

func drainChangedMsg(cmd tea.Cmd) (revsetbox.ChangedMsg, bool) {
	for _, msg := range drainMsgs(cmd) {
		if changed, ok := msg.(revsetbox.ChangedMsg); ok {
			return changed, true
		}
	}

	return revsetbox.ChangedMsg{}, false
}

func TestEnterRequiresBalancedDelimiters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		expr       string
		wantChange bool
	}{
		{"unbalanced call does not submit", "ancestors(", false},
		{"balanced call submits", "ancestors()", true},
		{"paren inside a quoted string does not count", `description("(")`, true},
		{"unterminated quote does not submit", `description("wip`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := revsetbox.New("")
			m.Focus()
			typeText(&m, tc.expr)

			_, got := drainChangedMsg(m.Update(tuitest.Key("enter")))
			if got != tc.wantChange {
				t.Errorf("Enter on %q emitted ChangedMsg = %v, want %v", tc.expr, got, tc.wantChange)
			}
		})
	}
}

func TestErrorRendersWithoutClearingValue(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("mine() ~ ::trunk()")
	m.SetError(errBogusFunction)

	view := m.View(testWidth)
	if !strings.Contains(view, "bogus") {
		t.Errorf("View() = %q, want the error text", view)
	}

	if m.Value() != "mine() ~ ::trunk()" {
		t.Errorf("Value() = %q, an error must not clear the last-typed revset", m.Value())
	}
}

func TestBalanced(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"empty", "", true},
		{"symbol", "trunk()", true},
		{"unclosed nested call", "ancestors(mine()", false},
		{"nested calls balanced", "ancestors(mine() | trunk())", true},
		{"unterminated quote", `description("wip`, false},
		{"paren inside quote is not a paren", `description("(")`, true},
		{"escaped quote does not close the string", `description("a\"b")`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := revsetbox.Balanced(tc.expr); got != tc.want {
				t.Errorf("Balanced(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestSignatureHelpShownOnlyInsideACall(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("")
	m.Focus()

	if strings.Contains(m.View(testWidth), "Ancestors of x") {
		t.Fatal("View() shows signature help before any call is open")
	}

	typeText(&m, "ancestors(")

	if !strings.Contains(m.View(testWidth), "Ancestors of x") {
		t.Errorf("View() = %q, want ancestors()'s signature help inside its call", m.View(testWidth))
	}

	typeText(&m, "x)")

	if strings.Contains(m.View(testWidth), "Ancestors of x") {
		t.Error("View() still shows signature help after the call closed")
	}
}

func TestViewGolden(t *testing.T) {
	t.Parallel()

	m := revsetbox.New("mine() ~ ::trunk()")
	m.Focus()
	typeText(&m, " | anc")

	tuitest.AssertGolden(t, m.View(testWidth))
}
