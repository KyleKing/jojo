package jj

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kyleking/aragonite/trail"
)

// defaultRevsetFallback is jj's own built-in default for revsets.log, used when the config key
// carries no value at all rather than an override.
const defaultRevsetFallback = "builtin_log()"

// Client runs jj in one repository. Every call shells out and blocks, so callers wrap them in a
// tea.Cmd.
type Client struct {
	// queue serializes every jj invocation that can write, the snapshotting log read included,
	// because a snapshot taken while a write is still running diverges the op heads and strands the
	// checkout. A read that ignores the working copy writes nothing, so it runs beside them. A
	// channel rather than a mutex, so the copy IgnoringImmutable returns shares the one queue with
	// its source.
	queue chan struct{}
	// linksUnsafe is set once a preview sandbox has seen a file it hard-linked change, after which
	// this client's sandboxes copy instead. A pointer, so IgnoringImmutable's copy shares it.
	linksUnsafe *atomic.Bool
	// trail records every call this client makes, shared by the copy IgnoringImmutable returns, and
	// nil to record nothing.
	trail   *trail.Trail
	baseDir string
	// ignoreImmutable adds --ignore-immutable to every write. jj refuses to rewrite immutable
	// revisions for a reason, so only a client built by IgnoringImmutable sets it.
	ignoreImmutable bool
}

// IgnoringImmutable returns a client whose writes rewrite revisions jj protects. Callers gate it
// behind a confirmation, because no other action here can change published history.
func (c *Client) IgnoringImmutable() *Client {
	forcing := *c
	forcing.ignoreImmutable = true

	return &forcing
}

// NewClient runs jj with baseDir as the working directory, so baseDir decides which repository every
// call acts on.
func NewClient(baseDir string) *Client {
	return &Client{baseDir: baseDir, queue: make(chan struct{}, 1), linksUnsafe: &atomic.Bool{}}
}

// WithTrail records every jj call the client makes in t from now on: its command line, whether it
// wrote, how long it took, how it exited, and what it said on stderr, with the operation a write
// created. It answers the client, so a caller can build and wire it in one expression.
func (c *Client) WithTrail(t *trail.Trail) *Client {
	c.trail = t

	return c
}

// Trail is the log the client records into, nil when it records nothing.
func (c *Client) Trail() *trail.Trail { return c.trail }

// CheckInstalled reports whether a jj binary is on PATH. It deliberately does not run jj: every
// invocation costs about as long as the first frame takes to paint, and the caller's next call
// proves the binary works anyway.
func (*Client) CheckInstalled() error {
	if _, err := exec.LookPath("jj"); err != nil {
		return fmt.Errorf("%w: jj is not on PATH", ErrCommandFailed)
	}

	return nil
}

