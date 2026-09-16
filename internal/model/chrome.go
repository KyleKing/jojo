package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/statusbar"
	"github.com/kyleking/jojo/internal/jj"
)

type headOpMsg struct {
	description string
}

// loadHeadOp names the operation u would reverse, refreshed after every write.
func (m Model) loadHeadOp() tea.Cmd {
	client, ctx := m.client, m.ctx

	return func() tea.Msg {
		ops, err := client.OpLog(ctx, 1)
		if err != nil || len(ops) == 0 {
			return headOpMsg{}
		}

		return headOpMsg{description: ops[0].Description}
	}
}

// footerHints ends on help, because the footer drops hints from the right and keeps the last.
func (m Model) footerHints() []keyhint.Hint {
	hints := rowHints(m.log.Selected())
	hints = append(hints, help.Hints(help.L0, help.L2)...)

	return append(hints, keyhint.Hint{Key: "?", What: "help"})
}

// rowHints is the one thing worth saying about the revision under the cursor.
func rowHints(rev *jj.Revision) []keyhint.Hint {
	switch {
	case rev == nil:
		return nil
	case rev.Conflict:
		return []keyhint.Hint{{Key: "c", What: "conflicted, resolve it"}}
	case rev.Immutable:
		return []keyhint.Hint{{Key: "e", What: "immutable, an edit asks first"}}
	case rev.Empty && !rev.WorkingCopy:
		return []keyhint.Hint{{Key: "a", What: "empty, abandon it"}}
	case len(rev.LocalBookmarks) > 0:
		return []keyhint.Hint{{Key: "b", What: "bookmarks, push from there"}}
	}

	return nil
}

func (m Model) footer() string {
	return m.status.View(m.width, statusbar.Context{
		Bookmarks: m.nearbyBookmarks(),
		Revset:    m.revset,
		Message:   m.message,
		Undo:      m.lastOp,
		Hints:     m.footerHints(),
	})
}

// nearbyBookmarks reports the bookmarks in the log rather than a single current branch, because jj
// has no current branch to report.
func (m Model) nearbyBookmarks() []statusbar.Bookmark {
	var out []statusbar.Bookmark

	for _, row := range m.rows {
		if row.Revision == nil {
			continue
		}

		for _, name := range row.Revision.Bookmarks {
			out = append(out, statusbar.Bookmark{Name: name, AtWorkingCopy: row.Revision.WorkingCopy})
		}
	}

	return out
}
