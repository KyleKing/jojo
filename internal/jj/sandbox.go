package jj

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sandbox is a throwaway copy of a repository's jj metadata that a write can run in without touching
// the repository it came from, which is how the destination picker draws the graph a rebase, squash,
// duplicate, or new will leave before any of them runs for real.
//
// The copy shares nothing writable with the original. Its op log, op heads, index, and change-id
// table are hard links (or copies) of the original's content-addressed files, which jj only ever
// adds beside rather than rewrites. Its git store is a new bare repository that borrows the
// original's objects through git alternates, so every commit, keep ref, and exported bookmark a
// preview writes lands in the sandbox. The original's op heads, refs, working copy, and user config
// stay byte for byte what they were.
type Sandbox struct {
	dir    string
	ws     string
	config string
	// base is the operation the sandbox was copied at, which the restore fallback returns to.
	base string
	mu   sync.Mutex
	// integrates is set once jj has refused --no-integrate-operation, which only jj 0.47 and later
	// accept. Each preview is then undone with op restore instead.
	integrates bool
	closed     bool
}

// Preview is the log a write would leave behind.
type Preview struct {
	Rows []LogRow
	// Unchanged says jj ran the write and moved nothing, so Rows is the log as it already is.
	Unchanged bool
}

// errSandboxClosed is what a preview racing the picker's close reports.
var errSandboxClosed = errors.New("the preview sandbox is closed")

// errNotGitDir names a path the repository's store points at that holds no git repository.
var errNotGitDir = errors.New("not a git directory")

// errNotGitBacked names a repository the sandbox cannot copy, since only the git backend keeps its
// commits somewhere alternates can share.
var errNotGitBacked = errors.New("previews need a git-backed repository")

// sandboxMaxAge is how long a sandbox a crashed jojo left behind is kept before the next one sweeps
// it. A live picker session lasts seconds, so an hour old is abandoned.
const sandboxMaxAge = time.Hour

const (
	sandboxDirMode  = 0o750
	sandboxFileMode = 0o600
)

// OpenSandbox snapshots the working copy and copies the repository's jj metadata into a new
// sandbox. It holds the command queue while it copies, so no write this client runs can land half
// way through the copy.
func (c *Client) OpenSandbox(ctx context.Context) (*Sandbox, error) {
	c.AcquireQueue()
	defer c.ReleaseQueue()

	// Any command that loads the workspace snapshots it first, which is what puts unsaved edits in
	// the copy, and `jj root` names the workspace the copy has to stand in for.
	if _, _, err := c.executeLocked(ctx, "log", "--no-graph", "-r", "@", "-T", "''"); err != nil {
		return nil, err
	}

	root, _, err := c.executeLocked(ctx, "root")
	if err != nil {
		return nil, err
	}

	config, err := c.scopedConfig(ctx)
	if err != nil {
		return nil, err
	}

	parent := sandboxParent()
	sweepSandboxes(parent, time.Now())

	return buildSandbox(ctx, strings.TrimSpace(root), parent, config)
}

// scopedConfig is the repository's and the workspace's own settings as one TOML document. The
// sandbox is handed them as a file rather than a copy of the config-id, because jj answers a copied
// id by copying the whole config directory into the user's jj config under a new id.
func (c *Client) scopedConfig(ctx context.Context) (string, error) {
	var out strings.Builder

	for _, scope := range []string{"--repo", "--workspace"} {
		text, _, err := c.executeLocked(ctx, "config", "list", scope)
		if err != nil {
			return "", err
		}

		out.WriteString(text)
	}

	return out.String(), nil
}

