// Package gh runs the GitHub CLI for the writes jojo makes to a forge: reading, opening, and editing
// the pull requests of a stack. Every call is filed in the same trail as jj's, so gm lists the gh
// commands a submit ran beside the jj ones, with what each said on stderr.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kyleking/aragonite/trail"
)

// commandWords is how much of a failed call its error names: "pr create".
const commandWords = 2

// ErrFailed wraps any non-zero gh exit, with what gh said on stderr.
var ErrFailed = errors.New("gh failed")

// ErrNotInstalled marks a gh binary that is not on PATH.
var ErrNotInstalled = errors.New("gh is not on PATH")

// PR states as gh reports them.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// PR is what a stack needs to know about one pull request, in gh's own field names.
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
	// Checks is the CI rollup in one word: pass, fail, pending, or "" when no check ran.
	Checks  string `json:"-"`
	Number  int    `json:"number"`
	IsDraft bool   `json:"isDraft"`
}

// viewFields is what PRFor asks gh for.
const viewFields = "number,state,isDraft,baseRefName,headRefName,url,title,body,reviewDecision,statusCheckRollup"

// CLI runs gh in one checkout.
type CLI struct {
	trail *trail.Trail
	dir   string
	bin   string
	env   []string
}

// Option configures a CLI.
type Option func(*CLI)

// WithBinary runs bin instead of the gh on PATH, which is how tests put a fake forge in its place.
func WithBinary(bin string) Option { return func(c *CLI) { c.bin = bin } }

// WithEnv adds variables to every call's environment.
func WithEnv(env ...string) Option { return func(c *CLI) { c.env = append(c.env, env...) } }

// New runs gh in dir, filing every call in t (nil files nothing).
func New(dir string, t *trail.Trail, opts ...Option) *CLI {
	c := &CLI{dir: dir, trail: t, bin: "gh"}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Installed reports whether the binary the CLI runs can be found.
func (c *CLI) Installed() error {
	if _, err := exec.LookPath(c.bin); err != nil {
		return fmt.Errorf("%w: %s", ErrNotInstalled, c.bin)
	}

	return nil
}

// Run runs gh with args, stdin fed from stdin when it is not empty, and answers stdout. The trail
// files it as a write when write is set.
func (c *CLI) Run(ctx context.Context, write bool, stdin string, args ...string) (string, error) {
	//nolint:gosec // G204: the binary is fixed at construction; only the arguments vary and no shell runs.
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = c.dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1", "GH_NO_UPDATE_NOTIFIER=1")
	cmd.Env = append(cmd.Env, c.env...)

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	start := time.Now()
	err := cmd.Run()

	c.trail.Add(trail.Call{
		Argv: append([]string{"gh"}, args...), Dir: c.dir, Write: write, Start: start, Took: time.Since(start),
		Exit: trail.ExitCode(err), Stderr: stderr.String(),
	})

	if err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = err.Error()
		}

		return "", fmt.Errorf("%w: gh %s: %s", ErrFailed, strings.Join(args[:min(commandWords, len(args))], " "), said)
	}

	return stdout.String(), nil
}

// ViewArgs is the gh call that reads the pull request for branch.
func ViewArgs(branch string) []string {
	return []string{"pr", "view", branch, "--json", viewFields}
}

// PRFor answers the pull request whose head is branch, nil when it has none.
func (c *CLI) PRFor(ctx context.Context, branch string) (*PR, error) {
	out, err := c.Run(ctx, false, "", ViewArgs(branch)...)
	if err != nil {
		if strings.Contains(err.Error(), "no pull requests found") {
			return nil, nil //nolint:nilnil // no pull request is an answer, not a failure
		}

		return nil, err
	}

	return ParsePR([]byte(out))
}

// ParsePR reads the JSON PRFor asks for.
func ParsePR(data []byte) (*PR, error) {
	//nolint:tagliatelle // gh's JSON names, not ours.
	var raw struct {
		Rollup []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
		} `json:"statusCheckRollup"`

		PR
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("reading gh's pull request: %w", err)
	}

	pr := raw.PR
	for _, r := range raw.Rollup {
		pr.Checks = worse(pr.Checks, checkWord(r.Status, r.Conclusion, r.State))
	}

	return &pr, nil
}

