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
