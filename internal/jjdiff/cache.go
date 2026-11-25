package jjdiff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// cacheFile is where ProbeCached keeps its answer, under the directory it is given.
const cacheFile = "jj-diff-probe.json"

// cacheMode is the answer file's mode: it names a binary and its version, nothing private.
const cacheMode = 0o600

// cachedProbe is one remembered answer, valid for the binary at Path while its size and modification
// time are the ones recorded.
type cachedProbe struct {
	Path    string `json:"path"`
	Tool    Tool   `json:"tool"`
	ModTime int64  `json:"mod_time"`
	Size    int64  `json:"size"`
}

// ProbeCached is Probe remembered in dir, keyed by the binary's path, size, and modification time: a
// probe runs jj-diff once or twice, which costs more than the first frame takes to draw, and its
// answer changes only when the binary does. An empty dir, or one that cannot be written, probes every
// time.
func ProbeCached(ctx context.Context, find LookPath, run Runner, dir string) Tool {
	path, err := find(Binary)
	if err != nil || dir == "" {
		return Probe(ctx, find, run)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Probe(ctx, find, run)
	}

	key := cachedProbe{Path: path, ModTime: info.ModTime().UnixNano(), Size: info.Size()}
	file := filepath.Join(dir, cacheFile)

	if remembered, ok := readCache(file); ok && remembered.Path == key.Path &&
		remembered.ModTime == key.ModTime && remembered.Size == key.Size {
		return remembered.Tool
	}

	key.Tool = Probe(ctx, func(string) (string, error) { return path, nil }, run)
	_ = writeCache(file, key) //nolint:errcheck // an unwritable cache only means the next launch probes again.

	return key.Tool
}

func readCache(file string) (cachedProbe, bool) {
	data, err := os.ReadFile(file) //nolint:gosec // G304: the cache file under the user's cache directory.
	if err != nil {
		return cachedProbe{}, false
	}

	var c cachedProbe
	if json.Unmarshal(data, &c) != nil {
		return cachedProbe{}, false
	}

	return c, true
}

func writeCache(file string, c cachedProbe) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encoding the probe: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil { //nolint:mnd // the user's own cache dir
		return fmt.Errorf("making the cache directory: %w", err)
	}

	if err := os.WriteFile(file, data, cacheMode); err != nil {
		return fmt.Errorf("writing the probe: %w", err)
	}

	return nil
}
