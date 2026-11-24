// Package help is jojo's keymap and the `?` overlay that draws it. The keymap is aragonite's shared
// vocabulary (tui/keys: Global, DiffView, the picker flags) merged with jojo's own jj verbs, so the
// dispatcher, the legend, the footers, the palette, and the which-key panel a waiting prefix shows
// all read one table and cannot disagree about what a key does.
package help

import (
	"slices"

	"github.com/kyleking/aragonite/tui/keys"
)

// The screens jojo adds to the shared ones (keys.Log, keys.Picker, keys.View, keys.List). A key is
// live on a screen when its binding's contexts overlap the one the model reports, so each mode of the
// log that answers keys of its own is a bit here.
const (
	// Travel is the log as it stood at an earlier operation, which is read-only.
	Travel keys.Context = 1 << (iota + 8) //nolint:mnd // the first bit aragonite leaves free
	// Outline is the stack outline, which takes the log's place while it is open.
	Outline
	// PickRebase, PickSquash, PickDuplicate, PickNew, and PickRestore are the in-log picker for
	// each verb. A verb's flags live on its own bit, so rebase's -r and restore's -i never meet.
	PickRebase
	PickSquash
	PickDuplicate
	PickNew
	PickRestore
	// Submit is a stack submit's plan, which takes the outline's place until Enter runs it or Esc
	// backs out.
	Submit
)

// PickBookmark is the in-log picker moving a bookmark (b s, b x). Aragonite's sixteen context bits
// are all spoken for, eight by the family and eight by the screens above, so it borrows Select: a
// screen choosing hunks inside a diff, which jojo hands to jj-diff and never reports itself.
// The help's sharedContexts keeps the diff vocabulary off it, so the borrowed bit answers only the picker's keys.
const PickBookmark = keys.Select

// PickVerbs maps each picker screen to the aragonite verb whose flags it offers.
//
//nolint:gochecknoglobals // a fixed table, read only.
var PickVerbs = map[keys.Context]string{
	PickRebase:    keys.VerbRebase,
	PickSquash:    keys.VerbSquash,
	PickDuplicate: keys.VerbDuplicate,
	PickNew:       keys.VerbNew,
	PickRestore:   keys.VerbRestore,
}

// keyEnter is the key every screen's own "do it" sits on.
const keyEnter = "enter"

// pickScreens is every picker screen.
const pickScreens = PickRebase | PickSquash | PickDuplicate | PickNew | PickRestore | PickBookmark

