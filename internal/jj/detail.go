package jj

import (
	"context"
	"strings"
)

// detailTemplate renders the metadata the log row demotes or drops, plus the full description. The
// description goes last and keeps its newlines, so everything before it stays one line per field.
const detailTemplate = `change_id.shortest(8) ++ "` + fieldSep + `" ++ ` +
	`commit_id.short(12) ++ "` + fieldSep + `" ++ ` +
	`author.name() ++ "` + fieldSep + `" ++ ` +
	`author.email() ++ "` + fieldSep + `" ++ ` +
	`author.timestamp().local().format("%Y-%m-%d %H:%M") ++ "` + fieldSep + `" ++ ` +
	`committer.timestamp().local().format("%Y-%m-%d %H:%M") ++ "` + fieldSep + `" ++ ` +
	`parents.map(|p| p.change_id().shortest(8)).join(" ") ++ "` + fieldSep + `" ++ ` +
	`description`

const detailFieldCount = 8

// Detail is everything about one revision that the log row has no room for.
type Detail struct {
	ChangeID    string
	CommitID    string
	AuthorName  string
	AuthorEmail string
	Authored    string
	Committed   string
	Description string
	Parents     []string
}

// RevisionDetail reads one revision's metadata and full description.
func (c *Client) RevisionDetail(ctx context.Context, rev string) (Detail, error) {
	out, err := c.executeJJ(ctx, "log", "--no-graph", "--color=never", "--no-pager", "-r", rev, "-T", detailTemplate)
	if err != nil {
		return Detail{}, err
	}

	return parseDetail(out)
}

func parseDetail(out string) (Detail, error) {
	fields := strings.SplitN(out, fieldSep, detailFieldCount)
	if len(fields) != detailFieldCount {
		return Detail{}, ErrCommandFailed
	}

	detail := Detail{
		ChangeID:    fields[0],
		CommitID:    fields[1],
		AuthorName:  fields[2],
		AuthorEmail: fields[3],
		Authored:    fields[4],
		Committed:   fields[5],
		Description: strings.TrimRight(fields[7], "\n"),
	}

	if parents := strings.Fields(fields[6]); len(parents) > 0 {
		detail.Parents = parents
	}

	return detail, nil
}
