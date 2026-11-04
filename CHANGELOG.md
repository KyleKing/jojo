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