func buildSandbox(ctx context.Context, root, parent, config string) (*Sandbox, error) {
	repo, err := repoDir(root)
	if err != nil {
		return nil, err
	}

	objects, err := gitObjects(filepath.Join(repo, "store"))
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(parent, sandboxDirMode); err != nil {
		return nil, fmt.Errorf("preview sandbox: %w", err)
	}

	dir, err := os.MkdirTemp(parent, strconv.Itoa(os.Getpid())+"-")
	if err != nil {
		return nil, fmt.Errorf("preview sandbox: %w", err)
	}

	s := &Sandbox{dir: dir, ws: filepath.Join(dir, "ws"), config: filepath.Join(dir, "config.toml")}

	if err := s.populate(root, repo, objects, config); err != nil {
		_ = os.RemoveAll(dir) //nolint:errcheck // the populate failure is the error worth reporting.

		return nil, err
	}

	base, _, err := s.run(ctx, "op", "log", "-n", "1", "--no-graph", "-T", "id")
	if err != nil {
		_ = os.RemoveAll(dir) //nolint:errcheck // the jj failure is the error worth reporting.

		return nil, err
	}

	s.base = strings.TrimSpace(base)

	return s, nil
}

// populate lays out the sandbox: a bare git repository borrowing the original's objects, a copy of
// .jj/repo pointing at it, and the workspace's checkout record so @ names the same workspace.
func (s *Sandbox) populate(root, repo, objects, config string) error {
	git := filepath.Join(s.dir, "git")
	if err := bareGit(git, objects); err != nil {
		return err
	}

	sandboxRepo := filepath.Join(s.ws, ".jj", "repo")
	if err := copyTree(repo, sandboxRepo); err != nil {
		return fmt.Errorf("preview sandbox: %w", err)
	}

	target := filepath.Join(sandboxRepo, "store", "git_target")
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("preview sandbox: %w", err)
	}

	if err := os.WriteFile(target, []byte(git), sandboxFileMode); err != nil {
		return fmt.Errorf("preview sandbox: %w", err)
	}

	workingCopy := filepath.Join(s.ws, ".jj", "working_copy")
	if err := os.MkdirAll(workingCopy, sandboxDirMode); err != nil {
		return fmt.Errorf("preview sandbox: %w", err)
	}

	for _, name := range []string{"checkout", "type"} {
		from := filepath.Join(root, ".jj", "working_copy", name)
		if err := copyFile(from, filepath.Join(workingCopy, name)); err != nil {
			return fmt.Errorf("preview sandbox: %w", err)
		}
	}

	if err := os.WriteFile(s.config, []byte(config), sandboxFileMode); err != nil {
		return fmt.Errorf("preview sandbox: %w", err)
	}

	return nil
}

// repoDir follows a secondary workspace's .jj/repo file to the repository it shares.
func repoDir(root string) (string, error) {
	repo := filepath.Join(root, ".jj", "repo")

	info, err := os.Stat(repo)
	if err != nil {
		return "", fmt.Errorf("preview sandbox: %w", err)
	}

	if info.IsDir() {
		return repo, nil
	}

	target, err := os.ReadFile(repo) //nolint:gosec // G304: jj's own pointer file, inside the workspace.
	if err != nil {
		return "", fmt.Errorf("preview sandbox: %w", err)
	}

	path := strings.TrimSpace(string(target))
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, ".jj", path)
	}

	return filepath.Clean(path), nil
}

// gitObjects finds the object directory of the git repository jj stores commits in, following a
// .git file (a worktree or submodule) and a commondir to where the objects actually live.
func gitObjects(store string) (string, error) {
	kind, err := os.ReadFile(filepath.Join(store, "type")) //nolint:gosec // G304: jj's store marker.
	if err != nil || strings.TrimSpace(string(kind)) != "git" {
		return "", errNotGitBacked
	}

	target, err := os.ReadFile(filepath.Join(store, "git_target")) //nolint:gosec // G304: jj's store pointer.
	if err != nil {
		return "", fmt.Errorf("preview sandbox: %w", err)
	}

	dir := strings.TrimSpace(string(target))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(store, dir)
	}

	dir, err = followGitFile(filepath.Clean(dir))
	if err != nil {
		return "", err
	}

	if common, err := os.ReadFile(filepath.Join(dir, "commondir")); err == nil { //nolint:gosec // G304: git's own.
		next := strings.TrimSpace(string(common))
		if !filepath.IsAbs(next) {
			next = filepath.Join(dir, next)
		}

		dir = filepath.Clean(next)
	}

	objects := filepath.Join(dir, "objects")
	if info, err := os.Stat(objects); err != nil || !info.IsDir() {
		return "", fmt.Errorf("preview sandbox: %w: no objects at %s", errNotGitDir, objects)
	}

	return objects, nil
}

