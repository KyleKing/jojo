package gh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/kyleking/aragonite/vcs"
)

// ErrNoRemote marks a repository with no git remote, which gh has nothing to ask about.
var ErrNoRemote = errors.New("no git remote: jj git remote add origin <url> adds one")

// ErrAmbiguousRemote marks a repository whose remotes name more than one repository and gh has no
// default among them, where gh itself would stop to ask.
var ErrAmbiguousRemote = errors.New("the git remotes name different repositories and gh has no default")

// Remote is one git remote as PickRepo reads it: its name, its URL, and gh's mark on it
// (remote.<name>.gh-resolved), "base" on the remote gh repo set-default chose.
type Remote struct {
	Name     string
	URL      string
	Resolved string
}

// resolvedBase is the mark gh repo set-default leaves on the remote it chose.
const resolvedBase = "base"

// PickRepo is the repository gh is pointed at with --repo, as HOST/OWNER/REPO: the one gh's default
// names when it has one, else the one the remotes agree on. It is "" when the remote is not a forge
// URL gh could read (a path on disk), which leaves gh to its own lookup. No remote, and remotes naming
// more than one repository with no default, are errors that say how to fix them.
func PickRepo(remotes []Remote) (string, error) {
	if len(remotes) == 0 {
		return "", ErrNoRemote
	}

	for _, r := range remotes {
		switch {
		case r.Resolved == resolvedBase:
			return vcs.RemoteIdentity(r.URL), nil
		case strings.Count(r.Resolved, "/") >= 1:
			// gh records a default that is no remote's as OWNER/REPO on the remote it was set from.
			return r.Resolved, nil
		}
	}

	var (
		repos []string
		names []string
	)

	for _, r := range remotes {
		if id := vcs.RemoteIdentity(r.URL); id != "" && !slices.Contains(repos, id) {
			repos = append(repos, id)
			names = append(names, r.Name)
		}
	}

	switch len(repos) {
	case 0:
		return "", nil
	case 1:
		return repos[0], nil
	}

	return "", fmt.Errorf("%w (%s): gh repo set-default picks one", ErrAmbiguousRemote, strings.Join(names, ", "))
}

// WithRepo points every call at the repository resolve answers, as --repo, so gh asks the forge the
// jj remote names rather than guessing from the checkout. It is asked once, on the first call, and a
// failure is every call's failure, since asking the wrong repository is worse than asking none, and
// is asked again on the next call, since adding a remote or a default fixes it mid-session.
func WithRepo(resolve func(context.Context) (string, error)) Option {
	return func(c *CLI) { c.resolve = &repoOnce{resolve: resolve} }
}

// repoOnce asks for the repository until it has an answer, then keeps it.
type repoOnce struct {
	resolve func(context.Context) (string, error)
	repo    string
	mu      sync.Mutex
	done    bool
}

func (r *repoOnce) get(ctx context.Context) (string, error) {
	if r == nil {
		return "", nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.done {
		return r.repo, nil
	}

	repo, err := r.resolve(ctx)
	if err != nil {
		return "", err
	}

	r.repo, r.done = repo, true

	return repo, nil
}

// Repo is the repository calls are pointed at, "" when gh is left to find it.
func (c *CLI) Repo(ctx context.Context) (string, error) { return c.resolve.get(ctx) }