// Actions jojo adds to the shared vocabulary. Shared actions keep aragonite's names (keys.Action*),
// so a [keys] rebind written for jj-diff or second-look means the same thing here.
const (
	ActionDiff          = "diff"
	ActionPreview       = "preview"
	ActionFiles         = "files"
	ActionCompare       = "compare"
	ActionNew           = "new"
	ActionNewPlaced     = "new-placed"
	ActionEdit          = "edit"
	ActionDiffedit      = "diffedit"
	ActionCommit        = "commit"
	ActionSquash        = "squash"
	ActionSplit         = "split"
	ActionDuplicate     = "duplicate"
	ActionRebase        = "rebase"
	ActionRebaseSubtree = "rebase-subtree"
	ActionRestore       = "restore"
	ActionAbandon       = "abandon"
	ActionAbsorb        = "absorb"
	ActionResolve       = "resolve"
	ActionConverge      = "converge"
	ActionArrange       = "arrange"
	ActionOpLog         = "op-log"
	ActionOpOlder       = "op-older"
	ActionOpNewer       = "op-newer"
	ActionFetch         = "fetch"
	// ActionSync is a fetch that reports what moved and offers to restack onto a moved trunk, and
	// ActionSyncTracked the same fetch of tracked bookmarks only.
	ActionSync        = "sync"
	ActionSyncTracked = "sync-tracked"
	// ActionTrunk jumps to trunk(), in the log or as a picker's destination.
	ActionTrunk = "trunk"
	// ActionBookmarkNext and ActionBookmarkPrev step to the next or previous bookmarked revision.
	ActionBookmarkNext   = "bookmark-next"
	ActionBookmarkPrev   = "bookmark-prev"
	ActionPush           = "push"
	ActionBookmarks      = "bookmarks"
	ActionBookmarkCreate = "bookmark-create"
	// The b menu's verbs on the revision under the cursor.
	ActionBookmarkSet     = "bookmark-set"
	ActionBookmarkAdvance = "bookmark-advance"
	ActionBookmarkRename  = "bookmark-rename"
	ActionBookmarkDelete  = "bookmark-delete"
	ActionBookmarkForget  = "bookmark-forget"
	ActionBookmarkTrack   = "bookmark-track"
	ActionBookmarkResolve = "bookmark-resolve"
	// ActionBookmarkBackwards is the bookmark picker's -B, jj's --allow-backwards, spelled as a flag
	// action so it reads like every other picker's.
	ActionBookmarkBackwards = "bookmark-flag-B"
	ActionRevset            = "revset"
	ActionCheck             = "check"
	ActionParallelize       = "parallelize"
	ActionBisect            = "bisect"
	ActionWorkingCopy       = "working-copy"
	ActionForge             = "forge"
	ActionWorkspaces        = "workspaces"

	ActionPickApply   = "pick-apply"
	ActionPickClear   = "pick-clear"
	ActionPickCompare = "pick-compare"

	ActionTravelRestore = "travel-restore"
	ActionChapterRead   = "chapter-read"

	// ActionSubmit submits the outline's stack as stacked pull requests, through a plan screen.
	ActionSubmit = "submit"
	// ActionSubmitStack is ActionSubmit from the log, opening the outline at the cursor first.
	ActionSubmitStack = "submit-stack"
	// ActionSubmitRun runs the plan on screen.
	ActionSubmitRun = "submit-run"
	// ActionSubmitDraft toggles whether new pull requests open as drafts, spelled as a flag.
	ActionSubmitDraft = "submit-flag-d"

	ActionFileOpen    = "file-open"
	ActionFileRestore = "file-restore"
	ActionFileSquash  = "file-squash"
	ActionFileResolve = "file-resolve"
	ActionFileStage   = "file-stage"
	ActionFileCommit  = "file-commit"

	// ActionHistory narrows the log to the changes that touched the file under the cursor.
	ActionHistory = "history"
)

// Legend groups jojo files its own verbs under, beside aragonite's.
const (
	GroupLook   = "Look"
	GroupWrite  = "Rewrite"
	GroupPlace  = "Pick where it goes"
	GroupRepo   = "Repository"
	GroupPicker = "Picking"
	GroupTravel = "Time travel"
	GroupStack  = "Outline"
	GroupSubmit = "Submit"
	GroupFiles  = "Files"
)

// unsupported is the shared vocabulary jojo has no concept for yet. Each is left out of the keymap
// rather than drawn dead, and TestKeymap_AgreesWithShared keeps its keys free for when it lands.
//
//nolint:gochecknoglobals // a fixed table, read only.
var unsupported = []string{
	keys.ActionOpenFile, keys.ActionDefinition, keys.ActionReferences, keys.ActionCalls,
	keys.ActionPageDown, keys.ActionPageUp, keys.ActionScrollDown, keys.ActionScrollUp,
	keys.ActionSideOld, keys.ActionSideNew, keys.ActionFilter,
	keys.ActionVisual, keys.ActionWhitespace, keys.ActionCosmetic, keys.ActionMoreContext,
	keys.ActionLessContext,
	keys.ActionFoldToggle, keys.ActionFoldInvert, keys.ActionFoldOpenAll, keys.ActionFoldAll,
	keys.ActionFrameMiddle, keys.ActionFrameTop, keys.ActionFrameBottom,
	keys.ActionViewWrap, keys.ActionViewNumbers, keys.ActionViewRefine, keys.ActionViewGrammar,
	keys.ActionViewImages, keys.ActionViewCycle,
}

