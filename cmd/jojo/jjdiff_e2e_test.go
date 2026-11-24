package main_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeJJDiff puts a jj-diff ahead of PATH that reports version, records the arguments it was run
// with in the returned file, and exits with code, writing one jj operation (a describe of the
// revision) when code is 0. It draws nothing, so the test pins jojo's half: the suspend, the launch,
// the resume, and what the footer says after.
func fakeJJDiff(t *testing.T, env []string, version string, code int) ([]string, string) {
	t.Helper()

	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = --version ]; then\n" +
		"  echo 'jj-diff " + version + " (commit: none, built: unknown)'; exit 0\n" +
		"fi\n" +
		"echo \"$@\" > '" + record + "'\n" +
		"[ " + strconv.Itoa(code) + " = 0 ] && jj describe -r \"$3\" -m 'split by the fake'\n" +
		"exit " + strconv.Itoa(code) + "\n"

	//nolint:gosec // G306: the fixture is a program, so it must be executable.
	if err := os.WriteFile(filepath.Join(dir, "jj-diff"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	out := make([]string, 0, len(env))

	for _, kv := range env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			kv = "PATH=" + dir + string(os.PathListSeparator) + path
		}

		out = append(out, kv)
	}

	return out, record
}

// S on a revision suspends jojo, runs jj-diff --split -r on it, and on return reads the log again and
// says whether jj-diff wrote, in the command's own words.
func TestJJDiff_SplitLaunchesAndReportsTheExit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		want string
		code int
	}{
		{name: "wrote", code: 0, want: "wrote"},
		{name: "quit without writing", code: 1, want: "exited 1, nothing written"},
		{name: "any other exit", code: 2, want: "exited 2, nothing written"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env, record := fakeJJDiff(t, isolated(t), "0.10.0", tc.code)
			dir := historyRepo(t, env, 3)

			sc := open(t, size{120, 24}, dir, env)
			sc.awaitFrame("the log", func(f []string) bool {
				return strings.Contains(strings.Join(f, "\n"), "commit number 1")
			})

			sc.press("S")
			sc.awaitFrame("the outcome", func(f []string) bool {
				last := f[len(f)-1]

				return strings.Contains(last, "jj-diff --split -r") && strings.Contains(last, tc.want)
			})

			args, err := os.ReadFile(record) //nolint:gosec // G304: the fixture's own file.
			if err != nil {
				t.Fatalf("jj-diff never ran: %v", err)
			}

			if fields := strings.Fields(string(args)); len(fields) != 3 || fields[0] != "--split" || fields[1] != "-r" {
				t.Errorf("jj-diff ran with %q, want --split -r <rev>", args)
			}

			// The log is read after jj-diff exits, so what it wrote is on screen when the terminal returns.
			if tc.code == 0 {
				sc.awaitFrame("the log jj-diff left", func(f []string) bool {
					return strings.Contains(strings.Join(f, "\n"), "split by the fake")
				})
			}
			sc.quit()
		})
	}
}
