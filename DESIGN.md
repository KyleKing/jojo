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

**Undo is the confirmation.** Every routine jj edit is an operation the log can reverse,
so describe, squash, abandon, rebase, and the rest run as the key lands and report the
command they ran. Two things ask instead, because `jj undo` cannot reach them: a push,
which has left the machine, and a write jj refused as immutable, where the escape rewrites
history other people may already have. The push dialog shows a real `--dry-run`, so what is
being agreed to is jj's own answer, and the immutable dialog says what the rule is rather
than only that it was broken. jj has no redo, so `u` records the operation the repository
was at and `Ctrl+r` restores it.

**The status line reports the command, not a description of it.** Every jj write returns
the invocation it ran, quoted so it can be pasted into a shell. A description would be a
second thing to keep true as the flags change, and it would teach nothing.

**The keymap is data.** `internal/components/help` holds the whole table: the footer, the
help overlay, the command palette, and the root model's dispatch all read one `Keymap`, so
a binding cannot drift between them. The palette shows each action beside the jj command it
runs, which is why the shapes live in that table too. A rebind from `config.toml` rewrites
that table once at startup and leaves a translation from the pressed key to the key the
handlers switch on, so the handlers never learn that rebinding exists. A key whose action
moved away translates to nothing rather than staying live, or the legend would be a lie.

**One list, every view.** The operation log, workspaces, bookmarks, the evolution timeline,
the file list, the diff pane, the destination picker, and the command palette are all
`internal/components/listview` with their own rows, keys, and messages. Scrolling, clamping,
truncation, the warning line, the minimum frame, and hint degradation are written once, so a
rendering fix lands in all of them.

**The diff is rendered by jj-diff, not by jojo.** `jj-diff/render` turns git-format diff
text into styled lines, unified or side by side, and jojo scrolls them. The parser, the
word-level intra-line highlighting, and the syntax highlighting are the ones the diff
editor uses, so the two tools cannot grow separate ideas of what a diff looks like. jojo
passes its own palette in rather than inheriting jj-diff's theme. The one diff that still
reaches jj's pager is an operation's, because `jj op show` prints a header no diff parser
reads.

**Moving part of a revision is three commands, not a mode.** A whole revision moves with
`s`, one file with `m` in the file list (`jj squash --from --into <path>`, no editor at
all), and hunks or lines with `I`, which picks the destination here and hands the terminal
to whichever diff editor the repository configures. jojo never reimplements hunk selection,
and the file case never pays for an editor it does not need.

**Configuration is two settings.** A theme and key rebinds, and nothing that jj already
carries: named revsets are jj's `revset-aliases`, which the revset box completes, so jojo
does not keep a second list of them. A setting jojo cannot honor stops startup, since a TUI
has nowhere to report it once it is drawing.

**Whoever is open owns the keys.** An open overlay takes every key before the log sees it,
since they share `j` and `k`. Below that, Tab cycles the log, the revset box, and the forge
pane, and each handles its own bindings while focused. That is what lets `r` mean rebase in
the log and refresh in the forge pane.

**Search moves the cursor, the revset changes the list.** `/` finds a revision among the
ones already listed and steps between matches inside its own bar, because `n` is taken by
"new" in the always-active layer. `L` changes which revisions are listed at all.

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
