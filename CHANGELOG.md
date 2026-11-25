## v0.29.0 (2026-11-25)

### Feat

- detect Nerd-capable terminals for icons (aragonite v1.35.0)

## v0.28.3 (2026-11-25)

### Fix

- **deps**: aragonite v1.34.2 (no detached git maintenance in test homes)

## v0.28.2 (2026-11-25)

### Fix

- **launch**: start through aragonite tui/launch, and ctrl+c leaves during the wait

## v0.28.1 (2026-11-25)

### Fix

- **launch**: the first key of a session acts instead of vanishing

## v0.28.0 (2026-11-25)

### BREAKING CHANGE

- the staging vocabulary is gone. The file list of @ no
longer lists @-'s files as staged, space no longer stages or unstages,
and c commits the checked files of @ rather than describing @- and
inserting a new index. A [keys] rebind of file-stage lands on check with
a footer warning saying staging is retired.

### Feat

- **keys**: space checks files in the file list, and staging is retired

## v0.27.3 (2026-11-25)

### Fix

- **deps**: update to my_go_template v0.24.3

## v0.27.2 (2026-11-25)

### Fix

- **jjdiff**: read jj-diff v0.15.2's outcome line, and require that release
- **picker**: say in the picker's own status line why enter did nothing

## v0.27.1 (2026-11-25)

### Fix

- **rounds**: size a squash against the change's own previous version, and say when a rebase conflicted

## v0.27.0 (2026-11-25)

### Feat

- **flows**: describe from the outline, a split asks to describe what it made, and refusals land where they point

## v0.26.4 (2026-11-25)

### Fix

- **deps**: aragonite v1.32.0, whose welcome card only closes on a verb and whose framecheck reads jj's graph

## v0.26.3 (2026-11-25)

### Perf

- **jj**: one snapshot point, a diff cache by commit id, and nothing slow before the first frame

## v0.26.2 (2026-11-25)

### Perf

- **model**: hold the skin and the big fields by pointer, so a keypress stops copying 700 KB

## v0.26.1 (2026-11-24)

### Fix

- **deps**: aragonite v1.31.1, whose hermetic unsets git's repository variables

## v0.26.0 (2026-11-24)

### Feat

- **log**: . repeats a description and the b verbs; ' and the jump list in the file list and op log
- **coach**: a welcome card on the first launch
- **log**: * shows a file's history, following its renames

### Fix

- **test**: await the repaired page after a resize instead of sleeping 300ms

## v0.25.0 (2026-11-24)

### Feat

- **log**: ' labels the rows on screen to jump to; ctrl+o and ctrl+i walk the jump list

## v0.24.0 (2026-11-24)

### Feat

- **picker**: -m writes the message in a footer prompt; squash -o/-A/-B into a new commit

## v0.23.0 (2026-11-24)

### Feat

- **log**: ]x ]b ]] motions over the log, and . repeats the last write on the cursor

## v0.22.0 (2026-11-24)

### Feat

- **sync**: F fetches and reports what moved, offering a restack onto trunk; gt jumps to trunk

## v0.21.0 (2026-11-24)

### Feat

- **bookmarks**: b menu on the cursor: set, advance, rename, delete, forget, track, resolve

## v0.20.1 (2026-11-24)

### Refactor

- **test**: take hermetic and Migrate from aragonite v1.30.0

## v0.20.0 (2026-11-24)

### Feat

- **reads**: keep read marks in the full-screen diff too

### Fix

- **reads**: file read marks under the full change id, not the shortest prefix
- **outline**: keep 24 cells of description beside the dock, dropping the size first
- **stack**: list each gh call of a submit plan exactly as it runs, --repo included
- **test**: run every test in a home of its own and fail a run that reaches the real one

## v0.19.0 (2026-11-24)

### Feat

- **stack**: name bookmarks as jj does, point gh at the remote, poll PRs as the outline opens

## v0.18.0 (2026-11-24)

### Feat

- **reads**: keep read marks per change with aragonite/seen

## v0.17.1 (2026-11-24)

### Fix

- **color**: draw the selected row as one even bar in 16 colors (aragonite v1.28.0)

## v0.17.0 (2026-11-24)

### Feat

- **stack**: submit the outline's stack as stacked pull requests

## v0.16.2 (2026-11-24)

### Fix

- **deps**: take jj-diff v0.12.1, which lexes a repeated path per file section
- **exec**: repaint after a handed-off program on bubbletea v2.1.2

## v0.16.1 (2026-11-24)

### Fix

- **color**: keep a color's hue in 256 colors through theme.Hue (aragonite v1.25.2)

## v0.16.0 (2026-11-24)

### Feat

- **discover**: one palette for ? and :, gates that say why, a coach, and the jj trail

### Fix

- **color**: fit the skin and the renderer to one color profile on aragonite v1.25.1
- **outline**: give the description the cells metadata columns padded

### Perf

- **dock**: draw only the rows on screen, so a revision costs the loop the pane, not the diff

## v0.15.0 (2026-11-24)

### BREAKING CHANGE

