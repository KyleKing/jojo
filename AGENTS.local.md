# jojo-specific guidance

## This repo is colocated with jj

`.jj/` and `.git/` both exist here, so drive it with `jj`, not `git`. Work in
incremental changes off the `main` bookmark (`jj describe -m` to name the working
change, `jj new` to start the next), and keep `git` read-only. A detached git HEAD is
normal here and is not something to fix.

The consequence for the check ladder: jj never runs git hooks, so `hk` does not fire on
`jj describe` or `jj new` the way it fires on `git commit`. Run `mise exec -- hk check
--all` by hand before treating a change as finished.
