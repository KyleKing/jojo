package jj

import (
	"context"
	"strings"
)

// evologTemplate renders one line per version of a change: its commit ID, description, age, the
// operation that produced it, and whether it carries no changes.
const evologTemplate = `commit.commit_id().short(8) ++ "` + fieldSep + `" ++ ` +
	`commit.description().first_line() ++ "` + fieldSep + `" ++ ` +
	`commit.committer().timestamp().ago() ++ "` + fieldSep + `" ++ ` +
	`operation.description().first_line() ++ "` + fieldSep + `" ++ ` +
	`if(commit.empty(),"E","") ++ "\n"`

const evologFieldCount = 5

// Evolog returns how rev reached its current state, newest version first.
func (c *Client) Evolog(ctx context.Context, rev string) ([]Evolution, error) {
	out, err := c.executeJJ(ctx,
		"evolog", "-r", rev, "--no-graph", "--color=never", "--no-pager", "-T", evologTemplate)
	if err != nil {
		return nil, err
	}

	return parseEvolog(out), nil
}

func parseEvolog(out string) []Evolution {
	lines := strings.Split(out, "\n")

	entries := make([]Evolution, 0, len(lines))

	for _, line := range lines {
		fields := strings.Split(line, fieldSep)
		if len(fields) != evologFieldCount {
			continue
		}

		entries = append(entries, Evolution{
			CommitID:    fields[0],
			Description: fields[1],
			Age:         fields[2],
			Operation:   fields[3],
			Empty:       fields[4] == "E",
		})
	}

	return entries
}

// Restore replaces into's content with from's, leaving into's description and other metadata alone.
func (c *Client) Restore(ctx context.Context, from, into string) (string, error) {
	return c.mutate(ctx, "restore", "--from", from, "--into", into)
}
