// Package jjdiff finds the jj-diff on PATH and says whether it has the modes jojo launches from the
// log: --split (one column per commit the split makes) and --resolve (the conflict viewer). Both
// landed in v0.10.0; v0.15.2 settled how they report back (see Outcome), which jojo now relies on.
package jjdiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Binary is the program jojo launches.
const Binary = "jj-diff"

// MinVersion is the first release whose --split and --resolve keep the exit contract Outcome reads:
// exit 0 whether or not they wrote, non-zero only when they could not run, and the outcome of a
// write as the last stderr line starting with OutcomePrefix.
const MinVersion = "0.15.2"

// OutcomePrefix starts the stderr line jj-diff ends a write with.
const OutcomePrefix = "jj-diff: "

// Mode is one of jj-diff's screens jojo opens on a revision.
type Mode string

// The modes jojo launches.
const (
	Split   Mode = "--split"
	Resolve Mode = "--resolve"
)

// Tool is what the probe found: where jj-diff is, the version it reports, and whether it has the
// modes. Why says what is missing when it has not, in words the footer can show.
type Tool struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Why     string `json:"why"`
	Modes   bool   `json:"modes"`
}

// Runner runs a binary and returns what it printed, so a test can stand in for jj-diff.
type Runner func(ctx context.Context, path string, args ...string) ([]byte, error)

// LookPath finds a binary on PATH, as exec.LookPath does.
type LookPath func(string) (string, error)

// Exec runs path with args and returns what it printed to either stream, since the flag package
// prints -h to stderr.
func Exec(ctx context.Context, path string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput() //nolint:gosec // G204: jj-diff, found on PATH
	if err != nil {
		return out, fmt.Errorf("running %s %s: %w", path, strings.Join(args, " "), err)
	}

	return out, nil
}

// Probe finds jj-diff and checks it can split and resolve. A release says its version and is held
// to MinVersion; a build from source says "dev", so it is asked for its flags instead.
func Probe(ctx context.Context, find LookPath, run Runner) Tool {
	path, err := find(Binary)
	if err != nil {
		return Tool{Why: Binary + " is not on PATH; install v" + MinVersion + " or newer"}
	}

	tool := Tool{Path: path}

	out, err := run(ctx, path, "--version")
	if err != nil {
		tool.Why = Binary + " --version failed: " + firstLine(err.Error())

		return tool
	}

	tool.Version = versionOf(out)

	if older, known := Older(tool.Version, MinVersion); known {
		tool.Modes = !older
		if older {
			tool.Why = Binary + " " + tool.Version + " is older than v" + MinVersion
		}

		return tool
	}

	help, _ := run(ctx, path, "-h") //nolint:errcheck // -h exits non-zero by the flag package's design.
	tool.Modes = bytes.Contains(help, []byte("-split")) && bytes.Contains(help, []byte("-resolve"))

	if !tool.Modes {
		tool.Why = Binary + " " + tool.Version + " has no --split or --resolve; install v" + MinVersion + " or newer"
	}

	return tool
}

// versionOf reads "jj-diff 0.10.0 (commit: …)" as "0.10.0".
func versionOf(out []byte) string {
	fields := strings.Fields(string(out))
	if len(fields) < 2 { //nolint:mnd // the program's name, then its version
		return ""
	}

	return strings.TrimPrefix(fields[1], "v")
}

// errNotSemver is a version that is not three dot-separated numbers.
var errNotSemver = errors.New("not a release version")

// Older reports whether version is older than floor, and whether that could be told at all: a
// "dev" build or anything not major.minor.patch cannot be.
func Older(version, floor string) (bool, bool) {
	a, err := parse(version)
	if err != nil {
		return false, false
	}

	b, err := parse(floor)
	if err != nil {
		return false, false
	}

	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i], true
		}
	}

	return false, true
}

func parse(version string) ([3]int, error) {
	var out [3]int

	core, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "-")

	parts := strings.Split(core, ".")
	if len(parts) != len(out) {
		return out, errNotSemver
	}

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, errNotSemver
		}

		out[i] = n
	}

	return out, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")

	return line
}

// Args is the argument list that opens mode on rev.
func Args(mode Mode, rev string) []string { return []string{string(mode), "-r", rev} }

// Outcome is what the footer says once jj-diff has given the terminal back, since the repaint covers
// whatever it printed. Whether it wrote is whether jj's operation log moved while it ran, which holds
// whatever jj-diff says. After a write jj-diff's own last "jj-diff: " line on stderr says what it
// did (split X into 2 commits · the jj command · how to undo), which beats a generic "wrote"; its
// "jj undo undoes it" becomes "u undoes it", jojo's key for the same thing. A non-zero exit means it
// could not run, and stderr's last line says why.
func Outcome(mode Mode, rev string, err error, wrote bool, stderr string) string {
	command := Binary + " " + strings.Join(Args(mode, rev), " ")
	said := lastOutcome(stderr)

	var exit *exec.ExitError

	switch {
	case err == nil && wrote && said != "":
		return strings.Replace(said, "jj undo undoes it", "u undoes it", 1)
	case err == nil && wrote:
		return command + " wrote " + rev + ", press u to undo"
	case err == nil:
		return command + " closed, nothing written"
	case errors.As(err, &exit):
		why := lastLine(stderr)
		if why == "" {
			why = "exited " + strconv.Itoa(exit.ExitCode())
		}

		return command + " could not run: " + why
	}

	return command + " did not run: " + firstLine(err.Error())
}

// lastOutcome is the last stderr line that starts with OutcomePrefix, without it.
func lastOutcome(stderr string) string {
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if said, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), OutcomePrefix); ok {
			return said
		}
	}

	return ""
}

// lastLine is stderr's last non-blank line, its OutcomePrefix dropped.
func lastLine(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])

	return strings.TrimPrefix(line, OutcomePrefix)
}
