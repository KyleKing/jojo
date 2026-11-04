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
8. **Done.** Repo health surface, built on the markers the log view already carries.
   Every jj invocation goes through one serialized queue, because a mid-write snapshot
   diverges the op heads; a stale working copy surfaces on any command's stderr and one
   confirmation runs `jj workspace update-stale`; a conflicted bookmark resolves to its
   remote-tracking side with `x`; and a divergent change shows on its row, with `x`
   listing the stale sides before `jj abandon` takes them

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

Everything in the order above is shipped, and so is every keybinding layer the
requirements describe: `b` bookmarks, `/` search, and `:` the command palette, plus the
L3 power keys (`|` parallelize, `X` `jj arrange`, `B` the scripted bisect, `x` the
divergent cleanup).

## What daily use needed after that

Written on 2026-09-16, after comparing jojo against jjui's current feature set rather
than against the requirements. Four gaps stood between "complete by the spec" and "a tool
someone lives in", and all four are now shipped:

1. **Done.** The git loop closes inside jojo: `f` fetches every remote and `P` pushes the
   bookmarks on a revision, so fetch, rebase onto trunk, and push no longer need a shell
2. **Done.** `l` lists the files a revision touches, with a per-file diff, restore, and
   conflict resolution. A merge's own diff can be empty while it is still conflicted, so
   the conflict list is folded in or those paths would not appear at all
3. **Done.** `c` resolves conflicts, in the whole revision or one file of it. Conflicts
   were rendered as state and nothing acted on them
4. **Done.** A diff opens in a pane rather than handing the terminal to jj's pager,
   rendered by [jj-diff](https://github.com/KyleKing/jj-diff)'s `render` package (v0.3.0,
   published for this) so both tools draw a diff the same way
5. **Done.** `config.toml` carries a theme and key rebinds, and nothing else. Named
   revsets were tried and dropped: jj's own `revset-aliases` already does that and the
   revset box already completes them
6. **Done.** `s` in the diff pane switches to side by side, drawn by the same jj-diff
   package (v0.4.0), which now shares its two-column pairing with the diff editor
7. **Done.** Moving part of a revision into a specific commit: `m` in the file list moves
   one file with no editor at all, and `I` picks a destination here and hands hunk and line
   selection to the configured diff editor

## Still open

**Large repositories are untested.** The log loads 500 rows in one call with a diff stat
per row, and reaching the last row doubles the window and reloads, so deep history pages
in rather than cutting off. Startup cost on a repo with tens of thousands of reachable
revisions is still unmeasured.

[docs/requirements.md](docs/requirements.md) carries the rest: conflict resolution inside
jj-diff's hunk engine rather than shelling out to `jj resolve`, a manual bisect form to sit
beside the scripted one, and sharing the operation log between machines.
