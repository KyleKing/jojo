package jj

import (
	"context"
	"strconv"
	"strings"
)

// opLogTemplate renders one line per operation: its ID, description, whether it is the operation the
// repo is currently at, and its age.
const opLogTemplate = `id.short(8) ++ "` + fieldSep + `" ++ ` +
	`description ++ "` + fieldSep + `" ++ ` +
	`if(current_operation,"@","") ++ "` + fieldSep + `" ++ ` +
	`time.end().ago() ++ "\n"`

const opLogFieldCount = 4

// OpLog runs `jj op log` and returns each operation newest first. A non-positive limit means no limit.
func (c *Client) OpLog(ctx context.Context, limit int) ([]Operation, error) {
	args := []string{"op", "log", "--no-graph", "--color=never", "--no-pager", "-T", opLogTemplate}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	out, err := c.executeJJ(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseOpLog(out), nil
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
			Description: fields[1],
			Current:     fields[2] == "@",
			Age:         fields[3],
		})
	}

	return ops
}
