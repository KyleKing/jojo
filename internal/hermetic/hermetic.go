// Package hermetic keeps a test binary off the account that runs it. Every process a test starts
// (jj, git, gh, jojo itself) and every path jojo reads through the environment lands in a home the
// binary made for itself, and the run fails when the real one changed anyway.
//
// jj 0.47 keeps a repository's config, and a record of every repository it opened, under
// $XDG_CONFIG_HOME/jj/repos rather than in the repository, so a fixture's `jj config set --repo`, or a
// .jj/repo/config.toml jj migrates on its next call, writes into whatever home the test inherited.
// Isolation has to come from the environment, which is what Run sets before any test starts.
package hermetic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// User is the name and email every jj and git call in a test commits as.
const (
	User  = "Test User"
	Email = "test@example.com"
)

const (
	dirMode  = 0o750
	fileMode = 0o600
)

// Runner is what Run drives: *testing.M.
type Runner interface{ Run() int }

// Main is a package's TestMain: it runs the tests in a home of their own and exits with their code,
// or with a failure when one of them reached the real home.
func Main(m Runner) { os.Exit(Run(m)) }

// Run isolates the process, runs m, and answers its exit code, which it turns into a failure when a
// guarded path of the real home changed and the change is this run's.
func Run(m Runner) int {
	owner, err := RealPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)

		return 1
	}

	root, err := os.MkdirTemp("", "jojo-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)

		return 1
	}

	defer os.RemoveAll(root) //nolint:errcheck // a temp dir left behind is harmless

	before := Take(owner.Guarded()...)

	if err := Isolate(root, owner); err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)

		return 1
	}

	code := m.Run()

	if leaks := Leaks(before, Take(owner.Guarded()...), owner.JJ, root); len(leaks) > 0 {
		fmt.Fprintf(os.Stderr, "hermetic: the tests changed the real home, which they must never touch:\n  %s\n",
			strings.Join(leaks, "\n  "))

		return 1
	}

	return code
}

// Paths are the real account's directories a test could reach through the environment.
type Paths struct {
	Home   string
	Config string // $XDG_CONFIG_HOME, or ~/.config
	State  string // $XDG_STATE_HOME, or ~/.local/state
	Cache  string // os.UserCacheDir
	JJ     string // jj's user config directory, which holds repos/
	GH     string // gh's config directory
}

// RealPaths reads the real account's directories from the environment as it is before Isolate.
func RealPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("finding the home directory: %w", err)
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("finding the cache directory: %w", err)
	}

	p := Paths{
		Home: home, Cache: cache,
		Config: envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config")),
		State:  envOr("XDG_STATE_HOME", filepath.Join(home, ".local", "state")),
	}
	p.JJ = filepath.Join(p.Config, "jj")
	p.GH = envOr("GH_CONFIG_DIR", filepath.Join(p.Config, "gh"))

	return p, nil
}

// Guarded is every real path a test run must leave as it found it: jj's config and its per-repository
// records, git's global config, gh's config, and jojo's own config, state, and cache.
func (p Paths) Guarded() []string {
	return []string{
		p.JJ,
		filepath.Join(p.Home, ".gitconfig"), filepath.Join(p.Config, "git"),
		p.GH,
		filepath.Join(p.Config, "jojo"), filepath.Join(p.State, "jojo"), filepath.Join(p.Cache, "jojo"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// Isolate points the process at a home under root: HOME and every XDG directory, jj's and git's user
// config, gh's config, and the temp directory, so every t.TempDir and every repository a test makes
// is under root too. Go's own caches stay where owner's are, since a cold build cache would rebuild
// the standard library for every binary that builds jojo.
func Isolate(root string, owner Paths) error {
	home := filepath.Join(root, "home")
	tmp := filepath.Join(root, "tmp")
	config := filepath.Join(home, ".config")

	for _, dir := range []string{tmp, filepath.Join(config, "jj")} {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return fmt.Errorf("making %s: %w", dir, err)
		}
	}

	env := []struct{ key, value string }{
		{"GOCACHE", envOr("GOCACHE", filepath.Join(owner.Cache, "go-build"))},
		{"GOPATH", envOr("GOPATH", filepath.Join(owner.Home, "go"))},
		{"GOENV", envOr("GOENV", filepath.Join(owner.Config, "go", "env"))},
		{"HOME", home},
		{"XDG_CONFIG_HOME", config},
		{"XDG_DATA_HOME", filepath.Join(home, ".local", "share")},
		{"XDG_STATE_HOME", filepath.Join(home, ".local", "state")},
		{"XDG_CACHE_HOME", filepath.Join(home, ".cache")},
		{"TMPDIR", tmp},
		{"JJ_CONFIG", filepath.Join(config, "jj", "config.toml")},
		{"JJ_USER", User},
		{"JJ_EMAIL", Email},
		{"GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig")},
		{"GIT_CONFIG_NOSYSTEM", "1"},
		{"GIT_AUTHOR_NAME", User},
		{"GIT_AUTHOR_EMAIL", Email},
		{"GIT_COMMITTER_NAME", User},
		{"GIT_COMMITTER_EMAIL", Email},
		{"GH_CONFIG_DIR", filepath.Join(config, "gh")},
	}

	for _, e := range env {
		if err := os.Setenv(e.key, e.value); err != nil {
			return fmt.Errorf("setting %s: %w", e.key, err)
		}
	}

	return nil
}

// stamp is what a snapshot keeps of one path: enough to tell it was rewritten.
type stamp struct {
	mod  time.Time
	size int64
	mode fs.FileMode
}

// Snapshot is every file and directory under some paths, by path.
type Snapshot map[string]stamp

// Take snapshots paths, each a file or a directory walked whole. A missing path is recorded as
// missing, so creating it reads as a change.
func Take(paths ...string) Snapshot {
	s := Snapshot{}

	for _, root := range paths {
		// The walk never fails: an unreadable entry is skipped and the rest recorded.
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error { //nolint:errcheck // see above
			if err != nil {
				return nil //nolint:nilerr // a missing or unreadable path is simply not recorded
			}

			info, err := d.Info()
			if err != nil {
				return nil //nolint:nilerr // vanished between the listing and the stat
			}

			s[path] = stamp{mod: info.ModTime(), size: info.Size(), mode: info.Mode()}

			return nil
		})
	}

	return s
}

