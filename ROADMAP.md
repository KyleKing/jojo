# jojo roadmap

Nothing is built yet. [docs/requirements.md](docs/requirements.md) is the full spec and
this file is the order to build it in. Three of the features below arrived from
[jj-diff's roadmap](https://github.com/KyleKing/jj-diff/blob/main/ROADMAP.md) on
2026-09-02, where they had been planned for a repo browser that repo decided not to
become.

## Order

The first two are what the tool is for, and nothing after them matters until they are
readable.

1. Log view with the row composition from the requirements: change id, bookmarks,
   description, and size (files touched, `+`/`-` lines), with author and timestamp
   demoted
2. Revset box, editable and live-filtering, defaulting to `revsets.log`
3. Forge pane: pull request state, review decision, and CI rollup for the bookmark at or
   near `@`, with the forge detected from the git remote
4. Per-commit actions bound directly (new, edit, describe, squash, split, abandon,
   rebase, absorb), each executing immediately with an undo hint rather than a confirm
   gate, because `jj undo` already covers them
5. Operation log view
6. Evolution timeline and interdiff
7. Workspace panel

## Operation log view

**Keybinding**: `o`

jj's undo history, which deserves browsing rather than a bare `jj undo` binding with no
visibility into what else is restorable.

```
jj op log --no-graph -T 'operation_id ++ "\t" ++ description ++ "\t" ++ time ++ "\n"'
jj undo
jj op restore <op-id>
```

Bind `u` anywhere for undo and `Ctrl+r` for the redo pattern (`jj op restore` to the
previous operation).

## Evolution timeline

**Keybinding**: `E`

How a change evolved through rebases, amends, and squashes, which is jj's core
differentiator from git and which no other tool surfaces well.

```
jj evolog -r <rev> --no-graph -T 'commit_id ++ "\t" ++ description.first_line() ++ "\n"'
jj diff --from <old> --to <new>
```

```
Evolution of change abc123
─────────────────────────────
  [3] 2h ago   rebase onto main
  [2] 5h ago   amend: fix typo        ← cursor
  [1] 1d ago   initial commit
─────────────────────────────
[Enter] view diff  [d] interdiff  [r] restore  [q] close
```

Interdiff hangs off the same view: mark one version with space, move to a second, and
diff the pair. Reading what a `describe`, `squash`, or `rebase` actually changed before
undoing it is the case that earns it.

## Interactive smartlog

The log view already is this once per-commit actions land, so it is listed here only to
record that Sapling's smartlog shape was the reference: the graph is the primary surface
and actions are bound on the selected row, with no separate mode to enter.

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
