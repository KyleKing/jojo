# jojo roadmap

[docs/requirements.md](docs/requirements.md) is the full spec and this file is the order
to build it in. Three of the features below arrived from
[jj-diff's roadmap](https://github.com/KyleKing/jj-diff/blob/main/ROADMAP.md) on
2026-09-02, where they had been planned for a repo browser that repo decided not to
become.

## Order

The first two are what the tool is for, and nothing after them matters until they are
readable.

1. **Done.** Log view with the row composition from the requirements: change id,
   bookmarks, description, and size (files touched, `+`/`-` lines), with author and
   timestamp demoted. The size counts come out of the same `jj log` call, because jj
   0.45's template language exposes `diff.stat()`
2. **Done.** Revset box, editable and live-filtering, defaulting to `revsets.log`, with
   completion over jj's revset functions and the repository's own aliases
3. **Done.** Forge pane: pull request state, review decision, and CI rollup for the
   bookmark at or near `@`, with the forge detected from the git remote
4. **Done.** Per-commit actions bound directly (new, edit, describe, squash, split,
   abandon, rebase, absorb), each executing immediately with an undo hint rather than a
   confirm gate, because `jj undo` already covers them. Rebase scope is picked by the
   key rather than inferred: `r` moves the revision, `R` moves it and its descendants
5. **Done.** Operation log view on `o`, jj's undo history as a browsable list rather
   than a bare `jj undo` binding with nothing to see. Restoring to an operation is itself
   an operation, so it undoes like everything else
6. **Done.** Evolution timeline on `E` and the interdiff built on it: how a change
   reached its current state, what a describe, squash, or rebase actually changed, and a
   way back to an earlier version
7. **Done.** Workspace panel on `w`, with add and forget, which neither lazyjj nor jjui
   exposes
8. Repo health surface: stale-working-copy recovery, conflicted-bookmark resolution,
   and divergent-side abandonment, built on the markers the log view already carries.
   The stale-checkout detection has to exist from day one in practice, because any jj
   command the app itself runs can hit or cause it — see the serialized-command-queue
   note in requirements.md

## Decisions worth keeping

**Redo is `jj op restore`, not a jj command.** jj has no redo, so `u` records the
operation the repository was at before undoing and `Ctrl+r` restores it. That makes redo
exact rather than a guess at which operation to go back to, and it is why the redo key
reports having nothing to go back to until an undo has run. `Ctrl+l` reloads the log,
which is what `Ctrl+r` used to do.

**The timeline lists newest first and numbers from the oldest.** `jj evolog` prints
newest first and the numbering counts up from the oldest version, so `[1]` stays `[1]`
as newer versions arrive.

**Restore from the timeline replaces content, not identity.** `r` runs
`jj restore --from <version> --into <change>`, which leaves the change's description and
its place in the graph alone. Restoring a whole revision is what the operation log is
for.

**A new workspace lands beside the repository.** A working copy nested under the
repository root would be snapshotted into its own commits. The panel refuses to forget
the workspace the process is running in, because that leaves jj with no working copy to
act on. jj-diff's scratch workspace is a separate mechanism and never appears here.

## Interactive smartlog

The log view is this, which is recorded here only because Sapling's smartlog was the
reference: the graph is the primary surface and actions are bound on the selected row,
with no separate mode to enter.

```
  @  abc123  (empty) working copy
  │
  ○  def456  feat: add auth
  │
  ○  ghi789  fix: handle errors       ← cursor
  │
  ◆  main    initial commit
```

## Not here

Hunk-level and line-level diff selection stays in
[jj-diff](https://github.com/KyleKing/jj-diff), which jojo runs as jj's diff editor.
Anything that reads across many repositories at once belongs to
[gh-repo-dashboard](https://github.com/KyleKing/gh-repo-dashboard) or
[gh-sweep](https://github.com/KyleKing/gh-sweep).

Everything in the order above except the repo health surface is shipped, and so is every
keybinding layer the requirements describe: `b` bookmarks, `/` search, and `:` the
command palette, plus the L3 power keys (`|` parallelize, `X` `jj arrange`, `B` the
scripted bisect).

[docs/requirements.md](docs/requirements.md) carries what else is still open: conflict
resolution inside jj-diff's hunk engine rather than shelling out to `jj resolve`, a
manual bisect form to sit beside the scripted one, and sharing the operation log between
machines.
