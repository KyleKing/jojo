package readmark

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ErrBusy marks a migration that could not take a scope file's lock: another process held it for
// longer than a save takes.
var ErrBusy = errors.New("read marks are being saved by another process")

const (
	lockWait  = 2 * time.Second
	lockStale = 10 * time.Second
	lockPoll  = 5 * time.Millisecond
	filePerm  = 0o600
	scopeExt  = ".toml"
)

// Migrate moves marks filed under a prefix of a change's full id into the file the full id names.
// An older jojo scoped marks by jj's change_id.shortest(8), which grows when another change comes to
// share its prefix, so the scope a change was filed under is a prefix of its full id. When dir has
// no file for full and exactly one file whose name is a proper prefix of full, that file's marks
// become full's: renamed, or merged into a full file another process wrote meanwhile. It takes the
// lock aragonite/seen's Save takes (the full file's path plus ".lock"), so a concurrent save of
// that scope waits rather than losing a mark. It reports whether it moved anything.
func Migrate(dir, full string) (bool, error) {
	target := filepath.Join(dir, full+scopeExt)
	if _, err := os.Stat(target); err == nil || !errors.Is(err, fs.ErrNotExist) {
		// A full file already there, or one that cannot be read, is not ours to move into.
		return false, nil
	}

	old, err := prefixFile(dir, full)
	if old == "" || err != nil {
		return false, err
	}

	unlock, err := lock(target)
	if err != nil {
		return false, err
	}

	moved, err := move(old, target)

	return moved, errors.Join(err, unlock())
}

// prefixFile is the one scope file in dir whose name is a proper prefix of full, "" when there is
// none or more than one, which leaves nothing safe to move.
func prefixFile(dir, full string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("listing %s: %w", dir, err)
	}

	var found []string

	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), scopeExt)
		if ok && !e.IsDir() && name != full && strings.HasPrefix(full, name) {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}

	if len(found) != 1 {
		return "", nil
	}

	return found[0], nil
}

// move renames old onto target, or merges it into a target written since Migrate looked.
func move(old, target string) (bool, error) {
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		if err := os.Rename(old, target); err != nil {
			return false, fmt.Errorf("moving %s to %s: %w", old, target, err)
		}

		return true, nil
	}

	merged, err := merge(target, old)
	if err != nil {
		return false, err
	}

	tmp := target + ".migrate"
	if err := os.WriteFile(tmp, merged, filePerm); err != nil {
		return false, fmt.Errorf("writing %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, target); err != nil {
		return false, fmt.Errorf("replacing %s: %w", target, err)
	}

	if err := os.Remove(old); err != nil {
		return false, fmt.Errorf("removing %s: %w", old, err)
	}

	return true, nil
}

// record and file are aragonite/seen's on-disk shape: a TOML array of [[hunk]] tables.
type record struct {
	Hash   string `toml:"hash"`
	Path   string `toml:"path"`
	Header string `toml:"header,omitempty"`
}

type file struct {
	Hunk []record `toml:"hunk"`
}

// merge is the union of the hunks two scope files hold, each hash once.
func merge(paths ...string) ([]byte, error) {
	var out file

	seen := map[string]bool{}

	for _, p := range paths {
		raw, err := os.ReadFile(p) //nolint:gosec // a scope file of the store being migrated
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}

		var f file
		if err := toml.NewDecoder(bytes.NewReader(raw)).Decode(&f); err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}

		for _, r := range f.Hunk {
			if !seen[r.Hash] {
				seen[r.Hash] = true
				out.Hunk = append(out.Hunk, r)
			}
		}
	}

	body, err := toml.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encoding the merged marks: %w", err)
	}

	return body, nil
}

// lock takes path's lock file the way aragonite/seen does: a sibling created exclusively, broken
// once it is older than any save takes.
func lock(path string) (func() error, error) {
	name := path + ".lock"
	deadline := time.Now().Add(lockWait)

	for {
		took, err := tryLock(name)
		if err != nil {
			return nil, fmt.Errorf("locking %s: %w", path, err)
		}

		if took {
			return func() error {
				if err := os.Remove(name); err != nil {
					return fmt.Errorf("unlocking %s: %w", path, err)
				}

				return nil
			}, nil
		}

		if info, err := os.Stat(name); err == nil && time.Since(info.ModTime()) > lockStale {
			if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("breaking a stale lock on %s: %w", path, err)
			}

			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("migrating %s: %w", path, ErrBusy)
		}

		time.Sleep(lockPoll)
	}
}

// tryLock creates the lock file and reports whether it did; one that already exists is not an
// error.
func tryLock(name string) (bool, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filePerm) //nolint:gosec // the store's own
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("creating %s: %w", name, err)
	}

	if err := f.Close(); err != nil {
		return false, fmt.Errorf("closing %s: %w", name, err)
	}

	return true, nil
}
