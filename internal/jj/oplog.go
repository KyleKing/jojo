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
			Args:        shortenIDs(readableArgs(strings.TrimPrefix(fields[4], "args: "))),
		})
	}

	return ops
}

// OpRevert applies the inverse of one operation, leaving everything done after it in place. That is
// what separates it from OpRestore, which throws away every later operation.
func (c *Client) OpRevert(ctx context.Context, opID string) (string, error) {
	return c.mutate(ctx, "op", "revert", opID)
}

// Operation descriptions jj writes, which UndoTarget and Refusal read.
const (
	undoPrefix   = "undo: "
	snapshotDesc = "snapshot working copy"
	pushPrefix   = "push "
)

// undoWindow is how far back UndoTarget can see: a run of undos longer than this reads as nothing
// left to name.
const undoWindow = 32

// UndoTarget is the operation `jj undo` would reverse next, given ops newest first. Since jj 0.33
// undo is sequential: each undo at the head steps one operation further back, so k undos in a row
// mean the next one reverses the (k+1)th operation beneath them that is not itself an undo.
func UndoTarget(ops []Operation) (Operation, bool) {
	undos := 0
	for undos < len(ops) && strings.HasPrefix(ops[undos].Description, undoPrefix) {
		undos++
	}

	skip := undos
	for _, op := range ops[undos:] {
		if strings.HasPrefix(op.Description, undoPrefix) {
			continue
		}

		if skip == 0 {
			return op, true
		}

		skip--
	}

	return Operation{}, false
}

// Refusal says why u must not reverse op, "" when it may. Reversing a snapshot puts the working copy
// back as it stood before jj recorded the edits on disk, which deletes them from disk; reversing a
// push moves the local bookmarks back while the remote keeps what was pushed.
func (op Operation) Refusal() string {
	switch {
	case op.Description == snapshotDesc:
		return "the last operation is jj recording your edits on disk, and undoing it would delete them; " +
			"the op log (o) reverses an earlier one"
	case strings.HasPrefix(op.Description, pushPrefix):
		return "the last operation pushed to a remote, which u cannot take back"
	}

	return ""
}

// UndoTargetNow snapshots the working copy, as jj undo itself would, and names what undo would then
// reverse.
func (c *Client) UndoTargetNow(ctx context.Context) (Operation, bool, error) {
	if err := c.Snapshot(ctx); err != nil {
		return Operation{}, false, err
	}

	ops, err := c.OpLog(ctx, undoWindow)
	if err != nil {
		return Operation{}, false, err
	}

	op, ok := UndoTarget(ops)

	return op, ok, nil
}

// HeadUndo names what u would reverse without snapshotting, for the footer: the operation and
// whether u may reverse it.
func (c *Client) HeadUndo(ctx context.Context) (Operation, bool, error) {
	ops, err := c.OpLog(ctx, undoWindow)
	if err != nil {
		return Operation{}, false, err
	}

	op, ok := UndoTarget(ops)

	return op, ok && op.Refusal() == "", nil
}

// plumbing is what jojo adds to every jj call to parse its output, which says nothing about what the
// call did: an op log row reads as the command a person would have typed without them.
//
//nolint:gochecknoglobals // a fixed table, read only.
var plumbing = map[string]bool{
	noPager: true, colorNever: true, ignoreWorkingCopy: true, "--quiet": true,
}

// withValue are flags whose value is plumbing too: a template is jojo's parsing format.
//
//nolint:gochecknoglobals // a fixed table, read only.
var withValue = map[string]bool{"-T": true, "--template": true}

// readableArgs is an operation's command line without jojo's plumbing flags, kept in the shell
// quoting jj recorded it in.
func readableArgs(args string) string {
	words := shellWords(args)
	out := make([]string, 0, len(words))

	for i := 0; i < len(words); i++ {
		bare := strings.Trim(words[i], "'")

		switch {
		case plumbing[bare]:
			continue
		case withValue[bare]:
			i++

			continue
		case strings.HasPrefix(bare, "--template="):
			continue
		}

		out = append(out, words[i])
	}

	return strings.Join(out, " ")
}

// shellWords splits a command line jj recorded on the spaces outside single quotes, keeping each
// word's quotes, which is the only quoting jj writes.
func shellWords(line string) []string {
	var (
		words  []string
		word   strings.Builder
		quoted bool
	)

	for _, r := range line {
		switch {
		case r == '\'':
			quoted = !quoted
		case r == ' ' && !quoted:
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}

			continue
		}

		word.WriteRune(r)
	}

	if word.Len() > 0 {
		words = append(words, word.String())
	}

	return words
}
