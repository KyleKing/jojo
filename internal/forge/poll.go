package forge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kyleking/aragonite/cache"
	"github.com/kyleking/aragonite/forge/github"
	"github.com/kyleking/aragonite/vcs"
)

// MinInterval and MaxInterval bound how often a focused pane polls, backing
// off toward MaxInterval as the GraphQL budget runs low.
const (
	MinInterval = 15 * time.Second
	MaxInterval = 60 * time.Second
)

const lowBudgetFraction = 0.1

const midBudgetFraction = 0.3

// midIntervalDivisor halves the min/max sum to get the middle tier's interval.
const midIntervalDivisor = 2

// Poller holds a bookmark's last-known Status and decides when it is next
// worth fetching; a caller drives Fetch and reads Interval to schedule it.
type Poller struct {
	lastErr     error
	provider    Provider
	lastStatus  *Status
	memCache    *cache.TTLCache[*Status]
	disk        *cache.DiskCache
	now         func() time.Time
	repoPath    string
	remoteID    string
	bookmark    string
	budget      github.Allowance
	mu          sync.Mutex
	focused     bool
	fetchedOnce bool
}

// Option configures a Poller at construction.
type Option func(*Poller)

// WithDiskCache sets where a Poller persists its last-known Status across
// restarts. A nil store (the default) keeps state in memory only.
func WithDiskCache(d *cache.DiskCache) Option {
	return func(p *Poller) { p.disk = d }
}

// WithClock overrides the source of Status.FetchedAt, so tests can advance
// time without sleeping.
func WithClock(now func() time.Time) Option {
	return func(p *Poller) { p.now = now }
}

// WithFocused sets the poller's initial focus state.
func WithFocused(focused bool) Option {
	return func(p *Poller) { p.focused = focused }
}

// NewPoller returns a Poller seeded from whatever cached Status survives for
// bookmark, so a cold start has something to render before Fetch completes.
func NewPoller(provider Provider, repoPath, remoteID, bookmark string, opts ...Option) *Poller {
	p := &Poller{
		provider: provider,
		repoPath: repoPath,
		remoteID: remoteID,
		bookmark: bookmark,
		memCache: cache.NewTTLCache[*Status](MaxInterval),
		now:      time.Now,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.lastStatus, _ = cache.PersistedUsing(p.disk, p.memCache, p.remoteID, p.cacheKey(), vcs.Stamp(p.repoPath))

	return p
}

// UserDiskCache returns the disk store a Poller should persist through in
// production, under the platform's per-user cache directory.
func UserDiskCache() *cache.DiskCache {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil
	}

	return cache.NewDiskCache(filepath.Join(base, "jojo", "forge"))
}

// Fetch runs one live query and updates the poller's last-known state,
// skipping the query and returning ErrRateLimited when the budget is at zero.
func (p *Poller) Fetch(ctx context.Context) (*Status, error) {
	if budget, err := github.Budgets(ctx, p.repoPath); err == nil {
		p.mu.Lock()
		p.budget = budget.GraphQL
		p.mu.Unlock()

		if budget.GraphQL.Limit > 0 && budget.GraphQL.Remaining == 0 {
			err := fmt.Errorf("%w: resets %s", ErrRateLimited, budget.GraphQL.Reset.Format(time.Kitchen))
			p.setErr(err)

			return p.Last(), err
		}
	}

	status, err := p.provider.StatusFor(ctx, p.Bookmark())
	if err != nil {
		p.setErr(err)

		return p.Last(), err
	}

	status.FetchedAt = p.now()
	p.store(status)

	return status, nil
}

// Interval reports how long to wait before the next Fetch, and whether one
// should be scheduled: an unfocused pane pauses polling entirely.
//
//nolint:unparam // the duration feeds internal/components/forgepane's poll loop; this package's tests only check ok
func (p *Poller) Interval() (time.Duration, bool) {
	p.mu.Lock()
	focused, budget := p.focused, p.budget
	p.mu.Unlock()

	if !focused {
		return 0, false
	}

	return clampInterval(budget), true
}

func clampInterval(a github.Allowance) time.Duration {
	if a.Limit == 0 {
		return MinInterval
	}

	switch frac := float64(a.Remaining) / float64(a.Limit); {
	case frac <= lowBudgetFraction:
		return MaxInterval
	case frac <= midBudgetFraction:
		return (MinInterval + MaxInterval) / midIntervalDivisor
	default:
		return MinInterval
	}
}

// Last returns the most recently known Status, from a live fetch or from the
// cache seeded at construction, or nil when neither exists yet.
func (p *Poller) Last() *Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.lastStatus
}

// LastErr returns the error from the most recent Fetch, or nil after a fetch
// that succeeded.
func (p *Poller) LastErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.lastErr
}

// FetchedLive reports whether a Fetch has completed successfully this
// session, as opposed to a Status seeded only from the cache.
func (p *Poller) FetchedLive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.fetchedOnce
}

// Bookmark returns the bookmark this poller is watching.
func (p *Poller) Bookmark() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.bookmark
}

// SetBookmark points the poller at a different bookmark, seeding its cached
// Status the same way NewPoller does.
func (p *Poller) SetBookmark(bookmark string) {
	p.mu.Lock()
	p.bookmark = bookmark
	p.fetchedOnce = false
	p.mu.Unlock()

	status, ok := cache.PersistedUsing(p.disk, p.memCache, p.remoteID, p.cacheKey(), vcs.Stamp(p.repoPath))

	p.mu.Lock()
	if ok {
		p.lastStatus = status
	} else {
		p.lastStatus = nil
	}
	p.lastErr = nil
	p.mu.Unlock()
}

// SetFocused records whether the pane this poller backs is the one the user
// is looking at.
func (p *Poller) SetFocused(focused bool) {
	p.mu.Lock()
	p.focused = focused
	p.mu.Unlock()
}

// Focused reports the poller's current focus state.
func (p *Poller) Focused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.focused
}

func (p *Poller) store(status *Status) {
	p.mu.Lock()
	p.lastStatus = status
	p.lastErr = nil
	p.fetchedOnce = true
	p.mu.Unlock()

	cache.PersistUsing(p.disk, p.memCache, p.remoteID, p.cacheKey(), vcs.Stamp(p.repoPath), status)
}

func (p *Poller) setErr(err error) {
	p.mu.Lock()
	p.lastErr = err
	p.mu.Unlock()
}

func (p *Poller) cacheKey() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.provider.Name() + "\x00" + p.bookmark
}
