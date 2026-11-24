# jojo

A terminal UI for [Jujutsu](https://github.com/jj-vcs/jj), meant as a replacement for
lazygit rather than as another jj log browser.

Everything in [ROADMAP.md](ROADMAP.md) is shipped;
[docs/dogfooding.md](docs/dogfooding.md) is how to drive it.

```sh
mise run build
./jojo --repo path/to/repo
```

The requirements and the survey of what exists were relocated from
[jj-diff](https://github.com/KyleKing/jj-diff) on 2026-09-02 once that project settled on
staying a diff tool.

## Why another one

[lazyjj](https://github.com/Cretezy/lazyjj) and [jjui](https://github.com/idursun/jjui)
both browse a jj repository, and neither reaches what lazygit does. The specific
complaints, which are the ones this project has to answer:

- The log is hard to read. A row spends its width on the author's full email address and
  a timestamp, when the change id, the bookmarks, and the description are what you scan
  for. Email and time are metadata, so they belong dimmed, abbreviated, or off the row
  entirely
- A row says nothing about size. How many files a revision touches and how many lines it
  adds and removes is what tells you whether to open it, and no existing tool puts that
  on the row
- Forge integration is missing or bolted on. Neither tool lists pull requests or CI state
  next to the revisions they belong to

### Alternatives

The wider field, for anyone who wants a jj front end today rather than waiting on this
one, plus the git TUIs and editors that set the bar:

| Service | Notes |
| --- | --- |
| [gg](https://github.com/gulbanana/gg) | "GG - Gui for JJ" |
| [gitui](https://github.com/gitui-org/gitui) | "Blazing 💥 fast terminal-ui for git written in rust 🦀" |
| [jayjay](https://github.com/hewigovens/jayjay) | "A native macOS GUI for Jujutsu (jj), DAG graph, side-by-side diffs, interdiff, conflict resolution. Built with Rust + SwiftUI" |
| [jj-fzf](https://github.com/tim-janik/jj-fzf) | "Text UI for Jujutsu based on fzf, centering around the jj log with key bindings for common operations" |
| [jj_tui](https://github.com/faldor20/jj_tui) | "A TUI for the Jujutsu version control system" |
| [jjui](https://github.com/idursun/jjui) | "jjui is a TUI designed for interacting with the Jujutsu version control system" |
| [Jujutsu Kaizen](https://github.com/keanemind/jjk) | "Jujutsu (jj) VCS support for VS Code" |
| [jujutsu.nvim](https://github.com/yannvanhalewyn/jujutsu.nvim) | "A Neovim frontend for Jujutsu with blazing fast UX" |
| [lazygit](https://github.com/jesseduffield/lazygit) | "simple terminal UI for git commands", the tool this project wants to replace |
| [lazyjj](https://github.com/Cretezy/lazyjj) | "TUI for Jujutsu/jj" |
| [lightjj](https://github.com/chronologos/lightjj) | "Lightning-fast JJ UI" |
| [madicen/jj-tui](https://github.com/madicen/jj-tui) | "A TUI for Jujutsu that incorporates some light PR and Ticket support" (name clash noted below) |
| [magit](https://github.com/magit/magit) | "It's Magit! A Git Porcelain inside Emacs" |
| [tig](https://github.com/jonas/tig) | "Text-mode interface for git" |
| [VisualJJ](https://www.visualjj.com) | "Jujutsu with your Git" (Closed source) |
| _[awesome-jj](https://github.com/Necior/awesome-jj)_ | "A curated list of awesome Jujutsu things" |

## What makes it different

The forge is detected from the git remote and the matching CLI is used behind an
interface (`gh` for GitHub first, `glab` and Forgejo later). This is not a `gh` extension
and does not install as one, which is where it parts company with
[gh-repo-dashboard](https://github.com/KyleKing/gh-repo-dashboard) and
[gh-sweep](https://github.com/KyleKing/gh-sweep). Those two are fleet tools that read
many repositories through GitHub. jojo reads one repository deeply and asks the forge
about it second.

Hunk-level and line-level diff work is not rebuilt here.
[jj-diff](https://github.com/KyleKing/jj-diff) already does it, so jojo runs it as jj's
diff editor the same way `jj split -i` does: when `jj-diff` is on PATH and jj's
`ui.diff-editor` is unset, the hunk verbs pass `--tool jj-diff`. Reading a diff happens inside
jojo, drawn by jj-diff's own `render` package, so a diff looks the same in both tools and
neither one has its own idea of what a diff looks like.

Shared plumbing comes from [aragonite](https://github.com/KyleKing/aragonite), which
already ships `vcs`, `forge/github`, `cache`, `transport`, `ratelimit`, and the `tui/`
packages for theming, overlays, tables, and key hints.

## Keys

`?` in the app is the authoritative list, generated from the same table the footer and
the handlers read. The ones worth knowing before you start:

| Key | Does |
| --- | --- |
| `j` / `k` | move a revision, skipping the graph connectors |
| `space` / `Esc` | check a revision, or clear the checks. While any are checked the footer counts them and `a` `n` `D` `r` `R` `s` `P` run over all of them instead of the cursor; a write that goes through clears them |
| `@` | jump to the working copy |
| `/` | search the log, stepping matches with `Ctrl+n` and `Ctrl+p` |
| `:` | the command palette, listing every action and the jj command it runs |
| `L` | edit the revset, which re-filters the log as you type |
| `Tab` | cycle the log, the revset box, the forge pane, and the docked diff |
| `Enter` / `Alt+d` | diff the revision in a pane, or diff any two revisions; `s` there switches to side by side |
| `v` / `l` | details of the revision, or the files it touches, where `m` moves one file into another revision |
| `space` / `C` in the files of `@` | stage or unstage the file, or commit what is staged, with `@-` standing in for git's index |
| `Alt+p` | dock the diff beside the log, where it follows the cursor and keeps the file you were reading; `Tab` reaches it, and there `]h` / `[h` step hunks, `]f` / `[f` files, and `n` / `N` repeat the last motion |
| `I` | the stack outline: the revisions between `trunk()` and the cursor, plus what is stacked on it, as chapters parent first, each with its size, empty and conflict flags, bookmark, the PR and CI state the forge pane already knows, and a read strip; the diff docks beside it on the chapter under the cursor, `]]` / `[[` step chapters, `]h` steps the dock's hunks, `Enter` reads in the dock, `Esc` returns to the log |
| `[r` / `]r` / `H` | rounds: step the docked diff back and forth through the change's `jj evolog` versions, each shown as a true `jj interdiff` from the version before it under a banner like `version 3 of 5 · rebase · 2 hours ago`; rebases that changed nothing fold into one "rebased, no content change" round; `]r` past the newest returns to the whole change; `H` lists the rounds to jump to one |
| `c` | resolve the conflict, in the whole revision or one file of it |
| `f` / `P` | fetch every remote, or push the revision's bookmarks; a bookmark new to the remote is pushed by name, which tracks it, and a revision with none goes up under jj's generated `push-<change id>` (`jj git push -c`), each shown in the confirmation first |
| `n` `e` `d` `s` `S` `a` `A` `D` | new, edit, describe, squash, split, abandon, absorb, duplicate; `n` `s` `D` open the picker below |
| `d` / `Alt+e` | describe in a prompt that holds the whole description, body and trailers included (`alt+enter` breaks a line, `alt+e` moves to `$EDITOR`), or straight in `$EDITOR` |
| `i` / `Alt+i` | squash hunks into the parent, or into any revision you pick |
| `r` / `R` | rebase the revision, or it and its descendants, onto any revision including `trunk()` and the root, in the picker below |
| in the picker | `j` / `k` choose the destination on the log while the graph redraws as it will be after the command; `o` `a` `b` onto, after, or before it; `r` `s` `B` move the revisions, them with descendants, or the branch (rebase); `e` skip emptied (rebase) or keep emptied (squash); `m` combine messages in `$EDITOR` (squash); `w` no-edit (new); `h` duplicate in place; `Tab` shows the graph before or after the command; `Enter` runs the command the footer shows as one operation; `Esc` leaves nothing behind |
| `b` | bookmarks: set, advance, create, rename, delete, forget, track, push |
| `o` | the operation log, drawing the log as it stood at the operation under the cursor, with restore (after a confirmation), revert, and per-operation diff drawn in the diff pane |
| `[o` / `]o` | step the log back or forward one operation, read-only: `r` restores to it, `o` opens the op log there, `Esc` comes back to now |
| `E` | the evolution of the revision, with interdiff and restore |
| `w` | the workspace panel, with add and forget |
| `p` | the forge pane |
| <code>&#124;</code> `X` `B` | parallelize the checked revisions, `jj arrange`, bisect between two checked revisions |
| `x` | converge divergent changes with `jj converge` (the one under the cursor when it is divergent), or `a` in its dialog to abandon the stale sides |
| `u` / `Ctrl+r` | undo the last operation, or redo it |
| `!` | the full text of the last failure, when the footer only had room for one line of it |

The line over the footer is the inspector: whatever the row under the cursor had no room for at
this width (the whole description, bookmarks, author, age), with the commit id at its right edge.

Every action runs immediately and tells you to press `u`, because jj's operation log
makes all of them reversible. The footer keeps naming the operation `u` would reverse, so
the way back is never a guess. `u` and `Ctrl+r` are jj's own `jj undo` and `jj redo`, so
they step through the operation log the way an editor's undo stack does.

### Staging, for people coming from git

The file list of the working copy (`l` on `@`) also speaks git, without changing anything else
about jj. `@-` stands in for the index: it lists the working copy's own files as unstaged and
`@-`'s files beneath them as staged. `space` stages the file under the cursor
(`jj squash --from @ --into @- <path>`) or unstages a staged one (the same squash in the other
direction), and `C` commits: it describes `@-` and inserts a fresh empty change between it and `@`
to be the next index. None of the three changes a file in the working copy, only which revision
records it. When `@-` is immutable, has another child, or `@` is a merge, the keys refuse and say
why rather than rewrite past the working copy.

Two things ask first, because `jj undo` cannot take them back: a push, which has left the
machine, and an edit to a revision jj marks immutable. The second is not a wall. jj
refuses, jojo says why, and `y` runs the same edit again with `--ignore-immutable`.

## Key usage

jojo logs the command keys you press, per context (`log`, `forge`, `help`, and one per overlay), and never the text you type into the revset box, a prompt, search, or the palette's query. A rebound key counts under its action. It records into `$JOJO_USAGE_DIR`, else `~/Sync/.jojo/usage/` when `~/Sync` exists, and records nothing when neither is there.

`jojo keys` reads the log back: the bindings never pressed, the ones gone stale for 30 days, keys pressed where nothing is bound, and keys that mean different things in different contexts. Keys you have not pressed lately draw bold or underlined in the footer and `?`.

Every run also logs the subcommand and the flag names it was given (`jojo keys -r`), never a flag's value, and `jojo keys` reports the subcommands and flags you have not used.

## Configuration

Everything is optional and jojo runs with no config file at all. It reads
`$XDG_CONFIG_HOME/jojo/config.toml`, or `~/.config/jojo/config.toml`:

```toml
# latte or light, macchiato or dark. Omit it to read the terminal's background.
theme = "macchiato"

# The diff editor split, squash -i and move-hunks hand the terminal to. Omitted or "auto"
# names jj-diff when it is on PATH and jj's own ui.diff-editor is unset; "none" leaves it to
# jj's config; any other name is passed as --tool.
diff_tool = "auto"

# The stack the outline (I) reads, {rev} standing for the revision under the cursor. Omitted, it
# is what that revision sits on above trunk() and what is stacked on it.
outline_revset = "(trunk()..{rev}) | ({rev}:: ~ ::trunk())"

# Move an action to another key, naming it the way `?` names it.
[keys]
describe = "M"
```

That is the whole file. Named revsets are jj's own `revset-aliases`, which the revset box
already completes, so jojo does not keep a second list of them.

A rebind moves a key rather than adding one, and jojo refuses to start on a setting it
cannot honor (an action it does not have, two actions on one key, a theme it does not
know), because a preference that silently does nothing is worse than one that says so.

The status line reports the jj command that ran (`jj rebase -r xyz --onto abc`) rather
than a description of it, and `:` lists every action beside the command it runs. Driving
the TUI should teach the CLI, not hide it.

## Docs

- [Requirements](docs/requirements.md) for what the tool has to do and why, including the
  jj paradigms the design has to respect
- [Comparison](docs/tui-comparison.md) for lazyjj, jjui, madicen/jj-tui, and jj-diff
  rated side by side, with screenshots
- [ROADMAP.md](ROADMAP.md) for the ordered work

## Name

Was jj-tui, until [madicen/jj-tui](https://github.com/madicen/jj-tui) turned out to be an
unrelated project with the same name. jojo replaced it: short, easy to type over an SSH
session, and close enough to the abbreviation to read as one. If you're wondering why it
fits so well: it was jj, all along.
