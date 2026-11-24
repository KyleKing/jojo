package help

import (
	"slices"
	"strings"

	"github.com/kyleking/aragonite/tui/keys"
	"github.com/kyleking/aragonite/tui/palette"
)

// about is what the palette teaches about an action beyond its binding: the jj command shape it
// runs, the words a search finds it by (git's among them, so a git user typing "amend" lands on
// squash and reads the jj word beside it), a sentence or two on what it does, and the actions worth
// knowing beside it. Shapes are written as shapes, not command lines; the trail echoes the exact one.
type about struct {
	cmd     string
	also    []string
	prose   string
	related []string
}

// Git words that find more than one action.
const (
	gitShow    = "show"
	gitStage   = "stage"
	gitRevert  = "revert"
	gitHistory = "history"
	gitReflog  = "reflog"
)

// abouts is jojo's teaching table, keyed by action.
//
//nolint:gochecknoglobals // a fixed table, read only.
var abouts = map[string]about{
	ActionDiff: {
		cmd: "jj diff -r <rev>", also: []string{gitShow},
		prose:   "Shows everything the revision changed, full screen.",
		related: []string{ActionPreview, ActionFiles, ActionCompare},
	},
	ActionPreview: {
		cmd: "jj diff -r <rev> --git", also: []string{gitShow, "preview"},
		prose:   "Docks the diff beside the log; it follows the cursor, and tab gives it the keys.",
		related: []string{keys.NextAction(keys.ObjHunk), keys.ActionOutline},
	},
	ActionFiles: {
		cmd: "jj diff -r <rev> --summary", also: []string{"status", "stat", gitStage, "add"},
		prose: "Lists the files the revision touches. On @ it stages and commits paths, the way " +
			"git add and git commit would.",
		related: []string{ActionSplit, ActionSquash},
	},
	ActionCompare: {
		cmd: "jj diff --from <a> --to <b>", also: []string{"range-diff", "compare"},
		prose: "Diffs the revision against another one you pick.",
	},
	ActionNew: {
		cmd: "jj new <revs>", also: []string{"stash", "checkout", "switch", "branch off", "merge"},
		prose: "Starts a new, empty change on top of the revision and makes it @, the working copy. " +
			"With two or more checked it is a merge. What @ held stays where it was, which is jj's " +
			"stash: nothing is ever lost.",
		related: []string{ActionNewPlaced, ActionCommit, ActionEdit},
	},
	ActionNewPlaced: {
		cmd: "jj new -o/-A/-B <dest>", also: []string{"insert", "checkout -b"},
		prose: "Starts a new change where you choose: onto a revision, or inserted after or before it.",
	},
	ActionEdit: {
		cmd: "jj edit <rev>", also: []string{"checkout", "switch"},
		prose: "Makes the revision @ itself, so what you change on disk rewrites it in place. n starts " +
			"a change on top instead, which is usually what you want.",
		related: []string{ActionNew},
	},
	ActionDiffedit: {
		cmd: "jj diffedit -r <rev>", also: []string{"edit hunks", "add -p"},
		prose: "Edits the revision's content in the diff editor: what you drop leaves it.",
	},
	keys.ActionDescribe: {
		cmd: "jj describe -r <rev> -m <message>", also: []string{"reword", "message"},
		prose:   "Sets the revision's message. Every change in jj is a commit already; describing names it.",
		related: []string{keys.ActionDescribeEditor, ActionCommit},
	},
	keys.ActionDescribeEditor: {
		cmd: "jj describe -r <rev>", also: []string{"reword", "message"},
		prose: "Sets the revision's message in $EDITOR.",
	},
	ActionCommit: {
		cmd: "jj commit -m <message>", also: []string{"commit"},
		prose: "Describes @ and starts a new change on top in one step: what git commit does, " +
			"since @ already holds your edits.",
		related: []string{keys.ActionDescribe, ActionNew},
	},
	ActionSquash: {
		cmd: "jj squash --from <rev> --into <dest>", also: []string{"amend", "fixup", gitStage, "move"},
		prose: "Moves the revision's changes into another one you pick, its parent by default, and " +
			"drops the revision once it is empty. -i chooses the hunks.",
		related: []string{ActionAbsorb, ActionSplit},
	},
	ActionSplit: {
		cmd: "jj-diff --split -r <rev>", also: []string{gitStage, "add -p", "commit -p"},
		prose:   "Splits the revision into commits, one column each, in jj-diff. Nothing is lost; u undoes.",
		related: []string{ActionSquash},
	},
	ActionDuplicate: {
		cmd: "jj duplicate <rev> [-o/-A/-B <dest>]", also: []string{"cherry-pick", "copy"},
		prose: "Copies the revision to where you pick; enter copies it in place.",
	},
	ActionRebase: {
		cmd: "jj rebase -r <rev> -o <dest>", also: []string{"rebase", "move", "reorder"},
		prose: "Moves the revision onto another one you pick. -s takes its descendants along, -b the " +
			"whole branch.",
		related: []string{ActionRebaseSubtree, ActionArrange},
	},
	ActionRebaseSubtree: {
		cmd: "jj rebase -s <rev> -o <dest>", also: []string{"rebase --onto", "move stack"},
		prose: "Moves the revision and everything stacked on it.",
	},
	ActionRestore: {
		cmd: "jj restore --from <src> --into <rev>", also: []string{"reset", "discard", "checkout --", gitRevert},
		prose: "Restores the revision's content from another one; from its parent, that discards " +
			"its changes.",
		related: []string{ActionAbandon, keys.ActionUndo},
	},
	ActionAbandon: {
		cmd: "jj abandon <revs>", also: []string{"drop", "reset --hard", gitRevert, "delete"},
		prose:   "Drops the revision; its children move onto its parent. u brings it back.",
		related: []string{keys.ActionUndo, ActionRestore},
	},
	ActionAbsorb: {
		cmd: "jj absorb -r <rev>", also: []string{"fixup", "autosquash"},
		prose:   "Moves each changed line into the revision below that last touched it.",
		related: []string{ActionSquash},
	},
	ActionResolve: {
		cmd: "jj-diff --resolve -r <rev>", also: []string{"merge", "mergetool", "conflict"},
		prose: "Resolves the revision's conflict side by side in jj-diff. A conflict in jj is a " +
			"state a commit can be in, so resolving is just another rewrite.",
	},
	ActionConverge: {
		cmd: "jj converge [-r change_id(<rev>)]", also: []string{"divergent", "dedupe"},
		prose: "Settles a divergent change, one change id on two commits, by keeping one side.",
	},
	ActionArrange: {
		cmd: "jj arrange", also: []string{"rebase -i", "reorder", "interactive rebase"},
		prose: "Arranges the graph in jj's own interactive editor.",
	},
	ActionParallelize: {
		cmd: "jj parallelize <revs>", also: []string{"parallel", "siblings"},
		prose: "Makes the checked revisions siblings rather than a stack.",
	},
	ActionBisect: {
		cmd: "jj bisect run --range <a>::<b>", also: []string{"bisect"},
		prose: "Bisects between the two checked revisions.",
	},
	ActionCheck: {
		prose: "Checks the revision for a batch: the verbs that take several revisions act on every " +
			"checked one. esc clears them.",
	},
	ActionOpLog: {
		cmd: "jj op log", also: []string{gitReflog, gitHistory, "undo history"},
		prose:   "Every operation jj has done, with the log as it stood after each one. Enter restores to one.",
		related: []string{keys.ActionUndo, ActionOpOlder},
	},
	ActionOpOlder: {
		cmd: "jj log --at-op <op> --ignore-working-copy", also: []string{gitReflog, "time travel"},
		prose: "Shows the log one operation back, read-only.",
	},
	ActionOpNewer: {
		cmd: "jj log --at-op <op> --ignore-working-copy", also: []string{"time travel"},
		prose: "Shows the log one operation forward.",
	},
	ActionFetch: {
		cmd: "jj git fetch --all-remotes", also: []string{"pull", "fetch"},
		prose: "Fetches every remote. jj never merges on fetch; a bookmark that moved both ways shows " +
			"as conflicted.",
	},
	ActionPush: {
		cmd: "jj git push -r <revs>", also: []string{"push", "publish"},
		prose:   "Pushes the revision's bookmarks, or offers a new one when it has none.",
		related: []string{ActionBookmarkCreate, ActionFetch},
	},
	ActionBookmarks: {
		cmd: "jj bookmark list", also: []string{"branch", "branches", "tags"},
		prose: "Bookmarks are jj's branches, except that they do not move as you commit.",
	},
	ActionBookmarkCreate: {
		cmd: "jj bookmark create <name> -r <rev>", also: []string{"branch", "checkout -b", "tag"},
		prose: "Names the revision with a new bookmark, jj's word for a branch.",
	},
	ActionRevset: {
		cmd: "jj log -r <revset>", also: []string{"filter", "query", "log"},
		prose: "Edits which revisions the log shows, in jj's revset language; tab completes.",
	},
	ActionWorkspaces: {
		cmd: "jj workspace list", also: []string{"worktree"},
		prose: "jj's worktrees: more than one working copy of one repository.",
	},
	ActionWorkingCopy: {
		also:  []string{"head", "working copy"},
		prose: "Jumps to @, the working copy: always a commit, which jj snapshots before every command.",
	},
	keys.ActionUndo: {
		cmd: "jj undo", also: []string{gitRevert, "reset", gitReflog},
		prose: "Undoes the last operation, whatever it was. Every write is one operation, so nothing " +
			"here needs a confirmation.",
		related: []string{keys.ActionRedo, ActionOpLog},
	},
	keys.ActionRedo:   {cmd: "jj redo", prose: "Redoes what undo undid."},
	keys.ActionReload: {cmd: "jj log", also: []string{"refresh"}},
	keys.ActionRounds: {
		cmd: "jj evolog -r <rev>", also: []string{gitHistory, "versions", gitReflog},
		prose: "The versions the change passed through as it was rewritten.",
	},
	keys.ActionOutline: {
		cmd: "jj log -r 'trunk()..<rev> | <rev>:: ~ ::trunk()'", also: []string{"stack", "series"},
		prose: "The stack the revision sits in, parent first, as chapters to read.",
	},
	keys.ActionInspect:             {cmd: "jj log -r <rev> -T ...", also: []string{gitShow, "details"}},
	keys.NextAction(keys.ObjRound): {cmd: "jj interdiff --from <a> --to <b>"},
	keys.PrevAction(keys.ObjRound): {cmd: "jj interdiff --from <a> --to <b>"},
	ActionTravelRestore: {
		cmd: "jj op restore <op>", also: []string{"reset --hard", gitReflog},
		prose: "Restores the whole repository to the operation on screen. u undoes that too.",
	},
	keys.ActionMessages: {
		also:  []string{"log", gitHistory, "trail", "errors"},
		prose: "Every jj command this session ran, newest first, with its exit and stderr.",
	},
}

