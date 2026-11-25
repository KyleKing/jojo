package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/theme"

	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/tuitest"
	"github.com/kyleking/jojo/internal/ui"
)

// liveRepo is a real jj repository shaped as a stack on a root commit with a side branch:
//
//	@ top -> middle -> bottom -> root, and side -> bottom.
func liveRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed; skipping")
	}

	dir := t.TempDir()
	jjRun(t, dir, "git", "init", ".")

	for _, name := range []string{"bottom", "middle", "top"} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		jjRun(t, dir, "commit", "-m", name)
	}

	jjRun(t, dir, "new", "description(exact:'bottom\n')", "-m", "side")
	jjRun(t, dir, "new", "description(exact:'top\n')")

	return dir
}

func jjRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(context.Background(), "jj", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jj %v: %v", args, err)
	}

	return string(out)
}

func changeFor(t *testing.T, dir, description string) string {
	t.Helper()

	return strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy",
		"-r", "description(exact:'"+description+"\n')", "-T", "change_id.shortest(8)"))
}

func opCount(t *testing.T, dir string) int {
	t.Helper()

	return len(strings.Fields(jjRun(t, dir, "op", "log", "--ignore-working-copy", "--no-graph", "-T", "id ++ \" \"")))
}

// repoPrint is every file under .jj and .git with its content, so equal prints mean nothing was
// written there at all.
func repoPrint(t *testing.T, dir string) string {
	t.Helper()

	var lines []string

	for _, sub := range []string{".jj", ".git"} {
		err := filepath.WalkDir(filepath.Join(dir, sub), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}

			data, err := os.ReadFile(path) //nolint:gosec // G304: a file in this test's own repository.
			if err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}

			sum := sha256.Sum256(data)
			lines = append(lines, path+" "+hex.EncodeToString(sum[:]))

			return nil
		})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
	}

	sort.Strings(lines)

	return strings.Join(lines, "\n")
}

// liveModel is the app over dir with its first log loaded and the cursor on the revision described.
func liveModel(t *testing.T, dir, cursorOn string) Model {
	t.Helper()

	opts := testOptions(t)
	opts.Ticker = instantTicks

	m := New(t.Context(), jj.NewClient(dir), ui.New(theme.Macchiato()), dir, "all()", opts)
	m = asModel(t, update(m, tea.WindowSizeMsg{Width: 100, Height: 30}))
	m = pump(t, m, m.loadLog("all()"))

	if !m.log.Focus(changeFor(t, dir, cursorOn)) {
		t.Fatalf("no row described %q", cursorOn)
	}

	return m
}

// instantTicks fires a debounce at once and drops the message timeout, so pump can wait on every
// command to finish rather than guess from the clock which ones are timers. A debounce that fires
// at once still drops a superseded tick, since each carries the sequence number it was set for.
func instantTicks(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg := fn(time.Now())
		if _, ok := msg.(clearMessageMsg); ok {
			return nil
		}

		return msg
	}
}

// cmdDeadline bounds one command, as a hang guard only: a command that outlives it is a failure,
// never a message to drop, because dropping a slow jj call is what made these tests flake.
const cmdDeadline = 2 * time.Minute

// pump runs cmd and feeds what it returns back through Update until nothing is left, the way the
// runtime would. Every command runs to completion; the model's timers go through instantTicks.
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	queue := []tea.Cmd{cmd}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		for _, msg := range runCmd(t, next) {
			var out tea.Cmd

			model, out := m.Update(msg)
			m = asModel(t, model)
			queue = append(queue, out)
		}
	}

	return m
}

func runCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()

	if cmd == nil {
		return nil
	}

	done := make(chan tea.Msg, 1)

	go func() { done <- cmd() }()

	var msg tea.Msg

	select {
	case msg = <-done:
	case <-time.After(cmdDeadline):
		t.Fatalf("a command ran past %s", cmdDeadline)
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(t, c)...)
		}

		return out
	}

	// tea.Sequence's message is unexported; it is a slice of commands run in order.
	if v := reflect.ValueOf(msg); v.IsValid() && v.Kind() == reflect.Slice &&
		v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		var out []tea.Msg
		for i := range v.Len() {
			c, _ := v.Index(i).Interface().(tea.Cmd) //nolint:errcheck // the element type was checked above.
			out = append(out, runCmd(t, c)...)
		}

		return out
	}

	if msg == nil {
		return nil
	}

	return []tea.Msg{msg}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, key := range keys {
		var cmd tea.Cmd

		model, cmd := m.Update(tuitest.Key(key))
		m = pump(t, asModel(t, model), cmd)
	}

	return m
}

