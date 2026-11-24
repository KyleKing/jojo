package jjdiff_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/jjdiff"
)

var errMissing = errors.New("not found")

func found(string) (string, error)   { return "/bin/jj-diff", nil }
func missing(string) (string, error) { return "", errMissing }

// runner answers --version with version and -h with help, as a jj-diff build would.
func runner(version, help string) jjdiff.Runner {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("jj-diff " + version + " (commit: abc, built: today)\n"), nil
		}

		return []byte(help), nil
	}
}

func TestProbe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		find    jjdiff.LookPath
		run     jjdiff.Runner
		name    string
		why     string
		version string
		modes   bool
	}{
		{name: "not installed", find: missing, run: runner("", ""), why: "not on PATH"},
		{
			name: "the first release with the modes", find: found, run: runner("0.10.0", ""), modes: true,
			version: "0.10.0",
		},
		{name: "a later one", find: found, run: runner("v1.2.3", ""), modes: true, version: "1.2.3"},
		{name: "too old", find: found, run: runner("0.9.0", ""), why: "older than v0.10.0", version: "0.9.0"},
		{
			name: "a dev build that has them", find: found, modes: true, version: "dev",
			run: runner("dev", "  -resolve\n  -split\n"),
		},
		{
			name: "a dev build that has not", find: found, why: "has no --split or --resolve", version: "dev",
			run: runner("dev", "  -i\n"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := jjdiff.Probe(t.Context(), tc.find, tc.run)
			if got.Modes != tc.modes || got.Version != tc.version || !strings.Contains(got.Why, tc.why) {
				t.Errorf("Probe = %+v, want modes %v, version %q, why containing %q", got, tc.modes, tc.version, tc.why)
			}

			if tc.modes && got.Why != "" {
				t.Errorf("a usable jj-diff came with a reason: %q", got.Why)
			}
		})
	}
}

func TestOlder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		version     string
		older, know bool
	}{
		{"0.9.9", true, true},
		{"0.10.0", false, true},
		{"0.10.1", false, true},
		{"1.0.0", false, true},
		{"v0.11.0-rc1", false, true},
		{"dev", false, false},
		{"0.10", false, false},
	} {
		if older, know := jjdiff.Older(tc.version, jjdiff.MinVersion); older != tc.older || know != tc.know {
			t.Errorf("Older(%q) = %v, %v; want %v, %v", tc.version, older, know, tc.older, tc.know)
		}
	}
}

// The footer names the command and whether it wrote, since the repaint covers what jj-diff printed.
func TestOutcome(t *testing.T) {
	t.Parallel()

	quit := exec.CommandContext(t.Context(), "sh", "-c", "exit 1").Run()

	for _, tc := range []struct {
		err   error
		want  string
		wrote bool
	}{
		{nil, "jj-diff --resolve -r abc wrote abc, press u to undo", true},
		{nil, "jj-diff --resolve -r abc closed, nothing written", false},
		{quit, "jj-diff --resolve -r abc exited 1, nothing written", false},
		{errMissing, "jj-diff --resolve -r abc did not run: not found", false},
	} {
		if got := jjdiff.Outcome(jjdiff.Resolve, "abc", tc.err, tc.wrote); got != tc.want {
			t.Errorf("Outcome(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