// objects are the ] and [ objects jojo steps: hunks and files in a diff, chapters of the outline,
// rounds (versions of the change), which the log steps too, and unread hunks, which the log and the
// outline step as revisions with something left to read.
//
//nolint:gochecknoglobals // a fixed table, read only.
var objects = []string{
	keys.ObjHunk, keys.ObjFile, keys.ObjChapter, keys.ObjUnread, keys.ObjRound, keys.ObjConflict,
}

// pickerOnly are the shared bindings the picker answers: moving the destination, help, quitting,
// and backing out. The rest would act on the log under it, so they are taken off its screens.
//
//nolint:gochecknoglobals // a fixed table, read only.
var pickerOnly = []string{
	keys.ActionHelp, keys.ActionQuit, keys.ActionBack, keys.ActionCancel, keys.ActionLineDown,
	keys.ActionLineUp, keys.ActionHalfPageDown, keys.ActionHalfPageUp, keys.ActionTop, keys.ActionBottom,
}

// travelOnly is what the past answers besides its own keys: the picker's, since every verb would
// act on the present.
//
//nolint:gochecknoglobals // a fixed table, read only.
var travelOnly = pickerOnly

// outlineOnly is what the outline leaves to the application besides its own keys.
//
//nolint:gochecknoglobals // a fixed table, read only.
var outlineOnly = append(slices.Clone(pickerOnly), keys.ActionPalette, keys.ActionSearch,
	keys.ActionSwitchPane, keys.ActionMessages)

// submitOnly is what the plan screen answers besides its own keys: the picker's, and the trail, where
// what a submit ran is listed.
//
//nolint:gochecknoglobals // a fixed table, read only.
var submitOnly = append(slices.Clone(pickerOnly), keys.ActionMessages)

// viewOnly is the shared General keys the docked diff answers.
//
//nolint:gochecknoglobals // a fixed table, read only.
var viewOnly = append(slices.Clone(pickerOnly), keys.ActionSwitchPane, keys.ActionAgain, keys.ActionReverse,
	keys.ActionMessages)

// pickerFlags are the flags jojo's picker builds a command from, per verb. -m opens a one-line prompt
// in the footer, and squash's -o, -A, and -B squash into a new commit placed there.
//
//nolint:gochecknoglobals // a fixed table, read only.
var pickerFlags = map[string][]string{
	keys.VerbRebase:    {"r", "s", "b", "o", "A", "B", "e", "d", "p"},
	keys.VerbSquash:    {"t", "o", "A", "B", "u", "m", "k", "i"},
	keys.VerbDuplicate: {"o", "A", "B"},
	keys.VerbNew:       {"o", "A", "B", "n", "m"},
	keys.VerbRestore:   {"i", "d"},
}

// PickerFlags is the flags of verb the picker offers, in aragonite's order.
func PickerFlags(verb string) []keys.Flag {
	var out []keys.Flag

	for _, f := range keys.Flags(verb) {
		if slices.Contains(pickerFlags[verb], f.Letter) {
			out = append(out, f)
		}
	}

	return out
}

// sharedBindings is the slice of aragonite's table jojo uses, each binding narrowed or widened to the
// screens where jojo acts on it.
func sharedBindings() []keys.Binding {
	drop := slices.Clone(unsupported)

	for _, o := range keys.Objects() {
		if !slices.Contains(objects, o.Noun) {
			drop = append(drop, keys.NextAction(o.Noun), keys.PrevAction(o.Noun))
		}
	}

	shared := keys.Without(slices.Concat(keys.Global(), keys.DiffView()), drop...)

	for i := range shared {
		shared[i].Contexts = sharedContexts(shared[i])
	}

	for _, ctx := range []keys.Context{PickRebase, PickSquash, PickDuplicate, PickNew, PickRestore} {
		verb := PickVerbs[ctx]

		for _, f := range keys.FlagBindings(verb) {
			if slices.Contains(pickerFlags[verb], f.Keys[0][len("- "):]) {
				f.Contexts = ctx
				shared = append(shared, f)
			}
		}
	}

	return shared
}

// everywhere is a zero context spelled out, so a screen can be taken off it.
const everywhere = ^keys.Context(0)

