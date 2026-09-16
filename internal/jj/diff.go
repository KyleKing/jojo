package jj

import (
	"context"
	"fmt"
	"os/exec"
)

// DiffTarget names exactly one diff to render: a single revision (jj diff -r), a from/to pair
// (jj diff --from --to), or an operation (jj op show). Exactly one form must be set.
type DiffTarget struct {
	Revision  string
	From      string
	To        string
	Operation string
	// Paths narrows the diff to these files. An operation ignores it, since jj op show takes none.
	Paths []string
}

// ErrInvalidDiffTarget marks a DiffTarget that sets none of, or more than one of, its three forms.
var ErrInvalidDiffTarget = fmt.Errorf("%w: diff target must set exactly one of "+
	"revision, from/to, or operation", ErrCommandFailed)

func (t DiffTarget) args() ([]string, error) {
	revision := t.Revision != ""
	fromTo := t.From != "" && t.To != ""
	operation := t.Operation != ""
	splitPair := (t.From != "") != (t.To != "")

	forms := 0
	for _, set := range []bool{revision, fromTo, operation} {
		if set {
			forms++
		}
	}

	switch {
	case splitPair || forms != 1:
		return nil, ErrInvalidDiffTarget
	case revision:
		return append([]string{"diff", "-r", t.Revision}, t.Paths...), nil
	case operation:
		return []string{"op", "show", t.Operation}, nil
	default:
		return append([]string{"diff", "--from", t.From, "--to", t.To}, t.Paths...), nil
	}
}

// DiffCommand returns t ready for tea.ExecProcess. It skips jjCommand's editor-disabling
// environment because this command is handed the real terminal on purpose, and jj's pager needs it.
func (c *Client) DiffCommand(ctx context.Context, t DiffTarget) (*exec.Cmd, error) {
	args, err := t.args()
	if err != nil {
		return nil, err
	}

	//nolint:gosec // G204: the binary is a literal; only the arguments vary and no shell is involved.
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = c.baseDir

	return cmd, nil
}

// DiffText returns t's diff as plain text, with jj's pager and color disabled.
func (c *Client) DiffText(ctx context.Context, t DiffTarget) (string, error) {
	args, err := t.args()
	if err != nil {
		return "", err
	}

	return c.executeJJ(ctx, append(args, "--no-pager", "--color=never")...)
}
