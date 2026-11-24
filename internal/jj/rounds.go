package jj

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// Version is one entry of a change's evolution read for rounds: the commit it was, the commits it
// was made from, the operation that made it, and what that operation did to the change's own patch.
type Version struct {
	ChangeID string
	CommitID string
	// Description is the whole description, so a describe that changed only the body still reads as
	// a change.
	Description string
	// Operation is the first line of the operation's description, and Age how long ago it ended.
	Operation string
	Age       string
	// Predecessors are the commits this version was made from, as change id and commit id. A squash
	// names a commit of another change here as well as the change's own previous version.
	Predecessors []Predecessor
	// Files, Added, and Removed size the interdiff from the predecessors: jj rebases them onto this
	// version's parents first, so a pure rebase reads as zero.
	Files   int
	Added   int
	Removed int
	// Snapshot marks a version the working copy's snapshot made rather than a command.
	Snapshot bool
}

// Predecessor is one commit a version was made from.
type Predecessor struct {
	ChangeID string
	CommitID string
}

// versionsTemplate is one line per evolog entry. The description goes last and as JSON, since it is
// the one field that may hold a newline.
const versionsTemplate = `commit.change_id() ++ "` + fieldSep + `" ++ ` +
	`commit.commit_id() ++ "` + fieldSep + `" ++ ` +
	`self.predecessors().map(|c| c.change_id() ++ ":" ++ c.commit_id()).join(",") ++ "` + fieldSep + `" ++ ` +
	`operation.description().first_line() ++ "` + fieldSep + `" ++ ` +
	`operation.time().end().ago() ++ "` + fieldSep + `" ++ ` +
	`if(operation.snapshot(), "S", "") ++ "` + fieldSep + `" ++ ` +
	`self.inter_diff().files().len() ++ "` + fieldSep + `" ++ ` +
	`self.inter_diff().stat().total_added() ++ "` + fieldSep + `" ++ ` +
	`self.inter_diff().stat().total_removed() ++ "` + fieldSep + `" ++ ` +
	`commit.description().escape_json() ++ "\n"`

const versionsFieldCount = 10

// Versions reads how the change at rev evolved, oldest version first and only the change's own
// versions: evolog also walks the history of anything squashed into it, which belongs to other
// changes. It reads without snapshotting, so stepping through the past never writes an operation.
func (c *Client) Versions(ctx context.Context, rev string) ([]Version, error) {
	out, err := c.executeJJ(ctx, "evolog", "--ignore-working-copy", "--no-graph", "--reversed",
		"-r", rev, "-T", versionsTemplate)
	if err != nil {
		return nil, err
	}

	return ownVersions(parseVersions(out)), nil
}

func parseVersions(out string) []Version {
	var versions []Version

	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, fieldSep)
		if len(fields) != versionsFieldCount {
			continue
		}

		var description string
		if err := json.Unmarshal([]byte(fields[9]), &description); err != nil {
			description = fields[9]
		}

		versions = append(versions, Version{
			ChangeID:     fields[0],
			CommitID:     fields[1],
			Predecessors: parsePredecessors(fields[2]),
			Operation:    fields[3],
			Age:          fields[4],
			Snapshot:     fields[5] == "S",
			Files:        atoiOrZero(fields[6]),
			Added:        atoiOrZero(fields[7]),
			Removed:      atoiOrZero(fields[8]),
			Description:  description,
		})
	}

	return versions
}

func parsePredecessors(field string) []Predecessor {
	var out []Predecessor

	for _, pair := range splitList(field) {
		change, commit, ok := strings.Cut(pair, ":")
		if ok {
			out = append(out, Predecessor{ChangeID: change, CommitID: commit})
		}
	}

	return out
}

// ownVersions keeps the versions of the change the newest entry belongs to.
func ownVersions(all []Version) []Version {
	if len(all) == 0 {
		return nil
	}

	change := all[len(all)-1].ChangeID
	out := make([]Version, 0, len(all))

	for i := range all {
		if all[i].ChangeID == change {
			out = append(out, all[i])
		}
	}

	return out
}

// Previous is the commit this version is compared against: its own change's predecessor where it
// has one, any predecessor otherwise (a split names the change it was cut from), and "" for the
// version that created the change.
func (v Version) Previous() string {
	for _, p := range v.Predecessors {
		if p.ChangeID == v.ChangeID {
			return p.CommitID
		}
	}

	if len(v.Predecessors) > 0 {
		return v.Predecessors[0].CommitID
	}

	return ""
}

// Interdiff returns what changed in the patch from one commit to another, in git format: jj rebases
// from onto to's parents before comparing, so what the parents changed in between drops out.
func (c *Client) Interdiff(ctx context.Context, from, to string) (string, error) {
	return c.executeJJ(ctx, "interdiff", "--ignore-working-copy", "--from", from, "--to", to, "--git")
}

// CommitDiff is one commit's own diff in git format, read without snapshotting, which is what a
// change's first version shows since there is nothing before it to compare against.
func (c *Client) CommitDiff(ctx context.Context, commit string) (string, error) {
	return c.executeJJ(ctx, "diff", "--ignore-working-copy", "-r", commit, "--git")
}

// VerbOf is the word an operation's description starts with, which is the jj command that made it
// ("rebase", "describe", "squash"), with the snapshot jj takes on its own named as one.
func VerbOf(operation string) string {
	if strings.HasPrefix(operation, "snapshot working copy") {
		return "snapshot"
	}

	word, _, _ := strings.Cut(strings.TrimSpace(operation), " ")
	word = strings.TrimRight(word, ":,.")

	if word == "" {
		return "unknown"
	}

	if _, err := strconv.Atoi(word); err == nil {
		return "unknown"
	}

	return word
}
