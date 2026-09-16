// Package forge detects which code host a checkout's remote points at and
// polls it for the pull request state of a bookmark, independent of any
// rendering concerned with how that state looks on screen.
package forge

import (
	"context"
	"errors"
	"strings"
	"time"

	aforge "github.com/kyleking/aragonite/forge"
	"github.com/kyleking/aragonite/vcs"
)

// ErrRateLimited wraps a fetch refused because the host's API budget is
// exhausted, so callers can render it as a state rather than a generic error.
var ErrRateLimited = errors.New("forge: rate limit exceeded")

// Status is a bookmark's forge-derived state as of one fetch: the pull
// request it has, if any, the CI rollup for that pull request's head, and
// whether the head is behind its base.
type Status struct {
	FetchedAt time.Time
	PR        *aforge.PullRequest
	Checks    aforge.ChecksStatus
	Behind    bool
}

// Provider answers forge queries for one repository, reporting only the CI
// aggregate in Status.Checks since per-check granularity varies by host.
type Provider interface {
	Name() string
	StatusFor(ctx context.Context, bookmark string) (*Status, error)
}

// Detection is the result of reading a checkout's remote. Provider is nil
// when Host names a remote this package has no implementation for, so a
// caller can say so plainly instead of treating it as a failed fetch.
type Detection struct {
	Provider Provider
	Host     string
	RemoteID string
}

// Supported reports whether Detect found a remote this package can talk to.
func (d Detection) Supported() bool {
	return d.Provider != nil
}

// Detect reads repoPath's remote and returns the provider for its host, or a
// Detection naming the host with no provider when the host isn't implemented,
// or a zero Detection when the checkout has no resolvable remote at all.
func Detect(ctx context.Context, repoPath string) Detection {
	remoteURL, err := vcs.GetOperations(repoPath).GetRemoteURL(ctx, repoPath)
	if err != nil || remoteURL == "" {
		return Detection{}
	}

	identity := vcs.RemoteIdentity(remoteURL)

	host, _, ok := strings.Cut(identity, "/")
	if !ok || host == "" {
		return Detection{}
	}

	if host != "github.com" {
		return Detection{Host: host}
	}

	return Detection{Host: host, RemoteID: identity, Provider: newGitHubProvider(repoPath, identity)}
}

func isNoPullRequest(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no pull requests found")
}

func isRateLimited(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "rate limit")
}
