package jj

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// fieldSep is the field separator for logTemplate. It is US (unit separator), a byte that never
// appears in jj's own output, so a line splits unambiguously into fields.
const fieldSep = "\x1f"

// logTemplate renders one line per revision with change ID, commit ID, bookmarks, a flags string,
// author email, age, added and removed line counts, file count, and the description's first line,
// leaving the graph itself to jj since this runs without --no-graph.
const logTemplate = `change_id.shortest(8) ++ "` + fieldSep + `" ++ ` +
	`commit_id.short(8) ++ "` + fieldSep + `" ++ ` +
	`bookmarks.join(",") ++ "` + fieldSep + `" ++ ` +
	`local_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
	`remote_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
	`(if(immutable,"I","") ++ if(conflict,"C","") ++ ` +
	`if(empty,"E","") ++ if(current_working_copy,"@","")) ++ "` + fieldSep + `" ++ ` +
	`author.email() ++ "` + fieldSep + `" ++ ` +
	`committer.timestamp().ago() ++ "` + fieldSep + `" ++ ` +
	`diff.stat().total_added() ++ "` + fieldSep + `" ++ ` +
	`diff.stat().total_removed() ++ "` + fieldSep + `" ++ ` +
	`diff.files().len() ++ "` + fieldSep + `" ++ ` +
	`description.first_line() ++ "\n"`

// changeIDSuffix matches the change ID glued onto the end of the graph prefix jj draws, since jj's
// change ID alphabet (lowercase k-z) never overlaps with the symbols and spaces that make up a graph.
var changeIDSuffix = regexp.MustCompile(`[a-z]+$`)

const logFieldCount = 12

// Log runs `jj log` over revset and returns each rendered line as a Revision or a graph-only
// connector. An empty revset lets jj apply its own configured default instead of an explicit one.
// A non-positive limit means no limit.
func (c *Client) Log(ctx context.Context, revset string, limit int) ([]LogRow, error) {
	args := []string{"log", "--color=never", "--no-pager", "-T", logTemplate}
	if revset != "" {
		args = append(args, "-r", revset)
	}

	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	out, err := c.executeJJ(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseLog(out), nil
}

func parseLog(out string) []LogRow {
	lines := strings.Split(out, "\n")

	rows := make([]LogRow, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		if !strings.Contains(line, fieldSep) {
			rows = append(rows, LogRow{Graph: &GraphLine{Graph: line}})

			continue
		}

		if rev := parseRevisionLine(line); rev != nil {
			rows = append(rows, LogRow{Revision: rev})
		}
	}

	return rows
}

func parseRevisionLine(line string) *Revision {
	fields := strings.Split(line, fieldSep)
	if len(fields) != logFieldCount {
		return nil
	}

	loc := changeIDSuffix.FindStringIndex(fields[0])
	if loc == nil {
		return nil
	}

	graph, changeID := fields[0][:loc[0]], fields[0][loc[0]:]
	flags := fields[5]

	return &Revision{
		ChangeID:        changeID,
		CommitID:        fields[1],
		Description:     fields[11],
		AuthorEmail:     fields[6],
		Age:             fields[7],
		Graph:           graph,
		Bookmarks:       splitList(fields[2]),
		LocalBookmarks:  splitList(fields[3]),
		RemoteBookmarks: dedupe(splitList(fields[4])),
		Added:           atoiOrZero(fields[8]),
		Removed:         atoiOrZero(fields[9]),
		Files:           atoiOrZero(fields[10]),
		Immutable:       strings.Contains(flags, "I"),
		Conflict:        strings.Contains(flags, "C"),
		Empty:           strings.Contains(flags, "E"),
		WorkingCopy:     strings.Contains(flags, "@"),
	}
}

func splitList(field string) []string {
	if field == "" {
		return nil
	}

	return strings.Split(field, ",")
}

// dedupe collapses one branch tracked at several remotes to a single name.
func dedupe(names []string) []string {
	seen := map[string]bool{}
	out := names[:0]

	for _, name := range names {
		if seen[name] {
			continue
		}

		seen[name] = true
		out = append(out, name)
	}

	return out
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}

	return n
}
