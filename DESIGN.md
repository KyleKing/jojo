# Design

## Architecture

`cmd/jojo` parses flags, detects the skin, and hands a `model.Model` to Bubble Tea.
Everything else is under `internal/`.

`internal/jj` is the only package that runs `jj`. It shells out, parses, and returns
structured values. Nothing else in the tree knows the CLI exists, which is what keeps a
template change to one file.

`internal/model` owns the panes, routes every key, and is the only caller of
`internal/jj`. Sub-models under `internal/components` render themselves and report
through messages. They never run a command and never hold a palette.

`internal/ui` holds the app's style vocabulary. Aragonite's rendering packages
(`tui/table`, `tui/keyhint`, `tui/overlay`, `tui/region`) each take a `Styles` value, and
`internal/ui` is the one place those get built, so a color is named once.

`internal/forge` sits behind a `Provider` interface over aragonite's `forge/github`. It
models the common case as one aggregate CI state with an optional per-check list, because
GitHub's check runs, GitLab's pipelines, and Forgejo's commit statuses do not line up one
to one.

`internal/tuitest` and `scripts/pty.sh` drive the app: golden frames in process, tmux for
a real terminal. [docs/dogfooding.md](docs/dogfooding.md) has the commands.

## Patterns

**The log row comes from one `jj log` call.** jj 0.45's template language exposes
`diff.stat().total_added()`, `total_removed()`, and `diff.files().len()`, so the files and
line counts on every row cost nothing extra. An earlier plan budgeted a second query and a
commit-id cache for this, and neither is needed.

**jj draws the graph, jojo parses it.** The log runs with the graph on and a one-line
template whose fields are separated by US (`\x1f`). A line carrying that byte is a
revision, and the text before its first field is the graph prefix jj drew. A line without
it is a connector or an elision marker, rendered in place and skipped by the cursor. Graph
width is measured across revision rows only, because jj writes prose on some connectors
(`~  (elided revisions)`) that would otherwise drag the table right.

**Bookmarks are kept in three forms.** `Bookmarks` is what the row renders and keeps jj's
markers (`main*` for a local bookmark ahead of its remote). `LocalBookmarks` and
`RemoteBookmarks` are stripped names, which is what a forge can be asked about. Handing
`main*` to `gh` would ask about a branch that does not exist.

**A reversible action executes, it does not ask.** Every routine jj edit is an operation
the log can undo, so describe, squash, abandon, rebase, and the rest run immediately and
report "done, press u to undo". Confirmation is reserved for what `jj undo` cannot reverse
cleanly: a push, a force-push, and anything run with `--ignore-immutable`.

**The keymap is data.** `internal/components/help.Bindings` is the whole table, and the
footer, the overlay, and the root model all read it. A binding cannot drift between the
three places it has to agree.

**A focused pane owns its keys.** Tab cycles the log, the revset box, and the forge pane,
and each handles its own bindings while focused. That is what lets `r` mean rebase in the
log and refresh in the forge pane.

**An open overlay owns every key.** The operation log, the evolution timeline, the
workspace panel, the describe prompt, and the destination picker all share `j` and `k`
with the log underneath them, so the root routes keys to whichever one is open before
anything else looks at them.

**Redo is an operation id, not a command.** jj has no redo. `u` records the operation the
repository was at and `Ctrl+r` restores it, which is exact rather than a guess at which
operation to return to.

**Unrecognized messages fan out to every sub-model.** A component's debounce and poll ticks
are its own unexported types, so the root model cannot switch on them. Anything it does
not own goes to every pane instead.

## Testing

Integration tests build real jj repositories in `t.TempDir()` and run the real binary,
skipping when `jj` is absent. Forge tests replay recorded `gh` calls through aragonite's
`ghcassette` and never reach GitHub.

Rendered output is asserted against golden files with ANSI stripped, regenerated only with
`go test ./... -update`.

`scripts/fixture-repo.sh` builds a repository carrying every shape the log has to render:
a merge, a conflict, immutable ancestors, a bookmark off the head, an empty working copy,
and a large diffstat. Change ids and commit ids differ between runs because jj reseeds its
randomness per process, so no test may assert on one.