// pressFast sends keys without letting their commands run, the way a held key outruns the
// debounce, then lets everything they started finish.
func pressFast(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	cmds := make([]tea.Cmd, 0, len(keys))

	for _, key := range keys {
		model, cmd := m.Update(tuitest.Key(key))
		m = asModel(t, model)
		cmds = append(cmds, cmd)
	}

	return pump(t, m, tea.Batch(cmds...))
}

// Moving the destination, holding j past the debounce, and canceling must leave the repository
// exactly as it was: not an operation, not a ref, not a byte under .jj or .git.
func TestPickerLive_MovingAndCancelingLeaveNoTrace(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	before, ops := repoPrint(t, dir), opCount(t, dir)

	m = press(t, m, "r")
	if m.pick.sandbox == nil {
		t.Fatalf("the picker opened no sandbox: %v", m.pick.sandboxErr)
	}

	sandbox := m.pick.sandbox.Dir()
	m = press(t, m, "j")
	m = pressFast(t, m, "j", "j", "k", "j", "a", "b", "s", "e")

	if m.pick.preview == nil || m.pick.shownFor != m.pick.command() {
		t.Fatalf("the last preview did not land: shown for %q, command %q (%v)",
			m.pick.shownFor, m.pick.command(), m.pick.previewErr)
	}

	m = press(t, m, "esc")
	if m.pick.active {
		t.Fatal("esc left the picker open")
	}

	if after := repoPrint(t, dir); after != before {
		t.Error("previewing and canceling wrote to the repository")
	}

	if got := opCount(t, dir); got != ops {
		t.Errorf("previewing and canceling left %d operations behind", got-ops)
	}

	if _, err := os.Stat(sandbox); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the sandbox outlived esc: %v", err)
	}
}

// A destination jj refuses is previewed as the refusal, and the refusal leaves nothing behind.
func TestPickerLive_RefusedPreviewLeavesNoTrace(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "middle")
	before, ops := repoPrint(t, dir), opCount(t, dir)

	// -s moves middle with its descendants, and top is one of them: jj refuses to rebase a revision
	// onto its own descendant.
	m = press(t, m, "r", "-", "s")
	m.pick.dest = changeFor(t, dir, "top")
	m = pump(t, m, m.changed())

	if m.pick.previewErr == nil {
		t.Fatal("rebasing onto a descendant previewed without a refusal")
	}

	if !strings.Contains(tuitest.StripANSI(m.render()), "jj would refuse") {
		t.Error("the footer does not say jj would refuse")
	}

	m = press(t, m, "esc")

	if after := repoPrint(t, dir); after != before {
		t.Error("a refused preview wrote to the repository")
	}

	if got := opCount(t, dir); got != ops {
		t.Errorf("a refused preview left %d operations behind", got-ops)
	}
}

// Enter runs what the preview drew, as one operation, so u takes all of it back.
func TestPickerLive_ApplyIsOneOperation(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	ops := opCount(t, dir)

	m = press(t, m, "r")
	m.pick.dest = changeFor(t, dir, "side")
	m = pump(t, m, m.changed())

	sandbox := m.pick.sandbox.Dir()
	moved := false

	for _, row := range m.pick.preview {
		if row.Revision != nil && row.Revision.Description == "top" {
			moved = true
		}
	}

	if !moved {
		t.Fatal("the preview lost the revision being moved")
	}

	m = press(t, m, "enter")

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("applying added %d operations, want 1", got)
	}

	parent := strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy",
		"-r", changeFor(t, dir, "top")+"-", "-T", "description.first_line()"))
	if parent != "side" {
		t.Errorf("after enter top sits on %q, want side", parent)
	}

	if _, err := os.Stat(sandbox); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the sandbox outlived enter: %v", err)
	}

	if m.pick.active {
		t.Error("the picker stayed open after enter")
	}
}

// Quitting from the picker takes the sandbox with it, since no command runs after tea.Quit.
func TestPickerLive_QuitRemovesTheSandbox(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := press(t, liveModel(t, dir, "top"), "s")

	if m.pick.sandbox == nil {
		t.Fatalf("no sandbox: %v", m.pick.sandboxErr)
	}

	sandbox := m.pick.sandbox.Dir()

	if _, cmd := m.Update(tuitest.Key("q")); cmd == nil {
		t.Fatal("q did not quit")
	}

	if _, err := os.Stat(sandbox); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the sandbox outlived q: %v", err)
	}
}