// sharedContexts is where jojo answers a shared binding: the log and the diff for most, with the
// modes of the log taking only what they handle.
func sharedContexts(b keys.Binding) keys.Context {
	ctx := b.Contexts
	if ctx == 0 {
		ctx = everywhere
	}

	switch b.Action {
	case keys.ActionRounds, keys.NextAction(keys.ObjRound), keys.PrevAction(keys.ObjRound):
		ctx |= keys.Log | Outline
	case keys.ActionOutline:
		ctx = keys.Log | Outline
	case keys.NextAction(keys.ObjUnread), keys.PrevAction(keys.ObjUnread), keys.ActionOnlyUnread:
		// The log and the outline step revisions with unread hunks, and U filters the dock beside them.
		ctx |= keys.Log | Outline
	case keys.ActionRead:
		// Space in the outline marks the whole chapter; in the log it checks the revision.
		ctx |= Outline
	case keys.ActionAgain, keys.ActionReverse:
		// The repeats step a diff's motions; nowhere else in jojo has one to repeat.
		return keys.Diff &^ PickBookmark
	case keys.NextAction(keys.ObjHunk), keys.PrevAction(keys.ObjHunk):
		ctx |= Outline
	case keys.NextAction(keys.ObjChapter), keys.PrevAction(keys.ObjChapter):
		// In the log and its pickers a chapter is the graph's edge: ]] a child, [[ the first parent.
		return keys.Diff&^PickBookmark | Outline | keys.Log | keys.Picker | pickScreens
	case keys.ActionLabelJump:
		// Labels go on the log's rows, which a picker walks too.
		return keys.Log | keys.Picker | pickScreens
	case keys.NextAction(keys.ObjConflict), keys.PrevAction(keys.ObjConflict):
		// jojo's diffs draw no conflict regions of their own (x hands them to jj-diff), so ]x steps the
		// log's conflicted revisions.
		return keys.Log
	}

	for _, only := range []struct {
		actions []string
		screen  keys.Context
	}{
		{pickerOnly, keys.Picker | pickScreens},
		{travelOnly, Travel},
		{outlineOnly, Outline},
		{submitOnly, Submit},
		{viewOnly, keys.View},
		{pickerOnly, keys.List},
	} {
		if b.Contexts == 0 && !slices.Contains(only.actions, b.Action) {
			ctx &^= only.screen
		}
	}

	// The bookmark picker's bit is borrowed from Select (see PickBookmark), which every diff binding
	// names, so only the picker's own keys keep it.
	if !slices.Contains(pickerOnly, b.Action) {
		ctx &^= PickBookmark
	}

	return ctx
}

// toolBindings is what jojo adds: the jj verbs on the log, the picker's own keys, time travel, the
// outline, and the file list.
func toolBindings() []keys.Binding {
	return slices.Concat(logBindings(), modeBindings(), fileBindings())
}