// Changed lists every path added, removed, or rewritten between s and after, sorted.
func (s Snapshot) Changed(after Snapshot) []string {
	var out []string

	for path, was := range s {
		if now, ok := after[path]; !ok || now != was {
			out = append(out, path)
		}
	}

	for path := range after {
		if _, ok := s[path]; !ok {
			out = append(out, path)
		}
	}

	slices.Sort(out)

	return out
}

// Leaks is what changed between before and after that this run did. Under jj's repos/ an entry is
// the run's only when one of its files names root, since jj records each repository by its path
// and every repository a test makes is under root; another program on the machine opening its own
// repository meanwhile is not a leak. A change anywhere else is.
func Leaks(before, after Snapshot, jjDir, root string) []string {
	repos := filepath.Join(jjDir, "repos")
	ours := map[string]bool{}

	var out []string

	for _, path := range before.Changed(after) {
		rel, err := filepath.Rel(repos, path)

		switch {
		case err != nil || strings.HasPrefix(rel, ".."):
			out = append(out, path)

			continue
		case rel == ".":
			// The directory itself changes with any entry added under it; the entry decides.
			continue
		}

		entry := filepath.Join(repos, strings.Split(rel, string(filepath.Separator))[0])
		if _, seen := ours[entry]; !seen {
			ours[entry] = names(entry, root)
		}

		if ours[entry] {
			out = append(out, path)
		}
	}

	return out
}

// names reports whether any file in dir holds root's path.
func names(dir, root string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // jj's own record of a repository
		if err == nil && bytes.Contains(data, []byte(root)) {
			return true
		}
	}

	return false
}

// ErrOutsideHome marks a repository config jj would keep outside the isolated home.
var ErrOutsideHome = errors.New("jj keeps this repository's config outside the test's home")

// RepoConfig sets the config of the jj repository at dir to toml, replacing what it held, by writing
// the file jj reads it from rather than going through `jj config set --repo`. Since jj 0.47 that
// file is under $XDG_CONFIG_HOME/jj/repos, so it refuses a path outside the run's root: a test that did not
// go through Main would otherwise write into the real home. A test may point XDG_CONFIG_HOME at a
// directory of its own under the root.
func RepoConfig(tb testing.TB, dir, toml string) {
	tb.Helper()

	path, err := repoConfigPath(tb.Context(), dir)
	if err != nil {
		tb.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		tb.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, fileMode) //nolint:gosec // jj named it
	if err != nil {
		tb.Fatal(err)
	}

	_, err = f.WriteString(toml + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		tb.Fatalf("writing %s: %v", path, err)
	}
}

func repoConfigPath(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", "config", "path", "--repo")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("jj config path --repo: %w", err)
	}

	path := strings.TrimSpace(string(out))

	// The run's root holds the home and every temp directory, where a test may point
	// XDG_CONFIG_HOME of its own.
	root := filepath.Dir(os.Getenv("HOME"))
	if rel, err := filepath.Rel(root, path); err != nil || strings.HasPrefix(rel, "..") ||
		!strings.HasPrefix(filepath.Base(root), "jojo-test-") {
		return "", fmt.Errorf("%w: %s; call hermetic.Main from TestMain", ErrOutsideHome, path)
	}

	return path, nil
}
