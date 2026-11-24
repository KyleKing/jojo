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

`internal/stack` turns a stack into stacked pull requests, and `internal/gh` is the one package
that runs `gh` for a write. `stack.Read` gathers what a plan needs (whole descriptions, trunk's
branch, and `gh pr view` per bookmark), `stack.Build` works out the plan as exact commands without
writing, and `stack.Run` carries it out through `jj.Client.Apply` and `gh.CLI`. Both runners file
into the one `trail`, so `gm` lists a submit's gh calls beside its jj ones.

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

**The keymap is data.** `internal/components/help` merges aragonite's shared `tui/keys` table
(`Global()`, `DiffView()`, the picker flags from `Flags(verb)`) with jojo's own jj verbs into one
`Keymap`. The root model feeds every key to one `keys.Matcher` with the screen it is on (the log, a
picker verb, time travel, the outline, the dock, the file list), which holds a prefix (`g`, `b`,
`]`, `[`, `\`, `-`) while it waits and answers an action name. Dispatch, the `?` legend for that
screen, the footers, the palette, and the which-key panel a waiting prefix floats all read that
table, so a binding cannot drift between them. `?` and `:` are one engine, aragonite's
`tui/palette`, over `palette.FromKeymap(keymap, screen, gate, help.About(keymap))`: `?` browses the
screen's keys and explains the one pressed, `:` searches and runs by action name, never by replaying
a key. `help.About` holds what the palette teaches beyond a binding: the jj command shape, git-word
aliases, prose, and related keys. `Model.gate` is the one switch that says why a log verb can do
nothing where the cursor is; the footer draws it flat, the palette lists it off with the reason, and
a press says the reason. The coach (`tui/coach`) reads the log through `logState` and the rules in
`internal/model/discover.go`, and every jj call goes through the client's `trail.Trail`, which `gm`
lists and a write's status line echoes. A rebind from `config.toml` names an action,
so the handlers never learn that rebinding exists, and an old action name resolves through
`help.Deprecated` with a footer warning for one release. `TestKeymap_AgreesWithShared` fails when a
jojo key reuses a key the shared table gives another action on a screen the two share, which is
what keeps one key meaning one thing across jojo, jj-diff, and second-look.

**One list, every view.** The operation log, workspaces, bookmarks, the rounds box,
the file list, the diff pane, and the destination picker are all
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

That safety rests on jj never rewriting a content-addressed file in place, which is jj's
behavior rather than its promise, so it is checked twice. The sandbox records the size and
modification time of every file it linked, and `Close` compares them when the picker closes: a
change says a preview may have written into the repository, the footer says so, and that
client's later sandboxes copy instead of link. And
`TestSandbox_EveryPreviewVerbLeavesTheLinkedFilesAlone` runs each picker verb against the
installed jj and fails on a jj newer than `linkVerifiedJJ` (0.47) until someone has re-checked
the op store and index and bumped it.

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

**The docked diff follows the log, debounced.** `p` draws the diff beside the log (under it
below 100 columns) and the log keeps the keys. Every cursor move bumps a sequence number and waits
120ms before reading, so holding `j` costs one jj call rather than one per row, and a read the
cursor has already left is dropped when it lands. The new diff opens on the file the reader was in
when the revision touches it, else on its first file. The lexer never runs on the update loop: a
diff is a jj-diff `render.Document`, drawn with `Peek` (plain where a file is not lexed yet), and
`Colorize` starts the lexers and returns a command that only waits for them, after which the rows
are drawn again in color on the same lines. The pane keeps the last
eight colored diffs, so stepping back to a revision just left draws it in color at once.

**A stack is reviewed as chapters.** `I` reads the stack at the cursor (`outline_revset`, by
default what the revision sits on above `trunk()` and what is stacked on it) and draws it in the
log's place, parent first, docking the diff beside it on the chapter under the cursor. The outline
is one background read: the graph, the stats of every chapter, and whatever the forge pane's caches
already hold for each bookmark (`forge.Poller.Known`, which never reaches the network). `]]` and
`[[` step chapters from the outline or the dock, and `]h` steps hunks in the dock without taking
the focus. The outline reads and never writes, so a verb pressed there says where the verbs are.

**A stack is submitted from a plan.** `P` in the outline replaces it with the plan, which is read
in the background and drawn like the picker: the rows, then a footer of what Enter runs and its
flags (`-d`, drafts). The plan is a list of commands in a fixed order: bookmarks, one push, creates
bottom up, retargets bottom up, then bodies, which wait for every number so each section can link
the others. A body edit is planned only when a pull request was opened (every number changed) or
when the section would read differently, compared with CRLF and trailing space ignored, so
re-submitting after an amend writes nothing to GitHub. Retargets edit by branch name, so a plan is
exact before a single number is known. Nothing deletes a remote branch: a base deleted under an
open pull request closes it. The gate refuses before a read, from the outline's own flags, and
the read refuses again on the whole descriptions. The empty, undescribed `@` jj leaves on top is
dropped rather than refused, since it is where the next change starts rather than a change.

**A change's versions are rounds.** `[r` and `]r` step the docked diff through `jj evolog`, each
version shown as `jj interdiff --from <its own previous version> --to <it>`, so what a rebase
changed in the parents drops out. The evolution is read once per commit id with one template that
also asks `self.inter_diff()` for its size, which is how a version that changed neither the patch
nor the description is known without diffing it: consecutive ones fold into one "rebased, no
content change" round. Both the evolution and each round's diff are cached for the session under
commit ids, which name their content, and every read rides the dock's sequence number, so a read
the cursor has left is dropped. `H` lists the rounds to jump to one.

**Read marks have a seam, not a store.** `model.ReadState` answers `Unread(changeID, hunkHash)`,
with hunks named by `internal/readmark` exactly as second-look's `internal/seen` names them (path
and every line's kind and text, line numbers left out), so the shared `aragonite/seen` store can
plug in without changing an id. Until it does, nothing is marked and each chapter's strip only
counts the hunks of a diff the dock has read.

**What a row had no room for goes on one line.** The inspector over the footer says the whole of
what the cursor row cut or collapsed at this width (description, bookmarks, author, age), with the
commit id at its right edge, so a narrow terminal loses nothing one cursor move cannot get back.
It costs one row, and none below 12 rows of terminal.

**Configuration is a handful of settings.** A theme, key rebinds, the diff tool, the outline's
stack revset, and the bookmark name a submit generates, and nothing that jj already
carries: named revsets are jj's `revset-aliases`, which the revset box completes, so jojo
does not keep a second list of them. A setting jojo cannot honor stops startup, since a TUI
has nowhere to report it once it is drawing.

**Whoever is open owns the keys.** An open overlay takes every key before the log sees it,
since they share `j` and `k`. Below that, Tab cycles the log, the forge pane, and the docked diff
(the revset box is `L`, so a stray Tab never types into it), and each screen's bindings are its own
contexts in the keymap. That is what lets `p` dock the diff from the log and show the graph before
the command in the picker.

**Search moves the cursor, the revset changes the list.** `/` finds a revision among the
ones already listed and steps between matches inside its own bar, because `n` is taken by
"new" in the always-active layer. `L` changes which revisions are listed at all.

**Unrecognized messages fan out to every sub-model.** A component's debounce and poll ticks
are its own unexported types, so the root model cannot switch on them. Anything it does
not own goes to every pane instead.

## Testing

Integration tests build real jj repositories in `t.TempDir()` and run the real binary,
skipping when `jj` is absent. Stack submit tests pair such a repository and a bare git remote with
a fake `gh` (`internal/gh/ghtest`, built from its `testdata`) that keeps a forge as JSON, answers
`pr view`, `create`, and `edit`, records every call, and fails loudly on anything else. Forge tests replay recorded `gh` calls through aragonite's
`ghcassette` and never reach GitHub.

Rendered output is asserted against golden files with ANSI stripped, regenerated only with
`go test ./... -update`.

`scripts/fixture-repo.sh` builds a repository carrying every shape the log has to render:
a merge, a conflict, immutable ancestors, a bookmark off the head, an empty working copy,
and a large diffstat. Change ids and commit ids differ between runs because jj reseeds its
randomness per process, so no test may assert on one.
