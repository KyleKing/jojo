package forge

import (
	"context"
	"fmt"

	"github.com/kyleking/aragonite/forge/github"
	"github.com/kyleking/aragonite/vcs"
)

// githubProvider answers forge queries by shelling out to gh through
// aragonite/forge/github, which already carries the runner seam tests inject.
type githubProvider struct {
	repoPath string
	remoteID string
}

func newGitHubProvider(repoPath, remoteID string) *githubProvider {
	return &githubProvider{repoPath: repoPath, remoteID: remoteID}
}

func (*githubProvider) Name() string { return "github" }

// StatusFor issues the one gh call this bookmark needs per poll. A bookmark
// with no pull request is not an error: most bookmarks have none yet.
func (p *githubProvider) StatusFor(ctx context.Context, bookmark string) (*Status, error) {
	//nolint:errcheck // upstream only sharpens the cache key; an empty one still works
	upstream, _ := vcs.GetOperations(p.repoPath).GetUpstream(ctx, p.repoPath, bookmark)

	pr, err := github.GetPRForBranch(ctx, p.repoPath, p.remoteID, bookmark, upstream)
	if err != nil {
		if isNoPullRequest(err) {
			return &Status{}, nil
		}

		if isRateLimited(err) {
			return nil, fmt.Errorf("%w: %w", ErrRateLimited, err)
		}

		return nil, fmt.Errorf("fetching pull request for %s: %w", bookmark, err)
	}

	return &Status{PR: pr, Checks: pr.Checks, Behind: pr.Mergeable == "BEHIND"}, nil
}