- keys follow the shared keymap. In the log: g -> gg;
alt+p (dock) -> p; p (forge pane) -> gh; w (workspaces) -> gw; ! (last
failure) -> gm, and ! opens a shell; alt+d (diff two revisions) -> =
(diff the cursor against a picked revision); alt+e (describe in $EDITOR)
-> D; D (duplicate) -> y; c (resolve) -> x; x (converge) -> X; X
(arrange) -> the palette only; E (evolog panel) -> H rounds, where R
restores a version; v (details) -> K; n (new, in the picker) -> n is an
immediate jj new and N opens the picker; R (rebase with descendants) ->
r then -s, and R is restore; i (squash -i into the parent) and alt+i
(move hunks into any revision) -> s then -i; b (bookmarks panel) -> bb;
Tab no longer stops in the revset box (L focuses it). In the picker: o a
b (onto, after, before) -> -o -A -B; r s B (revs, source, branch) -> -r
-s -b; e (skip or keep emptied) -> -e (rebase) or -k (squash); m
(combine messages) -> -u off; w (no-edit) -> -n; h (duplicate in place)
-> backspace; Tab (before/after) -> p. In time travel: r (restore) ->
enter. In the op log: r -> enter only; d (show diff) -> p; v (revert) ->
u. In the file list: r (restore) -> R; m (move) -> s; c (resolve) -> x;
C (commit staged) -> c; l (close) -> h or esc. In the forge pane: r
(refresh) -> ctrl+l. In the full diff: s (side by side) -> \s.

### Feat

- **log**: open jj-diff's split columns and conflict viewer from the log
- **keys**: adopt aragonite's shared keymap for dispatch, help, footers, and prefixes

### Fix

- **test**: wait on every command in the live picker tests instead of a one-second budget

### Perf

- **dock**: color diffs through jj-diff's render seam instead of a second render off the loop

## v0.14.0 (2026-11-24)

### Feat

- **outline**: give the outline its own width beside the dock, and the dock the rest
- **outline**: name the stack by its branch and size, and put its revset in the inspector
- **log**: give the opening revset's row back to the log

### Fix

- **picker**: check that previews never wrote through a hard link into the repository
- **tui**: reset the scroll margins on a resize so a grown screen repaints in place

### Perf

- **dock**: draw the docked diff off the update loop and color it when the lexer lands

## v0.13.0 (2026-11-24)

### Feat

- **review**: stack outline, evolog rounds, and an inspector line

## v0.12.0 (2026-11-24)

### Feat

- **log**: pick destinations on a previewed log, and travel the op log

## v0.11.0 (2026-11-24)

### Feat

- **hunks**: hand hunk work to jj-diff when it is installed and jj names no editor

### Perf

- **log**: paint the graph first and read row stats for the screen only

## v0.10.0 (2026-11-24)

### Feat

- **divergence**: converge divergent changes with jj converge

### Fix

- **undo**: use jj's own redo instead of restoring an operation
- **oplog**: show an operation's diff in the pane instead of a pager
- **push**: push a new bookmark by name, and a bookmarkless revision with -c

## v0.9.2 (2026-11-24)

### Fix

- **log**: one multi-select rule for every batch verb, counted and cleared
- **describe**: keep the body and trailers, skip unchanged writes, offer $EDITOR

## v0.9.1 (2026-11-24)

### Fix

- **errors**: keep failures visible, and show the whole of one on !
- **log**: highlight the whole cursor row, not just its graph
- **rebase**: allow trunk, main@origin and the root as rebase destinations

## v0.9.0 (2026-11-24)

### Feat

- **help**: draw the key legend's groups as labeled rules

## v0.8.0 (2026-11-24)

### Feat

- **diff**: draw the diff pane with the shared wash, sign bar, and hunk labels

### Fix

- **jj**: restore, move and resolve files whose paths have spaces or parentheses

## v0.7.0 (2026-11-23)

### Feat

- render diffs with jj-diff v0.7.0

## v0.6.0 (2026-11-23)

### Feat

- title lists with labeled rules and draw files with their type's icon
- dock the diff beside the log and follow the cursor
- stage, unstage, and commit from the file list of @

## v0.5.0 (2026-11-14)

### Feat

- record CLI subcommands and flags in key usage

## v0.4.0 (2026-11-14)

### Feat

- record key usage per context and add a keys report

## v0.3.0 (2026-11-04)

### Feat

- **startup**: refuse a filtered clone jj cannot read

## v0.2.1 (2026-11-04)

### Fix

- **ci**: satisfy golangci-lint 2.13 and gate the config in the project job

## v0.2.0 (2026-11-04)

### Feat

- **log**: page deeper history when the cursor reaches the bottom
- **diff**: step between files with [ and ]

## v0.1.0 (2026-11-04)

### Feat

- **log**: mark divergent changes and abandon the stale sides
- **bookmarks**: resolve a conflicted bookmark to its remote side
- **ui**: recover a stale working copy in one keypress
- **jj**: serialize every jj invocation through one queue
- **diff**: side-by-side diffs and moving files or hunks into any revision
- **ui**: read diffs in a pane and take settings from config.toml
- **log**: close the git loop with fetch and push, and browse a revision's files
- **ui**: detail overlay, teaching hints, and a faster start
- **ui**: report the jj command each action ran
- **log**: diff from the log, parallelize, bisect, arrange, and act on operations
- **bookmarks**: manage bookmarks, push behind a gate, and offer the immutable escape
- **log**: search the log with / and run any action from :
- **jj**: read bookmarks and add push, parallelize, revert, and the immutable escape hatch
- **evolog**: browse how a change evolved, with interdiff and restore
- **workspace**: list, add, and forget workspaces from a panel
- **oplog**: browse jj's operation log and restore from it
- **jj**: read the evolution timeline, the workspace list, and restore
- **log**: bind describe, squash, and both rebase scopes through the modals
- **ui**: add a describe prompt and a destination picker
- **log**: bind the per-commit jj verbs, each reversible with an undo hint
- **forge**: show pull request and CI state for the bookmark near @
- **revset**: filter the log live from an editable revset box
- **log**: put change id, bookmarks, description, and revision size on the row
- **ui**: theme, footer, and the keymap as data
- **jj**: read the log, its sizes, and the op log from one jj call

### Fix

- **jj**: keep the destination message on squash and fail fast on any editor

### Refactor

- **config**: keep config to a theme and key rebinds
- **ui**: build every overlay from one list component
