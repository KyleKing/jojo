package main_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeJJDiff puts a jj-diff ahead of PATH that reports version, records the arguments it was run
// with in the returned file, and keeps v0.15.2's exit contract: with code 0 it writes one jj
// operation (a describe of the revision) when write is set and says so on stderr in a "jj-diff: "
// line; a non-zero code means it could not run, and stderr says why. It draws nothing, so the test
// pins jojo's half: the suspend, the launch, the resume, and what the footer says after.
func fakeJJDiff(t *testing.T, env []string, version string, code int, write bool) ([]string, string) {
	t.Helper()

	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = --version ]; then\n" +
		"  echo 'jj-diff " + version + " (commit: none, built: unknown)'; exit 0\n" +
		"fi\n" +
		"echo \"$@\" > '" + record + "'\n" +
		"if [ " + strconv.Itoa(code) + " != 0 ]; then echo \"jj-diff: no revision $3\" >&2; exit " +
		strconv.Itoa(code) + "; fi\n"

	if write {
		script += "jj describe -r \"$3\" -m 'split by the fake'\n" +
			"echo \"jj-diff: split $3 into 2 commits · jj split -r $3 -B $3 · jj undo undoes it\" >&2\n"
	}

	script += "exit 0\n"

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
// says what jj-diff did, in its own outcome line, or that it wrote nothing, or why it could not run.
func TestJJDiff_SplitLaunchesAndReportsTheExit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		want  string
		code  int
		write bool
	}{
		{name: "wrote", code: 0, write: true, want: "into 2 commits · jj split -r"},
		{name: "closed without writing", code: 0, want: "jj-diff --split -r"},
		{name: "could not run", code: 2, want: "could not run: no revision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env, record := fakeJJDiff(t, isolated(t), "0.15.2", tc.code, tc.write)
			dir := historyRepo(t, env, 3)

			sc := open(t, size{120, 24}, dir, env)
			sc.awaitFrame("the log", func(f []string) bool {
				return strings.Contains(strings.Join(f, "\n"), "commit number 1")
			})

			sc.press("S")
			sc.awaitFrame("the outcome", func(f []string) bool {
				last := f[len(f)-1]

				return strings.Contains(last, tc.want) && (tc.write || strings.Contains(last, "nothing written") ||
					strings.Contains(last, "could not run"))
			})

			args, err := os.ReadFile(record) //nolint:gosec // G304: the fixture's own file.
			if err != nil {
				t.Fatalf("jj-diff never ran: %v", err)
			}

			if fields := strings.Fields(string(args)); len(fields) != 3 || fields[0] != "--split" || fields[1] != "-r" {
				t.Errorf("jj-diff ran with %q, want --split -r <rev>", args)
			}

			// The log is read after jj-diff exits, so what it wrote is on screen when the terminal returns.
			if tc.write {
				sc.awaitFrame("the log jj-diff left", func(f []string) bool {
					return strings.Contains(strings.Join(f, "\n"), "split by the fake")
				})
			}
			sc.quit()
		})
	}
}
