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

**Every jj call goes through one queue.** A second jj process writing the op log while
a command is still running diverges the op heads and strands the checkout, so the
client serializes every invocation — reads included, since a read still snapshots —
and the interactive verbs take the same queue for the life of the process holding the
terminal. That queue is also why a stale working copy can only come from outside the
app: whichever command's stderr reports it opens the `update-stale` offer rather than
a dead-end status line.

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
so abandon, edit, absorb, and the rest run as the key lands, a verb that needs a destination
runs on the Enter that picks it, and each reports the command it ran. A restore chosen while
looking at the past shows what it goes back to first. Two things ask instead, because `jj undo` cannot reach them: a push,
which has left the machine, and a write jj refused as immutable, where the escape rewrites
history other people may already have. The push dialog shows a real `--dry-run`, so what is
being agreed to is jj's own answer, and the immutable dialog says what the rule is rather
than only that it was broken. `u` and `Ctrl+r` are `jj undo` and `jj redo`, which jj has
had since 0.33.

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

**A destination is picked on the log, against the graph it will leave.** Rebase, squash,
duplicate, and new put the log into a picking mode instead of opening a list: what moves is
marked, the cursor walks the log as it was to choose the destination, and the graph redraws as
it will look once the command runs, with the moved, rewritten, and new rows chipped and the
destination marked `◀ onto`, `◀ after`, `◀ before`, or `◀ into`. The flags are footer toggles
over the exact command line the footer shows, Enter runs that command as one operation, and Esc
leaves nothing behind. The list picker stays where a list is the better shape: moving a file or
hunks, and bookmarks.

The preview is jj itself running the command, not a model of it, because a rebase's effect on
descendants, merges, and conflicts is jj's to decide and a reimplementation would be wrong in
exactly the cases worth previewing. It runs in a sandbox (`jj.Sandbox`): the repository's
`.jj/repo` copied as hard links into the user cache directory, beside a new bare git repository
that borrows the real objects through `objects/info/alternates`. jj only adds content-addressed
files beside the ones it has, and the few it deletes and recreates by name (op heads, the
change-id table's heads) are copied rather than linked, so nothing in the sandbox can reach a
byte of the original. Every commit, `refs/jj/keep/*` ref, and exported bookmark a preview writes
lands in the sandbox's git repository. Each preview runs the write with
`--no-integrate-operation`, so even the sandbox's op heads never move, then reads
`jj log --at-op` the operation it printed. A jj without that flag falls back to `op restore`
inside the sandbox.

The alternatives each wrote to the repository. Applying the command and then `jj op restore`
leaves both operations in the log, so the next `u` undoes the restore and brings the preview
back, and a restore would also discard anything another process wrote in between.
`--no-integrate-operation` against the real repository leaves the op log alone but adds a keep
ref per rewritten commit, which `git log --all` then shows. A copy of the metadata needs two more
cares: the repository's own config is handed over as a file from `jj config list --repo` and
`--workspace`, because a copied `config-id` makes jj copy the whole config directory into the
user's jj config, and signing is dropped, so a signing key never prompts on a cursor move. Tests
hash every file under `.jj` and `.git` before and after a burst of previews, a cancel, and a
refused write.

On a 3,300-commit repository with 300 operations, opening the sandbox costs 15ms of linking plus
the snapshot and config reads (70 to 90ms in all), and a preview costs a write and a log, 50 to
60ms on an idle machine and up to 150ms under load. Applying and restoring in place measured the
same per move, so the safe route costs nothing in latency. The cursor waits 150ms before a
preview runs, keeps the newest preview on screen until the next lands, and drops one that lands
for a command no longer being built.

**The past is read, never written.** The op log draws the log as it stood at the operation under
its cursor (`jj log --at-op <op> --ignore-working-copy`), and `[o` and `]o` step through
operations from the log itself. While the log shows the past it is read-only: a write verb would
land on the present, which is not what is on screen, so every one says why it did nothing. `r` or
Enter restores to the operation after a confirmation that shows what it was, and Esc returns to
now. `u` and `Ctrl+r` stay jj's own undo and redo.

**Moving part of a revision is three commands, not a mode.** A whole revision moves with
`s`, one file with `m` in the file list (`jj squash --from --into <path>`, no editor at
all), and hunks or lines with `I`, which picks the destination here and hands the terminal
to whichever diff editor the repository configures. jojo never reimplements hunk selection,
and the file case never pays for an editor it does not need.

**Staging is a reading of jj, not a mode.** The file list of `@` treats `@-` as git's index:
stage and unstage are `jj squash --keep-emptied` between the two, and commit is `jj describe @-`
followed by `jj new --insert-after @- --insert-before @`. Each step moves content between `@` and
`@-` or moves a parent pointer, so none can cost what the working copy holds, and `internal/jj`
refuses all three when `@-` is immutable, shared with another child, or one of several parents,
since rewriting it then reaches past the working copy. There is no setting: someone who does not
think in git never presses the keys.

**The docked diff follows the log, debounced.** `Alt+p` draws the diff beside the log (under it
below 100 columns) and the log keeps the keys. Every cursor move bumps a sequence number and waits
120ms before reading, so holding `j` costs one jj call rather than one per row, and a read the
cursor has already left is dropped when it lands. The new diff opens on the file the reader was in
when the revision touches it, else on its first file.

**Configuration is two settings.** A theme and key rebinds, and nothing that jj already
carries: named revsets are jj's `revset-aliases`, which the revset box completes, so jojo
does not keep a second list of them. A setting jojo cannot honor stops startup, since a TUI
has nowhere to report it once it is drawing.

**Whoever is open owns the keys.** An open overlay takes every key before the log sees it,
since they share `j` and `k`. Below that, Tab cycles the log, the revset box, the forge
pane, and the docked diff, and each handles its own bindings while focused. That is what lets `r` mean rebase in
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
