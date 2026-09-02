# jj-tui

A terminal UI for [Jujutsu](https://github.com/jj-vcs/jj), meant as a replacement for
lazygit rather than as another jj log browser.

No code yet. This repo currently holds the requirements, the survey of what exists, and
the roadmap, relocated from [jj-diff](https://github.com/KyleKing/jj-diff) on 2026-09-02
once that project settled on staying a diff tool.

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

## What makes it different

The forge is detected from the git remote and the matching CLI is used behind an
interface (`gh` for GitHub first, `glab` and Forgejo later). This is not a `gh` extension
and does not install as one, which is where it parts company with
[gh-repo-dashboard](https://github.com/KyleKing/gh-repo-dashboard) and
[gh-sweep](https://github.com/KyleKing/gh-sweep). Those two are fleet tools that read
many repositories through GitHub. jj-tui reads one repository deeply and asks the forge
about it second.

Hunk-level and line-level diff work is not rebuilt here.
[jj-diff](https://github.com/KyleKing/jj-diff) already does it, so jj-tui runs it as jj's
configured diff editor the same way `jj split -i` does.

Shared plumbing comes from [aragonite](https://github.com/KyleKing/aragonite), which
already ships `vcs`, `forge/github`, `cache`, `transport`, `ratelimit`, and the `tui/`
packages for theming, overlays, tables, and key hints.

## Docs

- [Requirements](docs/requirements.md) for what the tool has to do and why, including the
  jj paradigms the design has to respect
- [Comparison](docs/tui-comparison.md) for lazyjj, jjui, madicen/jj-tui, and jj-diff
  rated side by side, with screenshots
- [ROADMAP.md](ROADMAP.md) for the ordered work

## Name

[madicen/jj-tui](https://github.com/madicen/jj-tui) exists and is a different project
with the same name. Worth deciding whether to keep this one before the repo has users.