// While the log shows an earlier operation every write verb is refused with a notice, and nothing
// reaches the repository; esc comes back to the present.
func TestTravelLive_ReadOnly(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "middle")

	jjRun(t, dir, "abandon", "description(exact:'side\n')")
	m = pump(t, m, m.loadLog("all()"))
	ops := opCount(t, dir)

	m = press(t, m, "[", "o")
	if !m.travel.active {
		t.Fatal("[o did not start time travel")
	}

	// The debounce has to fire before the past is read.
	m = pump(t, m, func() tea.Msg { return travelTickMsg{seq: m.travel.seq} })

	if !strings.Contains(tuitest.StripANSI(m.render()), "side") {
		t.Errorf("one operation back the abandoned revision is missing:\n%s", tuitest.StripANSI(m.render()))
	}

	before := repoPrint(t, dir)

	for _, key := range []string{"a", "d", "s", "enter", "esc"} {
		if key == "enter" {
			m = press(t, m, key)
			if !m.confirm.IsOpen() {
				t.Error("enter did not ask before restoring")
			}

			m = press(t, m, "n")

			continue
		}

		if key == "esc" {
			break
		}

		m = press(t, m, key)

		if m.pick.active || m.prompt.IsOpen() {
			t.Errorf("%s opened a write while time traveling", key)
		}

		if !strings.Contains(m.message, "read-only") {
			t.Errorf("%s gave no read-only notice, footer says %q", key, m.message)
		}
	}

	if after := repoPrint(t, dir); after != before || opCount(t, dir) != ops {
		t.Error("time travel wrote to the repository")
	}

	m = press(t, m, "esc")
	if m.travel.active {
		t.Error("esc did not come back to the present")
	}
}

// enter on an earlier operation restores it after the confirmation, as one operation.
func TestTravelLive_RestoreAfterConfirming(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "middle")

	jjRun(t, dir, "abandon", "description(exact:'side\n')")
	m = pump(t, m, m.loadLog("all()"))
	ops := opCount(t, dir)

	m = press(t, m, "[", "o", "enter", "y")

	if got := opCount(t, dir) - ops; got != 1 {
		t.Errorf("restoring added %d operations, want 1", got)
	}

	if m.travel.active {
		t.Error("the restore left the log in the past")
	}

	if changeFor(t, dir, "side") == "" {
		t.Error("restoring the earlier operation did not bring back the abandoned revision")
	}
}

// A split that wrote asks for a description of each commit it made with none, one prompt at a time,
// so no undescribed commit is left for a push to refuse later.
func TestSplit_AsksToDescribeTheCommitsItMade(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "top")
	known := m.knownChanges()

	// What jj-diff's split leaves: a new commit before the source, with no description.
	jjRun(t, dir, "new", "--no-edit", "-B", "description(exact:'top\n')")

	next, cmd := m.Update(splitWroteMsg{known: known, done: messageMsg{text: "jj-diff wrote", wrote: true}})
	m = pump(t, asModel(t, next), tea.Batch(cmd, m.loadLog("all()")))

	if !m.prompt.IsOpen() {
		t.Fatalf("no describe prompt after the split; footer %q", m.message)
	}

	m = press(t, m, "n", "a", "m", "e", "d", "enter")

	if m.prompt.IsOpen() {
		t.Fatal("the prompt asked again after the only new commit was described")
	}

	got := strings.TrimSpace(jjRun(t, dir, "log", "--no-graph", "--ignore-working-copy", "-r",
		"parents(description(exact:'top\n'))", "-T", "description"))
	if got != "named" {
		t.Errorf("the split's new commit is described %q, want named", got)
	}
}

// r starts on the parent, where -r would lift the revision out of its stack and hang its children on
// the parent: Enter there by reflex writes nothing and says to pick a destination first.
func TestPicker_RebaseEnterOnTheStartingParentWritesNothing(t *testing.T) {
	t.Parallel()

	dir := liveRepo(t)
	m := liveModel(t, dir, "middle")
	ops := opCount(t, dir)

	m = press(t, m, "r", "enter")

	if got := opCount(t, dir); got != ops {
		t.Fatalf("enter on the starting destination wrote %d operations", got-ops)
	}

	if !m.pick.active || !strings.Contains(m.message, "still the parent") {
		t.Errorf("picker open %v, footer %q; want it open and saying why", m.pick.active, m.message)
	}

	press(t, m, "j", "enter")

	if opCount(t, dir) == ops {
		t.Error("enter after moving the destination wrote nothing")
	}
}
