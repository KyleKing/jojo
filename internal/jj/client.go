package jj

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// defaultRevsetFallback is jj's own built-in default for revsets.log, used when the config key
// carries no value at all rather than an override.
const defaultRevsetFallback = "builtin_log()"

// Client runs jj in one repository. Every call shells out and blocks, so callers wrap them in a
// tea.Cmd.
type Client struct {
	baseDir string
}

// NewClient runs jj with baseDir as the working directory, so baseDir decides which repository every
// call acts on.
func NewClient(baseDir string) *Client {
	return &Client{baseDir: baseDir}
}

// CheckInstalled reports whether a jj binary is on PATH and runs.
func (c *Client) CheckInstalled(ctx context.Context) error {
	_, err := c.executeJJ(ctx, "--version")

	return err
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

	return cmd
}

func (c *Client) executeJJ(ctx context.Context, args ...string) (string, error) {
	cmd := c.jjCommand(ctx, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", wrapJJError(args, stderr.String())
	}

	return stdout.String(), nil
}

// wrapJJError classifies stderr from a failed jj invocation into the sentinel errors callers inspect
// with errors.Is, falling back to ErrCommandFailed for anything else.
func wrapJJError(args []string, stderr string) error {
	trimmed := strings.TrimSpace(stderr)

	sentinel := ErrCommandFailed

	switch {
	case strings.Contains(stderr, "Failed to parse revset"):
		sentinel = ErrRevsetSyntax
	case strings.Contains(stderr, "is immutable"):
		sentinel = ErrImmutable
	}

	return fmt.Errorf("jj %s: %w: %s", strings.Join(args, " "), sentinel, trimmed)
}