func logBindings() []keys.Binding {
	b := func(action, group, what, short string, seqs ...string) keys.Binding {
		return keys.Binding{Action: action, Keys: seqs, What: what, Short: short, Group: group, Contexts: keys.Log}
	}

	return []keys.Binding{
		b(ActionDiff, GroupLook, "diff the revision", "diff", keyEnter),
		withContexts(b(ActionPreview, GroupLook, "dock the diff beside the log, following the cursor", "dock", "p"),
			keys.Log|Outline|keys.View),
		b(ActionFiles, GroupLook, "files in the revision", "files", "l"),
		b(ActionCompare, GroupLook, "diff against another revision", "compare", "="),
		b(ActionNew, GroupWrite, "new on the revision (two or more checked: a merge)", "new", "n"),
		b(ActionNewPlaced, GroupPlace, "new, choosing where it goes", "new…", "N"),
		b(ActionEdit, GroupWrite, "edit", "edit", "e"),
		b(ActionDiffedit, GroupWrite, "edit the revision's content in the diff editor", "diffedit", "E"),
		b(keys.ActionDescribe, GroupWrite, "describe", "describe", "d"),
		b(keys.ActionDescribeEditor, GroupWrite, "describe in $EDITOR", "", "D"),
		b(ActionCommit, GroupWrite, "commit @: describe it and start a new one", "commit", "c"),
		b(ActionSquash, GroupPlace, "squash, choosing where into", "squash", "s"),
		b(ActionSplit, GroupWrite, "split into commits, one column each, in jj-diff", "split", "S"),
		b(ActionDuplicate, GroupPlace, "duplicate, choosing where (enter: in place)", "duplicate", "y"),
		b(ActionRebase, GroupPlace, "rebase, choosing where", "rebase", "r"),
		b(ActionRebaseSubtree, GroupPlace, "rebase with descendants, choosing where", "", []string{}...),
		b(ActionRestore, GroupPlace, "restore content from another revision (the parent: discard)", "restore", "R"),
		b(ActionAbandon, GroupWrite, "abandon", "abandon", "a"),
		b(ActionAbsorb, GroupWrite, "absorb into the revisions that last touched each line", "absorb", "A"),
		b(ActionResolve, GroupWrite, "resolve the conflict, side by side in jj-diff", "resolve", "x"),
		b(ActionConverge, GroupWrite, "converge divergent changes", "converge", "X"),
		b(ActionArrange, GroupWrite, "arrange the graph", "", []string{}...),
		b(ActionParallelize, GroupWrite, "parallelize the checked revisions", "", "|"),
		b(ActionCheck, GroupWrite, "check for a batch action", "check", "space"),
		b(ActionBisect, GroupWrite, "bisect between the two checked revisions", "", "B"),
		withContexts(b(ActionOpLog, GroupRepo, "op log, with the log as it stood at each operation", "ops", "o"),
			keys.Log|Travel),
		withContexts(b(ActionOpOlder, GroupRepo, "the log one operation back, read-only", "older", "[ o"),
			keys.Log|Travel),
		withContexts(b(ActionOpNewer, GroupRepo, "the log one operation forward", "newer", "] o"), keys.Log|Travel),
		b(ActionFetch, GroupRepo, "fetch", "fetch", "f"),
		b(ActionSync, GroupRepo, "sync: fetch, say what moved, offer to restack onto trunk", "sync", "F"),
		b(ActionSyncTracked, GroupRepo, "sync only the tracked bookmarks", "", []string{}...),
		withContexts(b(ActionTrunk, keys.GroupMove, "jump to trunk()", "trunk", "g t"),
			keys.Log|keys.Picker|pickScreens),
		withContexts(b(ActionBookmarkNext, keys.GroupMotion, "next bookmarked revision", "bookmark", "] b"),
			keys.Log|keys.Picker|pickScreens),
		withContexts(b(ActionBookmarkPrev, keys.GroupMotion, "previous bookmarked revision", "", "[ b"),
			keys.Log|keys.Picker|pickScreens),
		b(ActionPush, GroupRepo, "push the revision's bookmarks", "push", "P"),
		b(ActionBookmarks, GroupRepo, "bookmarks: the list", "bookmarks", "b b"),
		b(ActionBookmarkCreate, GroupRepo, "create a bookmark here", "", "b c"),
		b(ActionBookmarkSet, GroupRepo, "set: move a bookmark here, previewed (-B: backwards)", "", "b s"),
		b(ActionBookmarkAdvance, GroupRepo, "advance: tug the nearest bookmark below up to here", "", "b a"),
		b(ActionBookmarkRename, GroupRepo, "rename a bookmark here", "", "b r"),
		b(ActionBookmarkDelete, GroupRepo, "delete a bookmark here (the remote's on the next push)", "", "b d"),
		b(ActionBookmarkForget, GroupRepo, "forget a bookmark here (nothing to push)", "", "b f"),
		b(ActionBookmarkTrack, GroupRepo, "track or untrack a remote bookmark here", "", "b t"),
		b(ActionBookmarkResolve, GroupRepo, "resolve a conflicted bookmark: pick where it goes", "", "b x"),
		b(ActionRevset, GroupRepo, "edit the revset", "revset", "L"),
		withContexts(b(ActionWorkingCopy, keys.GroupMove, "jump to the working copy", "@", "@"),
			keys.Log|keys.Picker|pickScreens),
		b(ActionForge, GroupRepo, "forge pane: pull requests and CI", "forge", "g h"),
		b(ActionWorkspaces, GroupRepo, "workspaces", "", "g w"),
		b(ActionSubmitStack, GroupRepo, "submit the stack at the cursor as stacked pull requests", "", []string{}...),
	}
}

