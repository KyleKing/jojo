package model

import (
	"errors"
	"fmt"

	"github.com/kyleking/aragonite/seen"

	"github.com/kyleking/jojo/internal/jj"
)

// errNoScope is a mark asked of a change whose full id no log or outline read has named yet.
var errNoScope = errors.New("this change's full id is not known yet: ctrl+l reads the log again")

// scopeBook maps the change id the screen shows, jj's shortest unique prefix, to the full change id
// read marks are filed under. The short id grows once another change shares its prefix, and a mark
// filed under it would then vanish; the full id is the change's for life. Held by pointer, so the
// copies Update makes of the model share it.
type scopeBook struct {
	full map[string]string
	// moved is every full id whose marks have been looked for under an older, shorter scope.
	moved map[string]bool
	// dir is the store's directory, "" when marks are kept nowhere, which has nothing to migrate.
	dir string
}

func newScopeBook(reads ReadState) *scopeBook {
	b := &scopeBook{full: map[string]string{}, moved: map[string]bool{}}
	if d, ok := reads.(interface{ Dir() string }); ok {
		b.dir = d.Dir()
	}

	return b
}

// of is change's full id, "" while it is not known.
func (b *scopeBook) of(change string) string { return b.full[change] }

// learn files each revision's full id under the id the screen shows it by, and the first time a
// full id is seen moves marks an older jojo filed under a prefix of it (aragonite's seen.Migrate). It
// answers the full ids whose marks moved, so the caller can drop what the store read for them, and
// the first failure to move.
func (b *scopeBook) learn(revs []*jj.Revision) ([]string, error) {
	var (
		moved []string
		errs  error
	)

	for _, rev := range revs {
		if rev == nil || rev.FullChangeID == "" {
			continue
		}

		b.full[rev.ChangeID] = rev.FullChangeID

		if b.dir == "" || b.moved[rev.FullChangeID] {
			continue
		}

		b.moved[rev.FullChangeID] = true

		did, err := seen.Migrate(b.dir, rev.FullChangeID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("moving %s's read marks to its full id: %w", rev.ChangeID, err))
		}

		if did {
			moved = append(moved, rev.FullChangeID)
		}
	}

	return moved, errs
}

// unread reports whether a hunk of change is unread, every hunk of a change whose full id is not
// known yet answering yes.
func (m Model) unread(change, hunk string) bool {
	scope := m.scopes.of(change)
	if scope == "" {
		return true
	}

	return m.reads.Unread(scope, hunk)
}

// learnScopes is learn over revs, dropping what the store holds for a scope whose marks moved and
// saying when a move failed.
func (m *Model) learnScopes(revs []*jj.Revision) {
	moved, err := m.scopes.learn(revs)

	if reloader, ok := m.reads.(ReadReloader); ok {
		for _, scope := range moved {
			reloader.Reload(scope)
		}
	}

	if err != nil {
		m.fail(err)
	}
}