func followGitFile(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("preview sandbox: %w", err)
	}

	if info.IsDir() {
		return dir, nil
	}

	text, err := os.ReadFile(dir) //nolint:gosec // G304: a .git file naming the real git directory.
	if err != nil {
		return "", fmt.Errorf("preview sandbox: %w", err)
	}

	path, found := strings.CutPrefix(strings.TrimSpace(string(text)), "gitdir:")
	if !found {
		return "", fmt.Errorf("preview sandbox: %w: %s", errNotGitDir, dir)
	}

	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(dir), path)
	}

	return filepath.Clean(path), nil
}

// bareGit writes the smallest bare repository git and gix accept, without needing a git binary, and
// points it at objects so every existing commit reads through it.
func bareGit(dir, objects string) error {
	for _, sub := range []string{"objects/info", "objects/pack", "refs/heads", "refs/tags"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), sandboxDirMode); err != nil {
			return fmt.Errorf("preview sandbox: %w", err)
		}
	}

	files := map[string]string{
		"HEAD":                    "ref: refs/heads/main\n",
		"config":                  "[core]\n\trepositoryformatversion = 0\n\tbare = true\n",
		"objects/info/alternates": objects + "\n",
	}

	// A shallow clone's boundary commits name parents that are not there, and the file saying so has
	// to travel with the objects or reading their history fails.
	shallowFile := filepath.Join(filepath.Dir(objects), "shallow")
	if shallow, err := os.ReadFile(shallowFile); err == nil { //nolint:gosec // G304: git's own file.
		files["shallow"] = string(shallow)
	}

	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), sandboxFileMode); err != nil {
			return fmt.Errorf("preview sandbox: %w", err)
		}
	}

	return nil
}

// skipped are the files the copy leaves behind: the repository's config identity (see scopedConfig),
// a legacy repo config jj would migrate into the user's config on first read, and lock files.
var skipped = map[string]bool{"config-id": true, "config.toml": true, "lock": true}

// copied are the directories whose files jj deletes and recreates by name rather than only adding
// beside, so they are copied outright instead of linked.
var copied = map[string]bool{"op_heads": true, filepath.Join("store", "extra", "heads"): true}

// copyTree mirrors src under dst. Content-addressed files are hard linked, which costs no space and
// no time on a large op log, and fall back to a copy across filesystems.
func copyTree(src, dst string) error {
	//nolint:gosec // G122: src is jj's own .jj/repo and dst a directory this process just created.
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("copying %s: %w", path, err)
		}

		target := filepath.Join(dst, rel)

		if entry.IsDir() {
			return os.MkdirAll(target, sandboxDirMode)
		}

		if skipped[entry.Name()] || !entry.Type().IsRegular() {
			return nil
		}

		if copied[filepath.Dir(rel)] || filepath.Dir(rel) == "." || filepath.Dir(rel) == "store" {
			return copyFile(path, target)
		}

		if os.Link(path, target) == nil {
			return nil
		}

		return copyFile(path, target)
	})
	if err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: a file inside the repository's own .jj directory.
	if err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}
	defer in.Close() //nolint:errcheck // read-only; a close failure loses nothing.

	//nolint:gosec // G304: dst is inside the sandbox this process created.
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, sandboxFileMode)
	if err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() //nolint:errcheck // the copy failure is the error worth reporting.

		return fmt.Errorf("copying %s: %w", src, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}

	return nil
}

// sandboxParent keeps sandboxes in the user cache directory, which is usually on the same
// filesystem as a home directory full of repositories, so the copy can be hard links.
func sandboxParent() string {
	if cache, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cache, "jojo", "preview")
	}

	return filepath.Join(os.TempDir(), "jojo-preview")
}

