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

// logTemplate renders one line per revision with change ID, bookmarks, a flags string, author email,
// age, commit ID, and the description's first line, leaving the graph itself to jj since this runs
// without --no-graph. It carries no diff stats: computing them is about ten times the cost of the
// whole graph, so Stats reads them afterwards for the rows on screen.
const logTemplate = `change_id.shortest(8) ++ "` + fieldSep + `" ++ ` +
	`bookmarks.join(",") ++ "` + fieldSep + `" ++ ` +
	`local_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
	`remote_bookmarks.map(|b| b.name()).join(",") ++ "` + fieldSep + `" ++ ` +
	`(if(immutable,"I","") ++ if(conflict,"C","") ++ if(divergent,"D","") ++ ` +
	`if(empty,"E","") ++ if(current_working_copy,"@","")) ++ "` + fieldSep + `" ++ ` +
	`author.email() ++ "` + fieldSep + `" ++ ` +
	`committer.timestamp().ago() ++ "` + fieldSep + `" ++ ` +
	`commit_id.short(12) ++ "` + fieldSep + `" ++ ` +
	`description.first_line() ++ "\n"`

// statsTemplate is the expensive part of a row, read only for the commits a caller names.
const statsTemplate = `commit_id.short(12) ++ "` + fieldSep + `" ++ ` +
	`diff.stat().total_added() ++ "` + fieldSep + `" ++ ` +
	`diff.stat().total_removed() ++ "` + fieldSep + `" ++ ` +
	`diff.files().len() ++ "\n"`

const statsFieldCount = 4

// Stats is what a revision changes, the size column of its row.
type Stats struct {
	Added   int
	Removed int
	Files   int
}

// Stats reads the diff stats of the commits named, keyed by the commit IDs the log reported. It runs
// against the operation the log was read at without snapshotting, so it never contends with a write.
func (c *Client) Stats(ctx context.Context, commitIDs []string) (map[string]Stats, error) {
	if len(commitIDs) == 0 {
		return map[string]Stats{}, nil
	}

	out, err := c.executeJJ(ctx, "log", "--no-graph", "--ignore-working-copy",
		"-r", strings.Join(commitIDs, "|"), "-T", statsTemplate)
	if err != nil {
		return nil, err
	}

	return parseStats(out), nil
}

func parseStats(out string) map[string]Stats {
	stats := map[string]Stats{}

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, fieldSep)
		if len(fields) != statsFieldCount {
			continue
		}

		stats[fields[0]] = Stats{
			Added:   atoiOrZero(fields[1]),
			Removed: atoiOrZero(fields[2]),
			Files:   atoiOrZero(fields[3]),
		}
	}

	return stats
}

// changeIDSuffix matches the change ID glued onto the end of the graph prefix jj draws, since jj's
// change ID alphabet (lowercase k-z) never overlaps with the symbols and spaces that make up a graph.
var changeIDSuffix = regexp.MustCompile(`[a-z]+$`)

const logFieldCount = 9

// Log runs `jj log` over revset and returns each rendered line as a Revision or a graph-only
// connector. An empty revset lets jj apply its own configured default instead of an explicit one.
// A non-positive limit means no limit.
func (c *Client) Log(ctx context.Context, revset string, limit int) ([]LogRow, error) {
	out, err := c.executeJJ(ctx, logArgs(revset, limit)...)
	if err != nil {
		return nil, err
	}

	return parseLog(out), nil
}

// LogAt is Log as the repository stood at operation op. It reads without snapshotting the working
// copy and writes no operation, so looking at the past cannot change the present.
func (c *Client) LogAt(ctx context.Context, op, revset string, limit int) ([]LogRow, error) {
	args := append([]string{"--at-op", op, "--ignore-working-copy"}, logArgs(revset, limit)...)

	out, err := c.executeJJ(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseLog(out), nil
}

func logArgs(revset string, limit int) []string {
	args := []string{"log", "-T", logTemplate}
	if revset != "" {
		args = append(args, "-r", revset)
	}

	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	return args
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
	flags := fields[4]

	return &Revision{
		ChangeID:        changeID,
		CommitID:        fields[7],
		Description:     fields[8],
		AuthorEmail:     fields[5],
		Age:             fields[6],
		Graph:           graph,
		Bookmarks:       splitList(fields[1]),
		LocalBookmarks:  splitList(fields[2]),
		RemoteBookmarks: dedupe(splitList(fields[3])),
		Immutable:       strings.Contains(flags, "I"),
		Conflict:        strings.Contains(flags, "C"),
		Divergent:       strings.Contains(flags, "D"),
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
