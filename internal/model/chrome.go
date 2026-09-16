package model

import (
	"github.com/kyleking/aragonite/tui/keyhint"

	"github.com/kyleking/jojo/internal/components/help"
	"github.com/kyleking/jojo/internal/components/statusbar"
)

// footerHints ends on help, because the footer drops hints from the right and
// keeps the last one whatever else goes.
func footerHints() []keyhint.Hint {
	return append(help.Hints(help.L0, help.L2), keyhint.Hint{Key: "?", What: "help"})
}

func (m Model) footer() string {
	return m.status.View(m.width, statusbar.Context{
		Bookmarks: m.nearbyBookmarks(),
		Revset:    m.revset,
		Message:   m.message,
		Hints:     footerHints(),
	})
}

// nearbyBookmarks reports the bookmarks in the log rather than a single current
// branch, because jj has no current branch to report.
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