// sweepSandboxes removes what a jojo that never reached Close left behind.
func sweepSandboxes(parent string, now time.Time) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < sandboxMaxAge {
			continue
		}

		_ = os.RemoveAll(filepath.Join(parent, entry.Name())) //nolint:errcheck // best effort; the next sweep retries.
	}
}

// Dir is where the sandbox lives, which a test reads to prove Close removed it.
func (s *Sandbox) Dir() string { return s.dir }

// Close removes the sandbox. It waits for a preview already running, and every later preview fails.
func (s *Sandbox) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("removing the preview sandbox: %w", err)
	}

	return nil
}

// uncommitted is how jj names the operation --no-integrate-operation left out of the op log.
var uncommitted = regexp.MustCompile(`--no-integrate-operation was requested: ([0-9a-f]+)`)

// noIntegrateFlag keeps a preview's operation out of even the sandbox's op log.
const noIntegrateFlag = "--no-integrate-operation"

// Preview runs args in the sandbox and returns the log over revset as it would read afterwards. The
// sandbox's op heads never move, so every preview starts from the state the sandbox was copied at.
func (s *Sandbox) Preview(ctx context.Context, args []string, revset string, limit int) (Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return Preview{}, errSandboxClosed
	}

	if s.integrates {
		return s.previewByRestore(ctx, args, revset, limit)
	}

	_, notice, err := s.run(ctx, append(slices.Clone(args), noIntegrateFlag)...)

	var failed *CommandError
	if errors.As(err, &failed) && strings.Contains(failed.Stderr, "unexpected argument '"+noIntegrateFlag) {
		s.integrates = true

		return s.previewByRestore(ctx, args, revset, limit)
	}

	if err != nil {
		return Preview{}, err
	}

	match := uncommitted.FindStringSubmatch(notice)
	if match == nil {
		// jj wrote no operation, which it says only when the write moved nothing.
		rows, err := s.log(ctx, "", revset, limit)

		return Preview{Rows: rows, Unchanged: true}, err
	}

	rows, err := s.log(ctx, match[1], revset, limit)

	return Preview{Rows: rows}, err
}

// previewByRestore is the path for a jj without --no-integrate-operation: write, read, and put the
// sandbox back at the operation it was copied at.
func (s *Sandbox) previewByRestore(ctx context.Context, args []string, revset string, limit int) (Preview, error) {
	_, notice, err := s.run(ctx, args...)
	if err != nil {
		return Preview{}, err
	}

	rows, logErr := s.log(ctx, "", revset, limit)

	if _, _, err := s.run(ctx, "op", "restore", s.base); err != nil {
		s.closed = true

		return Preview{}, err
	}

	return Preview{Rows: rows, Unchanged: strings.Contains(notice, nothingChangedNotice)}, logErr
}

func (s *Sandbox) log(ctx context.Context, op, revset string, limit int) ([]LogRow, error) {
	var args []string
	if op != "" {
		args = append(args, "--at-op", op)
	}

	args = append(args, logArgs(revset, limit)...)

	out, _, err := s.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseLog(out), nil
}

// run is a jj call against the sandbox: never the working copy, never the user's signing key, which
// would otherwise prompt for a passphrase on every cursor move.
func (s *Sandbox) run(ctx context.Context, args ...string) (string, string, error) {
	full := append([]string{
		"--no-pager", "--color=never", "--ignore-working-copy",
		"--config-file", s.config, "--config", "signing.behavior=drop",
	}, args...)

	//nolint:gosec // G204: the binary is a literal and the arguments are built by this package.
	cmd := exec.CommandContext(ctx, "jj", full...)
	cmd.Dir = s.ws
	cmd.Env = append(os.Environ(), "JJ_EDITOR=false", "EDITOR=false", "VISUAL=false")

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return "", "", wrapJJError(args, stderr.String())
	}

	return stdout.String(), stderr.String(), nil
}
