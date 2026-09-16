package forge

import (
	"context"
	"testing"
	"time"

	"github.com/kyleking/aragonite/cache"
	"github.com/kyleking/aragonite/forge/github"
)

type stubProvider struct{ name string }

func (s stubProvider) Name() string { return s.name }

func (stubProvider) StatusFor(_ context.Context, _ string) (*Status, error) {
	return &Status{}, nil
}

func TestClampInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		budget github.Allowance
		want   time.Duration
	}{
		{name: "no budget read yet", budget: github.Allowance{}, want: MinInterval},
		{name: "plenty of budget left", budget: github.Allowance{Limit: 5000, Remaining: 4000}, want: MinInterval},
		{name: "budget running low", budget: github.Allowance{Limit: 5000, Remaining: 200}, want: MaxInterval},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := clampInterval(tc.budget); got != tc.want {
				t.Errorf("clampInterval(%+v) = %v, want %v", tc.budget, got, tc.want)
			}
		})
	}
}

func TestPoller_Interval_PausesWhenUnfocused(t *testing.T) {
	t.Parallel()

	p := NewPoller(stubProvider{name: "test"}, t.TempDir(), "", "feature", WithFocused(false))

	if _, ok := p.Interval(); ok {
		t.Error("Interval() ok = true while unfocused, want polling paused")
	}

	p.SetFocused(true)

	if _, ok := p.Interval(); !ok {
		t.Error("Interval() ok = false after focusing, want polling resumed")
	}
}

// TestPoller_SeedsFromDiskAcrossRestarts proves the "stale-but-cached" state:
// a Poller built fresh, with no Fetch yet, renders whatever the previous
// process persisted.
func TestPoller_SeedsFromDiskAcrossRestarts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	disk := cache.NewDiskCache(dir)
	repoPath := t.TempDir()

	first := NewPoller(stubProvider{name: "test"}, repoPath, "remote-1", "feature", WithDiskCache(disk))
	if first.Last() != nil {
		t.Fatalf("Last() = %+v before any Fetch, want nil", first.Last())
	}

	seeded := &Status{Behind: true}
	first.store(seeded)

	second := NewPoller(stubProvider{name: "test"}, repoPath, "remote-1", "feature", WithDiskCache(disk))
	if second.FetchedLive() {
		t.Error("FetchedLive() = true for a poller that has not fetched, want the seeded value marked stale")
	}

	if got := second.Last(); got == nil || !got.Behind {
		t.Errorf("Last() = %+v, want the disk-seeded status", got)
	}
}
