// Command fakegh stands in for gh in tests: a forge kept as JSON under $FAKEGH_STATE that answers
// `pr view`, `pr create`, and `pr edit` the way GitHub would, and files every call it receives in
// calls.jsonl so a test can assert the exact commands a submit ran. Anything it does not route
// fails loudly, so a new call cannot pass as an empty answer.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type pr struct {
	Number         int    `json:"number"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	BaseRefName    string `json:"baseRefName"`
	HeadRefName    string `json:"headRefName"`
	URL            string `json:"url"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ReviewDecision string `json:"reviewDecision"`
	Rollup         []any  `json:"statusCheckRollup"`
}

type call struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin,omitempty"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dir := os.Getenv("FAKEGH_STATE")
	if dir == "" {
		return errors.New("fakegh: FAKEGH_STATE is not set")
	}

	stdin := ""
	if slices(args, "-") {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		stdin = string(data)
	}

	if err := record(dir, call{Args: args, Stdin: stdin}); err != nil {
		return err
	}

	prs, err := load(dir)
	if err != nil {
		return err
	}

	if len(args) < 2 || args[0] != "pr" {
		return fmt.Errorf("fakegh: unrouted call %q", args)
	}

	switch args[1] {
	case "view":
		return view(prs, args[2])
	case "create":
		return create(dir, prs, flags(args[2:]), stdin)
	case "edit":
		return edit(dir, prs, args[2], flags(args[3:]), stdin)
	}

	return fmt.Errorf("fakegh: unrouted call %q", args)
}

func slices(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}

	return false
}

func flags(args []string) map[string]string {
	out := map[string]string{}

	for i := 0; i < len(args); i++ {
		name := strings.TrimPrefix(args[i], "--")
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			out[name] = args[i+1]
			i++

			continue
		}

		out[name] = "true"
	}

	return out
}

func record(dir string, c call) error {
	f, err := os.OpenFile(filepath.Join(dir, "calls.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(c)
}

func load(dir string) ([]pr, error) {
	data, err := os.ReadFile(filepath.Join(dir, "prs.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var prs []pr

	return prs, json.Unmarshal(data, &prs)
}

func save(dir string, prs []pr) error {
	data, err := json.MarshalIndent(prs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "prs.json"), data, 0o600)
}

// find is the pull request a branch or number names: an open one first, else the newest.
func find(prs []pr, ref string) int {
	found := -1

	for i, p := range prs {
		if p.HeadRefName != ref && strconv.Itoa(p.Number) != ref {
			continue
		}

		if p.State == "OPEN" {
			return i
		}

		found = i
	}

	return found
}

func view(prs []pr, ref string) error {
	i := find(prs, ref)
	if i < 0 {
		return fmt.Errorf("no pull requests found for branch %q", ref)
	}

	return json.NewEncoder(os.Stdout).Encode(prs[i])
}

func create(dir string, prs []pr, f map[string]string, body string) error {
	if i := find(prs, f["head"]); i >= 0 && prs[i].State == "OPEN" {
		return fmt.Errorf("a pull request for branch %q into branch %q already exists", f["head"], f["base"])
	}

	n := len(prs) + 1
	p := pr{
		Number: n, State: "OPEN", IsDraft: f["draft"] == "true", BaseRefName: f["base"], HeadRefName: f["head"],
		Title: f["title"], Body: strings.TrimSuffix(body, "\n"), URL: "https://github.com/o/r/pull/" + strconv.Itoa(n),
	}

	if err := save(dir, append(prs, p)); err != nil {
		return err
	}

	fmt.Println(p.URL)

	return nil
}

func edit(dir string, prs []pr, ref string, f map[string]string, body string) error {
	i := find(prs, ref)
	if i < 0 || prs[i].State != "OPEN" {
		return fmt.Errorf("no open pull request found for %q", ref)
	}

	if base, ok := f["base"]; ok {
		prs[i].BaseRefName = base
	}

	if _, ok := f["body-file"]; ok {
		prs[i].Body = strings.TrimSuffix(body, "\n")
	}

	if err := save(dir, prs); err != nil {
		return err
	}

	fmt.Println(prs[i].URL)

	return nil
}
