package jj

import (
	"context"
	"fmt"
	"strings"
)

// describedTemplate is a revision's change id as the log prints it, the longer id a generated
// bookmark is named from, the bookmarks a remote holds at this very commit, and the whole description.
const describedTemplate = `change_id.shortest(8) ++ "` + fieldSep + `" ++ change_id.short(12) ++ "` + fieldSep +
	`" ++ ` + pushedTemplate + ` ++ "` + fieldSep + `" ++ description ++ "` + recordSep + `"`

// pushedTemplate names the remote bookmarks on a commit, leaving out jj's own git mirror, which a
// colocated repository keeps beside every local bookmark whether it was pushed or not.
const pushedTemplate = `remote_bookmarks.filter(|b| b.remote() != "git").map(|b| b.name()).join(",")`

// describedFields is how many fields describedTemplate writes.
const describedFields = 4

// Described is a revision's whole description, the id a bookmark generated for it is named from, and
// which of its bookmarks a remote already holds at this commit. PushName is the bookmark
// `jj git push -c` would create for it, from templates.git_push_bookmark, "" when that could not be
// read.
type Described struct {
	LongID      string
	Description string
	PushName    string
	Pushed      []string
}

// defaultPushTemplate is jj's own templates.git_push_bookmark, used when the config cannot be read.
const defaultPushTemplate = `"push-" ++ change_id.short()`

// pushNameSep splits a change id from its push bookmark's name, a byte no template writes.
const pushNameSep = "\x1f"

// PushTemplate is templates.git_push_bookmark as jj resolves it, jj's own default when it cannot be
// read. Since jj 0.47 ignores the older git.push-bookmark-prefix, this is the one name
// `jj git push -c` gives a change.
func (c *Client) PushTemplate(ctx context.Context) string {
	out, err := c.executeJJ(ctx, "config", "get", "templates.git_push_bookmark")
	if err != nil || strings.TrimSpace(out) == "" {
		return defaultPushTemplate
	}

	return strings.TrimSpace(out)
}

// PushNames is the bookmark `jj git push -c` would create for each change named, keyed by the change
// id the log reports. A template jj cannot evaluate answers jj's default names instead.
func (c *Client) PushNames(ctx context.Context, changeIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(changeIDs) == 0 {
		return out, nil
	}

	names := func(template string) (string, error) {
		return c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", strings.Join(changeIDs, "|"),
			"-T", `change_id.shortest(8) ++ "`+pushNameSep+`" ++ (`+template+`) ++ "`+recordSep+`"`)
	}

	text, err := names(c.PushTemplate(ctx))
	if err != nil {
		if text, err = names(defaultPushTemplate); err != nil {
			return nil, err
		}
	}

	for _, record := range strings.Split(text, recordSep) {
		id, name, ok := strings.Cut(strings.TrimLeft(record, "\n"), pushNameSep)
		if ok && name != "" {
			out[id] = name
		}
	}

	return out, nil
}

// Described reads the whole descriptions of the changes named, keyed by the change id the log
// reports, since a pull request's title and body are its subject and the rest.
func (c *Client) Described(ctx context.Context, changeIDs []string) (map[string]Described, error) {
	out := map[string]Described{}
	if len(changeIDs) == 0 {
		return out, nil
	}

	text, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy",
		"-r", strings.Join(changeIDs, "|"), "-T", describedTemplate)
	if err != nil {
		return nil, err
	}

	for _, record := range strings.Split(text, recordSep) {
		fields := strings.SplitN(strings.TrimLeft(record, "\n"), fieldSep, describedFields)
		if len(fields) != describedFields {
			continue
		}

		d := Described{LongID: fields[1], Description: fields[3]}
		if fields[2] != "" {
			d.Pushed = strings.Split(fields[2], ",")
		}

		out[fields[0]] = d
	}

	// The names jj would push under are a nicety: without them a generated bookmark falls back to
	// push-<change id>, which is jj's default anyway.
	if names, err := c.PushNames(ctx, changeIDs); err == nil {
		for id, name := range names {
			if d, ok := out[id]; ok {
				d.PushName = name
				out[id] = d
			}
		}
	}

	return out, nil
}

// ErrNoTrunk marks a repository whose trunk() resolves to no remote bookmark, so a stack has no base
// branch to open pull requests against.
var ErrNoTrunk = fmt.Errorf("%w: trunk() is on no remote bookmark", ErrCommandFailed)

// TrunkBranch is the branch trunk() stands for on the remote, the base the bottom of a stack's pull
// requests targets.
func (c *Client) TrunkBranch(ctx context.Context) (string, error) {
	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy", "-r", "trunk()", "-T", pushedTemplate)
	if err != nil {
		return "", err
	}

	name, _, _ := strings.Cut(strings.TrimSpace(out), ",")
	if name == "" {
		return "", ErrNoTrunk
	}

	return name, nil
}
