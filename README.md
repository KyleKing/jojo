# jojo

A terminal UI for [Jujutsu](https://github.com/jj-vcs/jj), meant as a replacement for
lazygit rather than as another jj log browser.

The log view, the revset box, and the forge pane work. See
[ROADMAP.md](ROADMAP.md) for what is done and what is next, and
[docs/dogfooding.md](docs/dogfooding.md) for how to drive it.

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
configured diff editor the same way `jj split -i` does.

Shared plumbing comes from [aragonite](https://github.com/KyleKing/aragonite), which
already ships `vcs`, `forge/github`, `cache`, `transport`, `ratelimit`, and the `tui/`
packages for theming, overlays, tables, and key hints.

## Keys

`?` in the app is the authoritative list, generated from the same table the footer and
the handlers read. The ones worth knowing before you start:

| Key | Does |
| --- | --- |
| `j` / `k` | move a revision, skipping the graph connectors |
| `space` | check a revision, so the next action runs over all the checked ones |
| `@` | jump to the working copy |
| `/` | search the log, stepping matches with `Ctrl+n` and `Ctrl+p` |
| `:` | the command palette, listing every action and the jj command it runs |
| `L` | edit the revset, which re-filters the log as you type |
| `Tab` | cycle the log, the revset box, and the forge pane |
| `Enter` / `Alt+d` | diff the revision, or diff any two revisions |
| `v` | details: the full description, the author, and the timestamps |
| `n` `e` `d` `s` `S` `i` `a` `A` `D` | new, edit, describe, squash, split, squash interactively, abandon, absorb, duplicate |
| `r` / `R` | rebase the revision, or rebase it and its descendants |
| `b` | bookmarks: set, advance, create, rename, delete, forget, track, push |
| `o` | the operation log, with restore, revert, and per-operation diff |
| `E` | the evolution of the revision, with interdiff and restore |
| `w` | the workspace panel, with add and forget |
| `p` | the forge pane |
| <code>&#124;</code> `X` `B` | parallelize the checked revisions, `jj arrange`, bisect between two checked revisions |
| `u` / `Ctrl+r` | undo the last operation, or redo it |

Every action runs immediately and tells you to press `u`, because jj's operation log
makes all of them reversible. The footer keeps naming the operation `u` would reverse, so
the way back is never a guess. jj has no redo, so `Ctrl+r` restores the operation the undo
moved away from.

Two things ask first, because `jj undo` cannot take them back: a push, which has left the
machine, and an edit to a revision jj marks immutable. The second is not a wall. jj
refuses, jojo says why, and `y` runs the same edit again with `--ignore-immutable`.

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