// actionVerbs are the picker verbs each action opens, whose flags the palette lists.
//
//nolint:gochecknoglobals // a fixed table, read only.
var actionVerbs = map[string]string{
	ActionSquash: keys.VerbSquash, ActionRebase: keys.VerbRebase, ActionRebaseSubtree: keys.VerbRebase,
	ActionDuplicate: keys.VerbDuplicate, ActionNewPlaced: keys.VerbNew, ActionRestore: keys.VerbRestore,
}

// About is what the palette teaches about each action, with related actions named by the keys km
// binds them to, so a rebind reaches the explainer too.
func About(km Keymap) map[string]palette.About {
	out := make(map[string]palette.About, len(abouts))

	for action, a := range abouts {
		entry := palette.About{Cmd: a.cmd, Also: a.also, Flags: flagSummary(actionVerbs[action])}
		if a.prose != "" {
			entry.Prose = []string{a.prose}
		}

		for _, r := range a.related {
			if label := km.Label(r); label != "" {
				b, _ := km.Binding(r)
				entry.Related = append(entry.Related, label+" "+short(b))
			}
		}

		out[action] = entry
	}

	return out
}

// Cmd is the jj shape action runs, "" for one that runs no jj command.
func Cmd(action string) string { return abouts[action].cmd }

// short is a binding's footer word.
func short(b keys.Binding) string {
	if b.Short != "" {
		return b.Short
	}

	return b.What
}

// flagSummary is a picker verb's flags as the explainer lists them: "-u keep the destination's
// message · -k keep the source if it empties".
func flagSummary(verb string) string {
	if verb == "" {
		return ""
	}

	parts := make([]string, 0, len(pickerFlags[verb]))
	for _, f := range PickerFlags(verb) {
		parts = append(parts, "-"+f.Letter+" "+f.What)
	}

	return strings.Join(parts, " · ")
}

// PaletteOnly is every log action the keymap binds no key to, which only the palette reaches.
func (k Keymap) PaletteOnly() []keys.Binding {
	var out []keys.Binding

	for _, b := range k.Keys().Bindings() {
		if b.Contexts.Overlaps(keys.Log) && len(b.Keys) == 0 && !slices.Contains(paletteSkips, b.Action) {
			out = append(out, b)
		}
	}

	return out
}
