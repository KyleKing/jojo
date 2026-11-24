package jj

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// opLogTemplate renders one line per operation: its ID, description, whether it is the operation the
// repo is currently at, its age, and the command line that made it. Version 0.47 of jj renamed tags
// to attributes; tags still answers there with a deprecation warning on stderr, and it is the only
// one older releases know.
const opLogTemplate = `id.short(8) ++ "` + fieldSep + `" ++ ` +
	`description ++ "` + fieldSep + `" ++ ` +
	`if(current_operation,"@","") ++ "` + fieldSep + `" ++ ` +
	`time.end().ago() ++ "` + fieldSep + `" ++ ` +
	`tags.lines().join(" ") ++ "\n"`

const opLogFieldCount = 5

// OpLog runs `jj op log` and returns each operation newest first. A non-positive limit means no limit.
func (c *Client) OpLog(ctx context.Context, limit int) ([]Operation, error) {
	args := []string{"op", "log", "--no-graph", "-T", opLogTemplate}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	out, err := c.executeJJ(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseOpLog(out), nil
}

// longHex matches the full commit and change ids jj writes into its operation descriptions. A row is
// scanned, and forty characters of hex is the part of it nobody reads.
var longHex = regexp.MustCompile(`\b[0-9a-f]{12,}\b`)

// shortIDLen matches the length the log rows use, so the same revision reads the same in both views.
const shortIDLen = 8

func shortenIDs(description string) string {
	return longHex.ReplaceAllStringFunc(description, func(id string) string {
		return id[:shortIDLen]
	})
}

func parseOpLog(out string) []Operation {
	lines := strings.Split(out, "\n")

	ops := make([]Operation, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, fieldSep)
		if len(fields) != opLogFieldCount {
			continue
		}

		ops = append(ops, Operation{
			ID:          fields[0],
			Description: shortenIDs(fields[1]),
			Current:     fields[2] == "@",
			Age:         fields[3],
			Args:        shortenIDs(strings.TrimPrefix(fields[4], "args: ")),
		})
	}

	return ops
}

// OpRevert applies the inverse of one operation, leaving everything done after it in place. That is
// what separates it from OpRestore, which throws away every later operation.
func (c *Client) OpRevert(ctx context.Context, opID string) (string, error) {
	return c.mutate(ctx, "op", "revert", opID)
}