// Root returns the repository root for the client's base directory.
func (c *Client) Root(ctx context.Context) (string, error) {
	out, err := c.executeJJ(ctx, "root")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// DefaultRevset returns the configured revsets.log, falling back to jj's own built-in default when
// the key carries no value.
func (c *Client) DefaultRevset(ctx context.Context) (string, error) {
	out, err := c.executeJJ(ctx, "config", "get", "revsets.log")
	if err != nil {
		if strings.Contains(err.Error(), "Value not found") {
			return defaultRevsetFallback, nil
		}

		return "", err
	}

	return strings.TrimSpace(out), nil
}

// DiffEditorConfigured reports whether the user's jj config sets ui.diff-editor, at any level. An
// unset key comes back from jj as a failure, which is the answer rather than an error here.
func (c *Client) DiffEditorConfigured(ctx context.Context) (bool, error) {
	if _, err := c.executeJJ(ctx, "config", "get", "ui.diff-editor"); err != nil {
		if strings.Contains(err.Error(), "Value not found") {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (c *Client) jjCommand(ctx context.Context, args ...string) *exec.Cmd {
	//nolint:gosec // G204: the binary is a literal; only the arguments vary and no shell is involved.
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = c.baseDir
	// A jj call that reaches for an editor has to fail rather than block: this
	// process holds the terminal, so a spawned editor would hang unseen. The
	// interactive verbs bypass this by handing the terminal over instead.
	cmd.Env = append(os.Environ(), "JJ_EDITOR=false", "EDITOR=false", "VISUAL=false")

	return cmd
}

func (c *Client) executeJJ(ctx context.Context, args ...string) (string, error) {
	out, _, err := c.execute(ctx, args...)

	return out, err
}

// writing marks a call the trail files as a write.
type writing bool

const (
	reads  writing = false
	writes writing = true
)

// AcquireQueue takes the command queue for a jj invocation this package does not run itself,
// which is how the interactive verbs handed the real terminal serialize against every other
// command. The caller must ReleaseQueue, or every later jj call waits forever.
func (c *Client) AcquireQueue() { c.queue <- struct{}{} }

// ReleaseQueue releases the queue AcquireQueue took.
func (c *Client) ReleaseQueue() { <-c.queue }

// The flags jojo adds to a jj call: no pager, no color in output it parses, and for a read, no
// snapshot of the working copy.
const (
	noPager           = "--no-pager"
	colorNever        = "--color=never"
	ignoreWorkingCopy = "--ignore-working-copy"
)

// execute returns jj's stdout and, on success, the notices it writes to stderr, which is where jj
// says what it did. A read never snapshots the working copy: the log read (Log) is the one place a
// read does, so every other read sees the repository as the log last showed it, takes no
// working-copy lock, and writes no "snapshot working copy" operation for u to offer back.
func (c *Client) execute(ctx context.Context, args ...string) (string, string, error) {
	if !slices.Contains(args, ignoreWorkingCopy) {
		args = append([]string{ignoreWorkingCopy}, args...)
	}

	// Without a snapshot the read cannot diverge the op heads, so it need not wait its turn.
	return c.runLocked(ctx, reads, args...)
}

// Snapshot records what is on disk into the working-copy commit, which a read of @ otherwise does
// not see until the next log read.
func (c *Client) Snapshot(ctx context.Context) error {
	_, err := c.runSnapshot(ctx, "log", "--no-graph", "-r", "@", "-T", "''")

	return err
}

// runSnapshot is execute for the one read that snapshots the working copy first.
func (c *Client) runSnapshot(ctx context.Context, args ...string) (string, error) {
	out, _, err := c.run(ctx, reads, args...)

	return out, err
}

// run is execute for a call the trail files as a read or a write.
func (c *Client) run(ctx context.Context, write writing, args ...string) (string, string, error) {
	c.queue <- struct{}{}
	defer func() { <-c.queue }()

	return c.runLocked(ctx, write, args...)
}

// executeLocked is execute for a caller already holding the queue.
func (c *Client) executeLocked(ctx context.Context, args ...string) (string, string, error) {
	return c.runLocked(ctx, reads, args...)
}

// runLocked runs jj and records the call in the trail. The trail files it as the command a reader
// would type, `jj` and args, without the --no-pager and --color=never every captured call adds,
// which change only how jj prints.
func (c *Client) runLocked(ctx context.Context, write writing, args ...string) (string, string, error) {
	// Captured output is parsed, so a user's ui.color = "always" or a pager must not reach it.
	cmd := c.jjCommand(ctx, append([]string{noPager, colorNever}, args...)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()

	if c.trail != nil {
		call := trail.Call{
			Argv: append([]string{"jj"}, args...), Dir: c.baseDir, Write: bool(write), Start: start,
			Took: time.Since(start), Exit: trail.ExitCode(err), Stderr: stderr.String(),
		}
		if write && err == nil {
			call.Op = c.headOpLocked(ctx)
		}

		c.trail.Add(call)
	}

	if err != nil {
		return "", "", wrapJJError(args, stderr.String())
	}

	return stdout.String(), stderr.String(), nil
}

// headOpLocked is the newest operation's id, read without snapshotting and without a trail row of
// its own, so a write's row can be undone back to; "" when it cannot be read.
func (c *Client) headOpLocked(ctx context.Context) string {
	cmd := c.jjCommand(ctx, "--no-pager", "--color=never", "--ignore-working-copy",
		"op", "log", "--no-graph", "-n", "1", "-T", "id.short(12)")

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// nothingChangedNotice is what jj prints when a command succeeds and moves
// nothing, which the caller must not report as a completed edit.
const nothingChangedNotice = "Nothing changed."

// mutate runs a jj write and returns the command line it ran, which is what the UI shows in place of
// a description of the edit. It reports ErrNothingChanged when jj did nothing.
func (c *Client) mutate(ctx context.Context, args ...string) (string, error) {
	if c.ignoreImmutable {
		args = append(args, "--ignore-immutable")
	}

	ran := CommandLine(args)

	_, notice, err := c.run(ctx, writes, args...)
	if err != nil {
		return ran, err
	}

	if strings.Contains(notice, nothingChangedNotice) {
		return ran, ErrNothingChanged
	}

	return ran, nil
}

// CommandLine renders args as the jj invocation a reader could paste into a shell, quoting only what
// a shell would otherwise split or interpret. It is the line the trail files the call under.
func CommandLine(args []string) string {
	return trail.Shell(append([]string{"jj"}, args...))
}

// CommandError is a failed jj invocation. Its Error is the one line worth putting in a footer, and
// Output keeps everything jj wrote, since the line that explains a failure is not always the first.
type CommandError struct {
	// sentinel is what errors.Is matches: ErrCommandFailed, or the class wrapJJError found.
	sentinel error
	summary  string
	Stderr   string
	Args     []string
}

func (e *CommandError) Error() string { return e.summary }

func (e *CommandError) Unwrap() error { return e.sentinel }

// Output is the failed command line followed by everything jj wrote to stderr.
func (e *CommandError) Output() string {
	return CommandLine(e.Args) + "\n\n" + strings.TrimSpace(e.Stderr)
}

// FullOutput is what jj wrote for err when err came from a jj invocation, and err's own text
// otherwise, which is what a "show me everything" view needs regardless of where err came from.
func FullOutput(err error) string {
	if err == nil {
		return ""
	}

	var failed *CommandError
	if errors.As(err, &failed) {
		return failed.Output()
	}

	return err.Error()
}

// wrapJJError classifies stderr from a failed jj invocation into the sentinel errors callers inspect
// with errors.Is. A classified failure reports jj's own complaint alone, because it is shown to the
// user in place; an unclassified one keeps the subcommand, so the footer says what failed.
func wrapJJError(args []string, stderr string) error {
	trimmed := strings.TrimSpace(stderr)

	failed := &CommandError{Args: args, Stderr: stderr}

	for _, class := range classes {
		if strings.Contains(stderr, class.marker) {
			failed.sentinel = class.sentinel
			failed.summary = class.sentinel.Error() + ": " + complaint(trimmed)

			return failed
		}
	}

	// The sentinel's own words add nothing to a line that already says which command failed.
	failed.sentinel = ErrCommandFailed
	failed.summary = "jj " + subcommand(args) + ": " + complaint(trimmed)

	return failed
}

// classes are the failures a caller recovers from, each found by a phrase jj always prints for it.
var classes = []struct {
	sentinel error
	marker   string
}{
	{ErrRevsetSyntax, "Failed to parse revset"},
	{ErrImmutable, "is immutable"},
	{ErrPushRefused, "Won't push"},
	{ErrStaleWorkingCopy, "working copy is stale"},
}

// subcommand is the verbs of args without their flags and operands, which is enough to say which
// command failed without repeating a description the user just typed.
func subcommand(args []string) string {
	var verbs []string

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || len(verbs) == maxVerbs {
			break
		}

		verbs = append(verbs, arg)
	}

	return strings.Join(verbs, " ")
}

// maxVerbs covers jj's deepest commands (`jj git remote add`) and stops before their operands.
const maxVerbs = 2

// complaint reduces jj's stderr to its headline and the one line that says something the headline
// does not: the "= detail" of a caret diagram, or jj's own "Hint:" line. Both are where jj explains
// what it wanted, which is the part worth putting in front of someone. The headline is jj's "Error:"
// line wherever it falls, because jj prints progress and warnings ahead of it.
func complaint(stderr string) string {
	lines := strings.Split(stderr, "\n")
	at := headlineAt(lines)
	headline := strings.TrimSpace(lines[at])

	for _, prefix := range []string{"Error: ", "error: "} {
		headline = strings.TrimPrefix(headline, prefix)
	}

	for _, line := range lines[at+1:] {
		detail := detailOf(line)
		if detail == "" || strings.Contains(headline, detail) {
			continue
		}

		return headline + " (" + detail + ")"
	}

	return headline
}

// headlineAt is the index of jj's error line, or of the first line that is neither a warning nor a
// hint when jj wrote none.
func headlineAt(lines []string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Error: ") || strings.HasPrefix(trimmed, "error: ") {
			return i
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "Warning: ") && !strings.HasPrefix(trimmed, "Hint: ") &&
			!strings.HasPrefix(line, " ") {
			return i
		}
	}

	return 0
}

func detailOf(line string) string {
	trimmed := strings.TrimSpace(line)

	if detail, found := strings.CutPrefix(trimmed, "= "); found {
		return detail
	}

	if detail, found := strings.CutPrefix(trimmed, "Hint: "); found {
		return detail
	}

	return ""
}
