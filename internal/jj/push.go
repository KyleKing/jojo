package jj

import (
	"context"
	"fmt"
	"strings"
)

// PushOptions says what `jj git push` sends. Nothing is pushed unless one of Bookmarks or Revisions
// is set, because jj's own default of pushing every eligible bookmark is too broad to trigger from a
// keypress.
type PushOptions struct {
	Remote    string
	Bookmarks []string
	Revisions []string
	// Changes pushes each revision under a bookmark jj names for it (push-<change id>), which is how
	// a revision with no bookmark of its own reaches the remote.
	Changes []string
	DryRun  bool
}

// ErrNothingToPush marks a push with no bookmark or revision named.
var ErrNothingToPush = fmt.Errorf("%w: no bookmark or revision to push", ErrCommandFailed)

// newBookmarkRefusal is the warning jj prints, once per bookmark, when a push by revision would create
// a bookmark on the remote. Naming the bookmark with --bookmark creates and tracks it instead.
const newBookmarkRefusal = "Refusing to create new remote bookmark "

// NewRemoteBookmarks reads the bookmarks a push by revision refused to create, and the remote it
// refused them on, out of jj's notice.
func NewRemoteBookmarks(notice string) ([]string, string) {
	var (
		names  []string
		remote string
	)

	for _, line := range strings.Split(notice, "\n") {
		_, ref, found := strings.Cut(line, newBookmarkRefusal)
		if !found {
			continue
		}

		at := strings.LastIndex(ref, "@")
		if at <= 0 {
			continue
		}

		names = append(names, ref[:at])
		remote = strings.TrimSpace(ref[at+1:])
	}

	return names, remote
}

// Push runs `jj git push`. It is the one action here that leaves the machine, so callers gate it
// behind a confirmation rather than treating it as undoable.
func (c *Client) Push(ctx context.Context, opts PushOptions) (string, error) {
	if len(opts.Bookmarks) == 0 && len(opts.Revisions) == 0 && len(opts.Changes) == 0 {
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

	for _, rev := range opts.Changes {
		args = append(args, "--change", rev)
	}

	if opts.DryRun {
		args = append(args, "--dry-run")
	}

	write := writes
	if opts.DryRun {
		write = reads
	}

	_, notice, err := c.run(ctx, write, args...)
	if err != nil {
		return "", err
	}

	return notice, nil
}
