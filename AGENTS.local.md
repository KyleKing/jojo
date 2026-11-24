# jojo-specific guidance

## This repo is colocated with jj

`.jj/` and `.git/` both exist here, so drive it with `jj`, not `git`. Work in
incremental changes off the `main` bookmark (`jj describe -m` to name the working
change, `jj new` to start the next), and keep `git` read-only. A detached git HEAD is
normal here and is not something to fix.

The consequence for the check ladder: jj never runs git hooks, so `hk` does not fire on
`jj describe` or `jj new` the way it fires on `git commit`. Run `mise exec -- hk check
--all` by hand before treating a change as finished.

## Tests never touch the real home

jj 0.47 keeps each repository's config, and a record of every repository it opens, under
`$XDG_CONFIG_HOME/jj/repos/`, so `jj config set --repo` in a fixture, or a `.jj/repo/config.toml`
jj migrates on its next call, writes into the home of whoever runs the tests. Every test package's
`TestMain` is `hermetic.Main(m)`, which points HOME, the XDG directories, `JJ_CONFIG`, git's global
config, gh's config, and `TMPDIR` at a root of the binary's own, names the committer through
`JJ_USER`/`JJ_EMAIL`, and fails the run when the real jj config, git or gh config, or jojo's config,
state, or cache changed because of it. `TestEveryTestPackageIsHermetic` fails a package that leaves
it out. A fixture sets repository config with `hermetic.RepoConfig`, never `jj config set`.
