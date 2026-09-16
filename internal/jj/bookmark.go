package jj

import (
	"context"
	"strconv"
	"strings"
)

// gitRemote is the pseudo-remote a colocated repository reports for its own git refs. It is not
// something a bookmark can be pushed to or tracked against, so it never reaches the caller.
const gitRemote = "git"

// bookmarkTemplate renders one line per bookmark: its name, the remote it lives on, the change it
// points at, whether it is tracked, how far it has diverged from its tracking ref, whether it is
// conflicted, and the target's description. The ahead and behind counts are only defined for a
// tracked ref, so they are guarded rather than read unconditionally.
const bookmarkTemplate = `self.name() ++ "` + fieldSep + `" ++ ` +
	`self.remote() ++ "` + fieldSep + `" ++ ` +
	`self.normal_target().change_id().shortest(8) ++ "` + fieldSep + `" ++ ` +
	`if(self.tracked(),"T","") ++ "` + fieldSep + `" ++ ` +
	`if(self.tracked(), self.tracking_ahead_count().lower(), "") ++ "` + fieldSep + `" ++ ` +
	`if(self.tracked(), self.tracking_behind_count().lower(), "") ++ "` + fieldSep + `" ++ ` +
	`if(self.conflict(),"C","") ++ "` + fieldSep + `" ++ ` +
	`self.normal_target().description().first_line() ++ "\n"`

const bookmarkFieldCount = 8

// Bookmarks lists every bookmark and its remote counterparts.
func (c *Client) Bookmarks(ctx context.Context) ([]Bookmark, error) {
	out, err := c.executeJJ(ctx,
		"bookmark", "list", "--all-remotes", "--color=never", "--no-pager", "-T", bookmarkTemplate)
	if err != nil {
		return nil, err
	}

	return parseBookmarks(out), nil
}

func parseBookmarks(out string) []Bookmark {
	lines := strings.Split(out, "\n")

	marks := make([]Bookmark, 0, len(lines))

	for _, line := range lines {
		fields := strings.Split(line, fieldSep)
		if len(fields) != bookmarkFieldCount || fields[1] == gitRemote {
			continue
		}

		marks = append(marks, Bookmark{
			Name:        fields[0],
			Remote:      fields[1],
			ChangeID:    fields[2],
			Tracked:     fields[3] == "T",
			Ahead:       atoi(fields[4]),
			Behind:      atoi(fields[5]),
			Conflict:    fields[6] == "C",
			Description: fields[7],
		})
	}

	return marks
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}

	return n
}

// Ref names the bookmark as jj does, so a caller can hand it straight back to jj.
func (b Bookmark) Ref() string {
	if b.Remote == "" {
		return b.Name
	}

	return b.Name + "@" + b.Remote
}

// BookmarkSet creates name or moves it onto rev, which is the one verb that covers both.
func (c *Client) BookmarkSet(ctx context.Context, name, rev string) error {
	return c.mutate(ctx, "bookmark", "set", name, "-r", rev)
}

// BookmarkRename renames a local bookmark, leaving its remote counterparts alone until the next push.
func (c *Client) BookmarkRename(ctx context.Context, from, to string) error {
	return c.mutate(ctx, "bookmark", "rename", from, to)
}

// BookmarkDelete deletes name and propagates the deletion to its remotes on the next push.
func (c *Client) BookmarkDelete(ctx context.Context, name string) error {
	return c.mutate(ctx, "bookmark", "delete", name)
}

// BookmarkForget drops jj's record of name without marking a deletion to push.
func (c *Client) BookmarkForget(ctx context.Context, name string) error {
	return c.mutate(ctx, "bookmark", "forget", name)
}

// BookmarkTrack starts tracking a remote bookmark, named in jj's "name@remote" form.
func (c *Client) BookmarkTrack(ctx context.Context, ref string) error {
	return c.mutate(ctx, "bookmark", "track", ref)
}

// BookmarkUntrack stops tracking a remote bookmark, named in jj's "name@remote" form.
func (c *Client) BookmarkUntrack(ctx context.Context, ref string) error {
	return c.mutate(ctx, "bookmark", "untrack", ref)
}

// BookmarkAdvance moves the bookmarks closest to rev onto it, or every eligible one when names is
// empty.
func (c *Client) BookmarkAdvance(ctx context.Context, names []string, rev string) error {
	args := append([]string{"bookmark", "advance", "--to", rev}, names...)

	return c.mutate(ctx, args...)
}
