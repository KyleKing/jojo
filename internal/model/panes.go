package model

import (
	"context"

	"github.com/kyleking/aragonite/tui/region"

	"github.com/kyleking/jojo/internal/components/forgepane"
	"github.com/kyleking/jojo/internal/jj"
	"github.com/kyleking/jojo/internal/ui"
)

// focus names the pane that receives keys the log would otherwise take.
type focus int

const (
	focusLog focus = iota
	focusRevset
	focusForge
)

// forgeRows is the height the forge pane takes when it is open.
const forgeRows = 7

// revsetRows is the height the revset box takes, before its completion list.
const revsetRows = 1

func forgeStyles(s ui.Skin) forgepane.Styles {
	return forgepane.Styles{
		Region: region.Styles{Rule: s.Muted, Label: s.Muted},
		OK:     s.Success,
		Warn:   s.Warning,
		Err:    s.Error,
		Dim:    s.Muted,
	}
}

// aliasSource lets the revset box complete the repository's own aliases,
// which is where trunk() and immutable_heads() come from.
func aliasSource(client *jj.Client) func(context.Context) ([]string, error) {
	return client.RevsetAliases
}

// bookmarkNearWorkingCopy is what the forge pane watches. Jujutsu has no
// current branch, so the bookmark on @ wins and the nearest one in the log
// stands in when @ carries none.
func bookmarkNearWorkingCopy(rows []jj.LogRow) string {
	fallback := ""

	for _, row := range rows {
		name := branchName(row.Revision)
		if name == "" {
			continue
		}

		if row.Revision.WorkingCopy {
			return name
		}

		if fallback == "" {
			fallback = name
		}
	}

	return fallback
}

// branchName is the branch a forge would know this revision by. A local
// bookmark wins, and a remote-only one still counts because a revision can be
// pushed without ever being tracked locally.
func branchName(rev *jj.Revision) string {
	switch {
	case rev == nil:
		return ""
	case len(rev.LocalBookmarks) > 0:
		return rev.LocalBookmarks[0]
	case len(rev.RemoteBookmarks) > 0:
		return rev.RemoteBookmarks[0]
	default:
		return ""
	}
}
