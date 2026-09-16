# Dogfooding jojo

## Fixture repos

`scripts/fixture-repo.sh [path]` builds a jj repo covering the shapes the log view has to render: a linear chain, a bookmark on a non-head revision, a merge commit, a conflicted commit, an empty `@`, immutable ancestors (the `wip` bookmark marks the immutable boundary), and a commit touching several files. It prints the repo path as the last line of stdout, so capture it with `repo=$(scripts/fixture-repo.sh)`. Pass a path yourself to keep the repo around for inspection.

The commit content, descriptions, bookmarks, and diffs are identical between two runs (`JJ_USER`, `JJ_EMAIL`, and `JJ_TIMESTAMP` are pinned). The change IDs and commit IDs are not: jj draws them from process-local randomness on every `jj new` or `jj describe`, and there is no environment variable that reproduces that sequence across separate CLI invocations, so do not assert against a specific ID.

## Driving the TUI under tmux

`scripts/pty.sh` runs the program under a real pty, since a piped subprocess reads as a non-tty and the program will never render. It keeps one tmux session, `jojo-pty` by default (override with `JOJO_PTY_SESSION`).

```
repo=$(scripts/fixture-repo.sh)
scripts/pty.sh start 100 30 -- ./jojo --repo "$repo"
scripts/pty.sh keys down down enter
scripts/pty.sh capture --plain   # SGR stripped, for diffing
scripts/pty.sh capture           # colors kept, for a human to read
scripts/pty.sh stop
```

`keys` forwards its arguments straight to `tmux send-keys`, so both named keys (`Enter`, `Escape`, `Up`, `C-c`) and literal characters work. `start` polls the pane for its first non-blank frame instead of sleeping a fixed amount, up to `JOJO_PTY_READY_TIMEOUT_S` (default 5) seconds.

## Golden-frame tests

`internal/tuitest` drives a `tea.Model` in-process, without tmux, for tests that assert an exact frame.

```go
frame := tuitest.Run(t, myModel, 80, 24, tuitest.Keys("down", "down", "enter")...)
tuitest.AssertGolden(t, frame)
```

`Run` sends `Init`'s first command (if any), a `WindowSizeMsg` at the given size, then each message in order, and returns the resulting frame with ANSI stripped. It does not run Bubble Tea's scheduler, so a model that depends on a command's result arriving later (an I/O response, a timer) needs that result passed as one of the trailing messages instead.

`AssertGolden` compares the frame against `testdata/<TestName>.golden`. Regenerate goldens with `go test ./... -update` and review the diff; never hand-edit a `.golden` file.

`charm.land/x/exp/teatest/v2` is not part of this module's dependency graph (it is not required by `go.mod`, and adding it is out of scope for this harness), so `tuitest` is built directly on `golden` and the model's own `Update`/`View`.
