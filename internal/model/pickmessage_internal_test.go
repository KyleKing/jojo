package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/tuitest"
)

// typeText presses each rune of text, the way it arrives typed into a prompt.
func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()

	for _, r := range text {
		m, _ = pressOne(t, m, string(r))
	}

	return m
}

// -m opens the footer's prompt; Enter keeps the message, which the command line then carries in
// place of -u, and esc (to normal mode, then out) leaves the flag off. -m again drops it.
func TestPickMessage_PromptBuildsTheCommand(t *testing.T) {
	t.Parallel()

	m := verbFrame(t, [2]int{100, 30}, "")
	for _, k := range []string{"s", "-", "m"} {
		m, _ = pressOne(t, m, k)
	}

	if !m.typingMessage() {
		t.Fatal("-m did not open the prompt")
	}

	m = typeText(t, m, "fold it in")
	m, _ = pressOne(t, m, "enter")

	got := m.pick.command()
	if !strings.Contains(got, "-m 'fold it in'") || strings.Contains(got, "--use-destination-message") {
		t.Errorf("the command is %q, want -m and no -u", got)
	}

	if !m.pick.active || m.typingMessage() {
		t.Error("enter on the prompt left it open or closed the picker")
	}

	m, _ = pressOne(t, m, "-")
	m, _ = pressOne(t, m, "m")

	if m.pick.message != "" || m.typingMessage() {
		t.Error("-m a second time did not drop the message")
	}

	m, _ = pressOne(t, m, "-")
	m, _ = pressOne(t, m, "m")
	m = typeText(t, m, "nope")
	m, _ = pressOne(t, m, "esc")

	if !m.typingMessage() {
		t.Fatal("the first esc left the prompt rather than going to normal mode")
	}

	m, _ = pressOne(t, m, "esc")

	if m.pick.message != "" || !m.pick.active || m.typingMessage() {
		t.Error("esc in normal mode kept what was typed, left the prompt open, or closed the picker")
	}
}

// squash -m writes the message as one operation; squash -A makes a new commit after the destination
// holding the source's change; new -m starts a described change.
func TestPickMessageLive(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "top"), "s", "-", "m")
	m = typeText(t, m, "merged")
	ops := opCount(t, dir)
	press(t, m, "enter", "enter")

	if changeFor(t, dir, "merged") == "" || changeFor(t, dir, "top") != "" {
		t.Error("squash -m did not leave one revision described merged")
	}

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("squash -m took %d operations, want 1", got)
	}

	dir = liveRepo(t)
	oldTop := changeFor(t, dir, "top")
	press(t, liveModel(t, dir, "top"), "s", "-", "A", "enter")

	newTop := changeFor(t, dir, "top")
	parent := strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r", newTop+"-",
		"-T", "description.first_line()"))

	if newTop == "" || newTop == oldTop || parent != "middle" {
		t.Errorf("squash -A left top as %q (was %q) on %q, want a new commit after middle", newTop, oldTop, parent)
	}

	dir = liveRepo(t)
	m = press(t, liveModel(t, dir, "middle"), "N", "-", "m")
	m = typeText(t, m, "fresh start")
	press(t, m, "enter", "enter")

	if changeFor(t, dir, "fresh start") == "" {
		t.Error("new -m did not describe the new change")
	}
}

// The footer's prompt open over the picker, and the command line it builds once Enter keeps it.
func TestPickMessage_Frames(t *testing.T) {
	t.Parallel()

	for _, size := range verbSizes {
		for _, done := range []bool{false, true} {
			name := map[bool]string{false: "typing", true: "kept"}[done]

			t.Run(fmt.Sprintf("%s/%dx%d", name, size[0], size[1]), func(t *testing.T) {
				t.Parallel()
				assertMessageFrame(t, size, done)
			})
		}
	}
}

// assertMessageFrame types a message into squash -A's prompt, keeping it when done, and pins the frame.
func assertMessageFrame(t *testing.T, size [2]int, done bool) {
	t.Helper()

	m := verbFrame(t, size, "wkwyyxvq")
	for _, k := range []string{"s", "-", "A", "-", "m"} {
		m, _ = pressOne(t, m, k)
	}

	m = typeText(t, m, "parse: the header")
	if done {
		m, _ = pressOne(t, m, "enter")
	}

	frame := m.render()
	if done && !strings.Contains(m.pick.command(), "-m 'parse: the header'") {
		t.Errorf("the command line lost the message: %s", m.pick.command())
	}

	if !done && !strings.Contains(tuitest.StripANSI(frame), "-m parse: the header") {
		t.Errorf("the prompt is not on the footer:\n%s", tuitest.StripANSI(frame))
	}

	assertVerbFrame(t, frame, size)
}
