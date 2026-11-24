// Package ghtest builds a fake gh for tests and reads what it was asked. The fake keeps a forge as
// JSON in a state directory, answers pr view, create, and edit from it, and records every call, so
// a test drives a real jj repository with a bare git remote while the forge stays on disk.
package ghtest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/kyleking/aragonite/trail"
)

// stateMode is the fake's files: private to the test.
const stateMode = 0o600

// Forge is one fake forge: the binary to run as gh and the directory its state lives in.
type Forge struct {
	Bin   string
	State string
}

// Env is the variable that points the fake at this forge's state.
func (f Forge) Env() string { return "FAKEGH_STATE=" + f.State }

// errBuildFailed marks a fake that would not compile.
var errBuildFailed = errors.New("go build")

var (
	buildOnce sync.Once
	built     string
	errBuild  error
)

// New builds the fake once per test binary and gives the test a forge of its own.
func New(tb testing.TB) Forge {
	tb.Helper()

	buildOnce.Do(func() { built, errBuild = build() })

	if errBuild != nil {
		tb.Fatalf("building the fake gh: %v", errBuild)
	}

	return Forge{Bin: built, State: tb.TempDir()}
}

func build() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("%w: no caller to find the fake's source by", errBuildFailed)
	}

	dir, err := os.MkdirTemp("", "fakegh")
	if err != nil {
		return "", fmt.Errorf("making the fake's directory: %w", err)
	}

	bin := filepath.Join(dir, "gh")
	src := filepath.Join(filepath.Dir(file), "testdata", "fakegh")

	//nolint:gosec // G204: the go binary with a fixed package path.
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".")
	cmd.Dir = src

	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%w: %s", errBuildFailed, out)
	}

	return bin, nil
}

// Call is one invocation the fake received.
type Call struct {
	Stdin string   `json:"stdin,omitempty"`
	Args  []string `json:"args"`
}

// Line is the call as a shell line, the way the trail prints it.
func (c Call) Line() string { return trail.Shell(append([]string{"gh"}, c.Args...)) }

// Calls is every call the forge received, oldest first.
func (f Forge) Calls(tb testing.TB) []Call {
	tb.Helper()

	file, err := os.Open(filepath.Join(f.State, "calls.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // a read-only file has nothing to flush

	var out []Call

	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 0, 1<<16), 1<<20) //nolint:mnd // a body fits in a megabyte

	for scan.Scan() {
		var c Call
		if err := json.Unmarshal(scan.Bytes(), &c); err != nil {
			tb.Fatal(err)
		}

		out = append(out, c)
	}

	return out
}

// Writes is the calls that change the forge, as shell lines: what a submit did, without its reads.
func (f Forge) Writes(tb testing.TB) []string {
	tb.Helper()

	var out []string

	for _, c := range f.Calls(tb) {
		if len(c.Args) > 1 && c.Args[1] != "view" {
			out = append(out, c.Line())
		}
	}

	return out
}

// Reset forgets the calls so far, keeping the forge's pull requests.
func (f Forge) Reset(tb testing.TB) {
	tb.Helper()

	if err := os.Remove(filepath.Join(f.State, "calls.jsonl")); err != nil && !errors.Is(err, os.ErrNotExist) {
		tb.Fatal(err)
	}
}

// PR is one pull request as the fake stores it, in gh's field names.
//
//nolint:tagliatelle // gh's JSON names, not ours.
type PR struct {
	State          string `json:"state"`
	BaseRefName    string `json:"baseRefName"`
	HeadRefName    string `json:"headRefName"`
	URL            string `json:"url"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ReviewDecision string `json:"reviewDecision"`
	Rollup         []any  `json:"statusCheckRollup"`
	Number         int    `json:"number"`
	IsDraft        bool   `json:"isDraft"`
}

// PRs is the forge's pull requests.
func (f Forge) PRs(tb testing.TB) []PR {
	tb.Helper()

	data, err := os.ReadFile(filepath.Join(f.State, "prs.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		tb.Fatal(err)
	}

	var prs []PR
	if err := json.Unmarshal(data, &prs); err != nil {
		tb.Fatal(err)
	}

	return prs
}

// SetPRs replaces the forge's pull requests.
func (f Forge) SetPRs(tb testing.TB, prs []PR) {
	tb.Helper()

	data, err := json.MarshalIndent(prs, "", "  ")
	if err != nil {
		tb.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(f.State, "prs.json"), data, stateMode); err != nil {
		tb.Fatal(err)
	}
}

// Merge marks the open pull request for head merged, the way the forge would after a merge button.
func (f Forge) Merge(tb testing.TB, head string) {
	tb.Helper()

	prs := f.PRs(tb)
	for i := range prs {
		if prs[i].HeadRefName == head && prs[i].State == "OPEN" {
			prs[i].State = "MERGED"
		}
	}

	f.SetPRs(tb, prs)
}

// For is the newest pull request for head, the zero PR when it has none.
func (f Forge) For(tb testing.TB, head string) PR {
	tb.Helper()

	var found PR

	prs := f.PRs(tb)
	for i := range prs {
		if prs[i].HeadRefName == head {
			found = prs[i]
		}
	}

	return found
}
