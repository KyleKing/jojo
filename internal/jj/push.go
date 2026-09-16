package jj

import (
	"context"
	"fmt"
)

// PushOptions says what `jj git push` sends. Nothing is pushed unless one of Bookmarks or Revisions
// is set, because jj's own default of pushing every eligible bookmark is too broad to trigger from a
// keypress.
type PushOptions struct {
	Remote    string
	Bookmarks []string
	Revisions []string
	DryRun    bool
}

// ErrNothingToPush marks a push with no bookmark or revision named.
var ErrNothingToPush = fmt.Errorf("%w: no bookmark or revision to push", ErrCommandFailed)

// Push runs `jj git push`. It is the one action here that leaves the machine, so callers gate it
// behind a confirmation rather than treating it as undoable.
func (c *Client) Push(ctx context.Context, opts PushOptions) (string, error) {
	if len(opts.Bookmarks) == 0 && len(opts.Revisions) == 0 {
		return "", ErrNothingToPush
	}

	args := []string{"git", "push"}
	if opts.Remote != "" {
		args = append(args, "--remote", opts.Remote)
	}

	for _, name := range opts.Bookmarks {
		args = append(args, "--bookmark", name)
	}

	for _, rev := range opts.Revisions {
		args = append(args, "--revision", rev)
	}

	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	_, notice, err := c.execute(ctx, args...)
	if err != nil {
		return "", err
	}

	return notice, nil
}