// checkWord is one check run or commit status in the rollup's three words.
func checkWord(status, conclusion, state string) string {
	switch strings.ToUpper(conclusion + state) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return checkPass
	case "FAILURE", "ERROR", "CANCELLED", //nolint:misspell // GitHub spells it so
		"TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return checkFail
	}

	if status != "" || state != "" {
		return checkPending
	}

	return ""
}

// How much each rollup word matters, the worst highest.
const (
	rankNone = iota
	rankPass
	rankPending
	rankFail
)

const (
	checkPass    = "pass"
	checkFail    = "fail"
	checkPending = "pending"
)

// worse keeps the rollup word that matters more: a failure over anything running, running over
// passing.
func worse(a, b string) string {
	rank := map[string]int{"": rankNone, checkPass: rankPass, checkPending: rankPending, checkFail: rankFail}
	if rank[b] > rank[a] {
		return b
	}

	return a
}

// CreateOptions is a pull request to open.
type CreateOptions struct {
	Head  string
	Base  string
	Title string
	Draft bool
}

// CreateArgs is the gh call that opens a pull request, its body read from stdin.
func CreateArgs(opts CreateOptions) []string {
	args := []string{
		"pr", "create", "--head", opts.Head, "--base", opts.Base, "--title", opts.Title, "--body-file", "-",
	}
	if opts.Draft {
		args = append(args, "--draft")
	}

	return args
}

// Create opens a pull request and answers its number, read from the URL gh prints.
func (c *CLI) Create(ctx context.Context, opts CreateOptions, body string) (int, string, error) {
	out, err := c.Run(ctx, true, bodyOrBlank(body), CreateArgs(opts)...)
	if err != nil {
		return 0, "", err
	}

	url := lastLine(out)

	return NumberOf(url), url, nil
}

// RetargetArgs is the gh call that moves a pull request onto another base.
func RetargetArgs(branch, base string) []string {
	return []string{"pr", "edit", branch, "--base", base}
}

// Retarget moves branch's pull request onto base.
func (c *CLI) Retarget(ctx context.Context, branch, base string) error {
	_, err := c.Run(ctx, true, "", RetargetArgs(branch, base)...)

	return err
}

// BodyArgs is the gh call that replaces a pull request's body with stdin.
func BodyArgs(branch string) []string {
	return []string{"pr", "edit", branch, "--body-file", "-"}
}

// SetBody replaces branch's pull request body.
func (c *CLI) SetBody(ctx context.Context, branch, body string) error {
	_, err := c.Run(ctx, true, bodyOrBlank(body), BodyArgs(branch)...)

	return err
}

// bodyOrBlank keeps an empty body from leaving stdin unset, which gh would wait on.
func bodyOrBlank(body string) string {
	if body == "" {
		return "\n"
	}

	return body
}

// NumberOf is the pull request number at the end of its URL, 0 when there is none.
func NumberOf(url string) int {
	n, err := strconv.Atoi(url[strings.LastIndex(url, "/")+1:])
	if err != nil {
		return 0
	}

	return n
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	return strings.TrimSpace(lines[len(lines)-1])
}

// Summary is a pull request in the few words an outline row has room for: number, state, review,
// and CI.
func (p *PR) Summary() string {
	if p == nil {
		return ""
	}

	parts := []string{"#" + strconv.Itoa(p.Number)}

	switch {
	case p.State == StateMerged:
		parts = append(parts, "merged")
	case p.State == StateClosed:
		parts = append(parts, "closed")
	case p.IsDraft:
		parts = append(parts, "draft")
	default:
		parts = append(parts, "open")
	}

	switch p.ReviewDecision {
	case "APPROVED":
		parts = append(parts, "approved")
	case "CHANGES_REQUESTED":
		parts = append(parts, "changes")
	}

	if p.Checks != "" && p.State == StateOpen {
		parts = append(parts, "CI "+p.Checks)
	}

	return strings.Join(parts, " · ")
}
