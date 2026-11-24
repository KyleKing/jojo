package stack

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/kyleking/jojo/internal/gh"
	"github.com/kyleking/jojo/internal/jj"
)

// Reader is the reads a plan needs from jj.
type Reader interface {
	Described(ctx context.Context, changeIDs []string) (map[string]jj.Described, error)
	TrunkBranch(ctx context.Context) (string, error)
}

// Asker is the read a plan needs from the forge.
type Asker interface {
	PRFor(ctx context.Context, branch string) (*gh.PR, error)
}

// Changes is the stack's revisions, parent first, as a plan reads them, with their whole
// descriptions and what a remote already holds.
func Changes(revs []jj.Revision, described map[string]jj.Described) []Change {
	out := make([]Change, 0, len(revs))

	for k := range revs {
		r := &revs[k]
		d := described[r.ChangeID]
		subject, body, _ := strings.Cut(d.Description, "\n")

		c := Change{
			ID: r.ChangeID, LongID: d.LongID, PushName: d.PushName,
			Subject: strings.TrimSpace(subject), Body: strings.TrimSpace(body),
			Empty: r.Empty, Immutable: r.Immutable, Conflict: r.Conflict, Divergent: r.Divergent,
			WorkingCopy: r.WorkingCopy,
		}
		if c.LongID == "" {
			c.LongID = r.ChangeID
		}

		if c.Subject == "" {
			c.Subject = strings.TrimSpace(r.Description)
		}

		if len(r.LocalBookmarks) > 0 {
			c.Bookmark = r.LocalBookmarks[0]
			c.Pushed = slices.Contains(d.Pushed, c.Bookmark)
		}

		out = append(out, c)
	}

	return out
}

// Read gathers what Build needs for revs: their descriptions, trunk's branch, and the pull request
// of each bookmark the stack has or would be given. It writes nothing.
func Read(
	ctx context.Context, revs []jj.Revision, jjr Reader, forge Asker, template string, draft bool,
) (Input, error) {
	ids := make([]string, 0, len(revs))
	for k := range revs {
		r := &revs[k]
		ids = append(ids, r.ChangeID)
	}

	described, err := jjr.Described(ctx, ids)
	if err != nil {
		return Input{}, fmt.Errorf("reading the stack's descriptions: %w", err)
	}

	trunk, err := jjr.TrunkBranch(ctx)
	if err != nil {
		return Input{}, fmt.Errorf("finding trunk's branch: %w", err)
	}

	changes, skipped := Trim(Changes(revs, described))
	in := Input{
		Changes: changes, Skipped: skipped, Trunk: trunk, Template: template, Draft: draft, PRs: map[string]*gh.PR{},
	}

	for _, c := range changes {
		name := c.Bookmark
		if name == "" {
			name = BookmarkName(template, c)
		}

		pr, err := forge.PRFor(ctx, name)
		if err != nil {
			return Input{}, fmt.Errorf("reading %s's pull request: %w", name, err)
		}

		if pr != nil {
			in.PRs[name] = pr
		}
	}

	return in, nil
}
