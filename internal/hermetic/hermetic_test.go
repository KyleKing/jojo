package hermetic_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kyleking/jojo/internal/hermetic"
)

func TestMain(m *testing.M) { hermetic.Main(m) }

// Every process a test starts finds the isolated home, and jj's per-repository records land in it.
func TestIsolate_PointsJJAtTheTestHome(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}

	home := os.Getenv("HOME")
	if !strings.Contains(home, "jojo-test-") {
		t.Fatalf("HOME is %s, not a home of the test's own", home)
	}

	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "JJ_CONFIG", "GH_CONFIG_DIR"} {
		if !strings.HasPrefix(os.Getenv(key), home) {
			t.Errorf("%s is %s, outside %s", key, os.Getenv(key), home)
		}
	}

	if dir := t.TempDir(); !strings.HasPrefix(dir, filepath.Dir(home)) {
		t.Errorf("t.TempDir is %s, outside the run's root", dir)
	}

	dir := t.TempDir()
	run(t, dir, "git", "init", ".")
	hermetic.RepoConfig(t, dir, "[ui]\ndiff-editor = \":builtin\"")

	if got := strings.TrimSpace(run(t, dir, "config", "get", "ui.diff-editor")); got != ":builtin" {
		t.Errorf("the repository config reads %q", got)
	}

	path := strings.TrimSpace(run(t, dir, "config", "path", "--repo"))
	if !strings.HasPrefix(path, home) {
		t.Errorf("jj keeps the repository config at %s, outside %s", path, home)
	}

	if got := strings.TrimSpace(run(t, dir, "config", "get", "user.name")); got != hermetic.User {
		t.Errorf("jj commits as %q", got)
	}
}

// A guarded path changing is a leak, except a jj repository record that names no repository of
// this run's, which another program opening its own repository writes.
func TestLeaks_OnlyTheRunsOwnRecordsCount(t *testing.T) {
	t.Parallel()

	jjDir, root := filepath.Join(t.TempDir(), "jj"), "/tmp/jojo-test-123"
	write(t, filepath.Join(jjDir, "config.toml"), "[ui]\n")
	write(t, filepath.Join(jjDir, "repos", "old", "metadata.binpb"), "/home/me/project/.jj/repo")

	before := hermetic.Take(jjDir)

	write(t, filepath.Join(jjDir, "repos", "ours", "metadata.binpb"), root+"/tmp/TestX/001/.jj/repo")
	write(t, filepath.Join(jjDir, "repos", "theirs", "metadata.binpb"), "/home/me/other/.jj/repo")

	if leaks := hermetic.Leaks(before, hermetic.Take(jjDir), jjDir, root); !slices.Equal(leaks, []string{
		filepath.Join(jjDir, "repos", "ours"),
		filepath.Join(jjDir, "repos", "ours", "metadata.binpb"),
	}) {
		t.Errorf("leaks %q", leaks)
	}

	before = hermetic.Take(jjDir)
	write(t, filepath.Join(jjDir, "config.toml"), "[ui]\npager = \"less\"\n")

	if leaks := hermetic.Leaks(before, hermetic.Take(jjDir), jjDir, root); len(leaks) != 1 ||
		leaks[0] != filepath.Join(jjDir, "config.toml") {
		t.Errorf("a rewritten user config leaks %q", leaks)
	}

	missing := filepath.Join(t.TempDir(), "gh")
	before = hermetic.Take(missing)
	write(t, filepath.Join(missing, "hosts.yml"), "x")

	if leaks := hermetic.Leaks(before, hermetic.Take(missing), jjDir, root); len(leaks) != 2 {
		t.Errorf("creating a guarded directory leaks %q", leaks)
	}
}

// Every package with tests installs the guard, so a test added to a new package cannot reach the
// real home unnoticed.
func TestEveryTestPackageIsHermetic(t *testing.T) {
	t.Parallel()

	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	guarded, err := testPackages(module)
	if err != nil {
		t.Fatal(err)
	}

	for dir, ok := range guarded {
		if !ok {
			rel, _ := filepath.Rel(module, dir) //nolint:errcheck // dir is under module
			t.Errorf("%s has tests and no TestMain calling hermetic.Main", rel)
		}
	}
}

// testPackages is every directory under module holding tests, and whether one of its test files
// installs the guard.
func testPackages(module string) (map[string]bool, error) {
	guarded := map[string]bool{}

	err := filepath.WalkDir(module, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && path != module && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata"):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(path, "_test.go"):
			return nil
		}

		data, err := os.ReadFile(path) //nolint:gosec // a test file of this module
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		dir, text := filepath.Dir(path), string(data)
		guarded[dir] = guarded[dir] ||
			strings.Contains(text, "hermetic.Main(") || strings.Contains(text, "hermetic.Run(")

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", module, err)
	}

	return guarded, nil
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()

	//nolint:gosec // G204: fixture helper runs the literal jj binary with test arguments.
	cmd := exec.CommandContext(t.Context(), "jj", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}

	return string(out)
}

func write(t *testing.T, path, text string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
