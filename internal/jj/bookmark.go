package jj

import (
	"context"
	"slices"
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
		"bookmark", "list", "--all-remotes", "-T", bookmarkTemplate)
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
func (c *Client) BookmarkSet(ctx context.Context, name, rev string) (string, error) {
	return c.mutate(ctx, BookmarkSetArgs(name, rev, false)...)
}

// BookmarkSetArgs is the invocation moving name onto rev. A move to anything but a descendant
// (backwards or sideways) is refused by jj unless backwards says --allow-backwards, which is what the
// bookmark picker's -B adds.
func BookmarkSetArgs(name, rev string, backwards bool) []string {
	args := []string{"bookmark", "set", name, "-r", rev}
	if backwards {
		args = append(args, "--allow-backwards")
	}

	return args
}

// backwardsRefusal is jj's words refusing to move a bookmark backwards or sideways, which --allow-backwards
// overrides.
const backwardsRefusal = "backwards or sideways"

// IsBackwards reports whether err is jj refusing to move a bookmark backwards or sideways.
func IsBackwards(err error) bool {
	return err != nil && strings.Contains(FullOutput(err), backwardsRefusal)
}

// BookmarkRename renames a local bookmark, leaving its remote counterparts alone until the next push.
func (c *Client) BookmarkRename(ctx context.Context, from, to string) (string, error) {
	return c.mutate(ctx, "bookmark", "rename", from, to)
}

// BookmarkDelete deletes name and propagates the deletion to its remotes on the next push.
func (c *Client) BookmarkDelete(ctx context.Context, name string) (string, error) {
	return c.mutate(ctx, "bookmark", "delete", name)
}

// BookmarkForget drops jj's record of name without marking a deletion to push.
func (c *Client) BookmarkForget(ctx context.Context, name string) (string, error) {
	return c.mutate(ctx, "bookmark", "forget", name)
}

// BookmarkTrack starts tracking a remote bookmark, named in jj's "name@remote" form.
func (c *Client) BookmarkTrack(ctx context.Context, ref string) (string, error) {
	return c.mutate(ctx, "bookmark", "track", ref)
}

// BookmarkUntrack stops tracking a remote bookmark, named in jj's "name@remote" form.
func (c *Client) BookmarkUntrack(ctx context.Context, ref string) (string, error) {
	return c.mutate(ctx, "bookmark", "untrack", ref)
}

// BookmarkAdvance moves the bookmarks closest to rev onto it, or every eligible one when names is
// empty.
func (c *Client) BookmarkAdvance(ctx context.Context, names []string, rev string) (string, error) {
	args := append([]string{"bookmark", "advance", "--to", rev}, names...)

	return c.mutate(ctx, args...)
}

// Advance is one bookmark BookmarkAdvance would move: its name, the change it sits on now, and
// whether it is the trunk's, which a tug onto unreviewed work would publish on the next push.
type Advance struct {
	Name string
	From string
	// Trunk marks a bookmark named like one trunk() resolves to, or one on an immutable commit.
	Trunk bool
}

// advanceFromRevset is the bookmarks a tug to rev moves: those on its nearest bookmarked ancestors,
// which is jj bookmark advance's own default from-revset, heads(::to & bookmarks()), with rev's own
// bookmarks left out.
func advanceFromRevset(rev string) string {
	return "heads(::(" + rev + ")- & bookmarks())"
}

// advanceTemplate is one line per commit the read names: its change id, its local and remote
// bookmark names, and flags for immutable (I), trunk() (T), and a from-revset member (F).
func advanceTemplate(from string) string {
	return `change_id.shortest(8) ++ "` + fieldSep + `" ++ ` +
		`local_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
		`remote_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
		`if(immutable,"I","") ++ if(self.contained_in("trunk()"),"T","") ++ ` +
		`if(self.contained_in("` + from + `"),"F","") ++ "\n"`
}

const advanceFieldCount = 4

// Advances is what a tug to rev would move, read before it moves anything so the move can be named
// first. A bookmark is the trunk's when trunk() carries its name, locally or at a remote, or when it
// sits on an immutable commit.
func (c *Client) Advances(ctx context.Context, rev string) ([]Advance, error) {
	from := advanceFromRevset(rev)

	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy",
		"-r", from+" | trunk()", "-T", advanceTemplate(from))
	if err != nil {
		return nil, err
	}

	return parseAdvances(out), nil
}

func parseAdvances(out string) []Advance {
	type line struct {
		change, flags string
		local, remote []string
	}

	var lines []line

	trunk := map[string]bool{}

	for _, raw := range strings.Split(out, "\n") {
		fields := strings.Split(raw, fieldSep)
		if len(fields) != advanceFieldCount {
			continue
		}

		l := line{change: fields[0], local: splitList(fields[1]), remote: splitList(fields[2]), flags: fields[3]}
		if strings.Contains(l.flags, "T") {
			for _, name := range append(slices.Clone(l.local), l.remote...) {
				trunk[name] = true
			}
		}

		lines = append(lines, l)
	}

	var advances []Advance

	for _, l := range lines {
		if !strings.Contains(l.flags, "F") {
			continue
		}

		for _, name := range l.local {
			advances = append(advances, Advance{
				Name: name, From: l.change, Trunk: trunk[name] || strings.Contains(l.flags, "I"),
			})
		}
	}

	return advances
}