func withContexts(b keys.Binding, ctx keys.Context) keys.Binding {
	b.Contexts = ctx

	return b
}

func modeBindings() []keys.Binding {
	return []keys.Binding{
		{
			Action: ActionPickApply, Keys: []string{keyEnter}, What: "run it", Group: GroupPicker,
			Contexts: keys.Picker | pickScreens,
		},
		{
			Action: ActionBookmarkBackwards, Keys: []string{"- B"},
			What: "allow moving it backwards or sideways (--allow-backwards)", Short: "--allow-backwards",
			Group: "Flags", Contexts: PickBookmark,
		},
		{
			Action: ActionPickClear, Keys: []string{"backspace"}, What: "back to jj's default destination",
			Short: "default", Group: GroupPicker, Contexts: PickDuplicate,
		},
		{
			Action: ActionPickCompare, Keys: []string{"p"}, What: "show the graph before / after the command",
			Short: "before", Group: GroupPicker, Contexts: keys.Picker | pickScreens,
		},
		{
			Action: ActionTravelRestore, Keys: []string{keyEnter}, What: "restore the repository to this operation",
			Short: "restore", Group: GroupTravel, Contexts: Travel,
		},
		{
			Action: ActionChapterRead, Keys: []string{keyEnter}, What: "read the chapter in the dock",
			Short: "dock", Group: GroupStack, Contexts: Outline,
		},
		{
			Action: ActionSubmit, Keys: []string{"P"}, What: "submit the stack as stacked pull requests (a plan first)",
			Short: "submit", Group: GroupStack, Contexts: Outline,
		},
		{
			Action: ActionSubmitRun, Keys: []string{keyEnter},
			What: "run the plan: push, then open and edit the pull requests", Short: "run", Group: GroupSubmit,
			Contexts: Submit,
		},
		{
			Action: ActionSubmitDraft, Keys: []string{"- d"}, What: "new pull requests open as drafts (--draft)",
			Short: "--draft", Group: "Flags", Contexts: Submit,
		},
	}
}

func fileBindings() []keys.Binding {
	f := func(action, what, short, seq string) keys.Binding {
		return keys.Binding{
			Action: action, Keys: []string{seq}, What: what, Short: short, Group: GroupFiles,
			Contexts: keys.List,
		}
	}

	return []keys.Binding{
		f(ActionFileOpen, "diff the file", "diff", keyEnter),
		f(ActionFileRestore, "restore (discard) the file", "restore", "R"),
		f(ActionFileSquash, "squash the file into…", "squash", "s"),
		f(ActionFileResolve, "resolve the file's conflict", "resolve", "x"),
		f(ActionFileStage, "stage or unstage the file (in @)", "stage", "space"),
		f(ActionFileCommit, "commit what is staged (in @)", "commit", "c"),
		{
			Action: ActionHistory, Keys: []string{"*"}, What: "history: the changes that touched the file",
			Short: "history", Group: GroupFiles, Contexts: keys.List | keys.View | keys.Log,
		},
	}
}

// Shared is every binding of aragonite's table jojo's own must agree with, unnarrowed:
// TestKeymap_AgreesWithShared checks jojo's verbs against it.
func Shared() []keys.Binding {
	out := slices.Concat(keys.Global(), keys.DiffView())
	for _, verb := range PickVerbs {
		out = append(out, keys.FlagBindings(verb)...)
	}

	return out
}

// Tool is jojo's own bindings, for TestKeymap_AgreesWithShared.
func Tool() []keys.Binding { return toolBindings() }
