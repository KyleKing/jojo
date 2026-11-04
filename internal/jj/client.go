package jj

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// defaultRevsetFallback is jj's own built-in default for revsets.log, used when the config key
// carries no value at all rather than an override.
const defaultRevsetFallback = "builtin_log()"

// Client runs jj in one repository. Every call shells out and blocks, so callers wrap them in a
// tea.Cmd.
type Client struct {
	baseDir string
	// ignoreImmutable adds --ignore-immutable to every write. jj refuses to rewrite immutable
	// revisions for a reason, so only a client built by IgnoringImmutable sets it.
	ignoreImmutable bool
	// queue serializes every jj invocation, including the reads, because a snapshot taken while a
	// write is still running diverges the op heads and strands the checkout. A pointer, so the
	// copy IgnoringImmutable returns shares one queue with its source.
	queue *sync.Mutex
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
	return &Client{baseDir: baseDir, queue: &sync.Mutex{}}
}

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

// Lock takes the command queue for a jj invocation this package does not run itself, which is how
// the interactive verbs handed the real terminal serialize against every other command. The caller
// must Unlock, or every later jj call waits forever.
func (c *Client) Lock() { c.queue.Lock() }

// Unlock releases the queue Lock took.
func (c *Client) Unlock() { c.queue.Unlock() }

// execute returns jj's stdout and, on success, the notices it writes to stderr,
// which is where jj says what it did.
func (c *Client) execute(ctx context.Context, args ...string) (string, string, error) {
	c.queue.Lock()
	defer c.queue.Unlock()

	cmd := c.jjCommand(ctx, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", "", wrapJJError(args, stderr.String())
	}

	return stdout.String(), stderr.String(), nil
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

	_, notice, err := c.execute(ctx, args...)
	if err != nil {
		return ran, err
	}

	if strings.Contains(notice, nothingChangedNotice) {
		return ran, ErrNothingChanged
	}

	return ran, nil
}

// CommandLine renders args as the jj invocation a reader could paste into a shell, quoting only what
// a shell would otherwise split or interpret.
func CommandLine(args []string) string {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, "jj")

	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}

	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\"'\\$`*?[]()<>|&;#~!") {
		return arg
	}

	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

// wrapJJError classifies stderr from a failed jj invocation into the sentinel errors callers inspect
// with errors.Is. A classified failure reports jj's own complaint alone, because it is shown to the
// user in place; an unclassified one keeps the command line, because it is a bug report.
func wrapJJError(args []string, stderr string) error {
	trimmed := strings.TrimSpace(stderr)

	switch {
	case strings.Contains(stderr, "Failed to parse revset"):
		return fmt.Errorf("%w: %s", ErrRevsetSyntax, complaint(trimmed))
	case strings.Contains(stderr, "is immutable"):
		return fmt.Errorf("%w: %s", ErrImmutable, complaint(trimmed))
	case strings.Contains(stderr, "Won't push"):
		return fmt.Errorf("%w: %s", ErrPushRefused, complaint(trimmed))
	}

	return fmt.Errorf("jj %s: %w: %s", strings.Join(args, " "), ErrCommandFailed, trimmed)
}

// complaint reduces jj's stderr to its headline and the one line that says something the headline
// does not: the "= detail" of a caret diagram, or jj's own "Hint:" line. Both are where jj explains
// what it wanted, which is the part worth putting in front of someone.
func complaint(stderr string) string {
	lines := strings.Split(stderr, "\n")
	headline := strings.TrimPrefix(strings.TrimSpace(lines[0]), "Error: ")

	for _, line := range lines[1:] {
		detail := detailOf(line)
		if detail == "" || strings.Contains(headline, detail) {
			continue
		}

		return headline + " (" + detail + ")"
	}

	return headline
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
