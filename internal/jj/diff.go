package jj

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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
		// -p puts each changed commit's patch under it, and a flat list keeps the graph's drawing
		// out of text the pane renders itself.
		return []string{"op", "show", "-p", "--no-graph", t.Operation}, nil
	default:
		return append([]string{"diff", "--from", t.From, "--to", t.To}, t.Paths...), nil
	}
}

// DiffCommand returns t ready for tea.ExecProcess, for a caller that hands jj the real terminal. It
// skips jjCommand's editor-disabling environment, because jj's pager needs it.
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

// GitDiff returns t's diff as git-format text, with jj's pager and color disabled. The format is
// git's rather than jj's own because that is the one a renderer parses.
func (c *Client) GitDiff(ctx context.Context, t DiffTarget) (string, error) {
	args, err := t.args()
	if err != nil {
		return "", err
	}

	return c.executeJJ(ctx, append(args, "--git")...)
}

// commitSep ends each commit's diff in GitDiffs' output: record and group separators on a line of
// their own, which no git-format diff jj writes contains.
const commitSep = "\x1e\x1d\n"

// GitDiffs is GitDiff for many commits in one jj call, keyed by the commit IDs the log reported. It
// reads at the operation the log left, without snapshotting, so it never contends with a write.
func (c *Client) GitDiffs(ctx context.Context, commitIDs []string) (map[string]string, error) {
	if len(commitIDs) == 0 {
		return map[string]string{}, nil
	}

	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", strings.Join(commitIDs, "|"),
		"-T", `commit_id.short(12) ++ "\n" ++ diff.git() ++ "`+commitSep[:2]+`\n"`)
	if err != nil {
		return nil, err
	}

	diffs := make(map[string]string, len(commitIDs))

	for _, chunk := range strings.Split(out, commitSep) {
		commit, text, ok := strings.Cut(chunk, "\n")
		if ok && commit != "" {
			diffs[commit] = text
		}
	}

	return diffs, nil
}
