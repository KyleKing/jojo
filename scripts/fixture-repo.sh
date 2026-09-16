#!/usr/bin/env bash
# Builds a jj repo covering jojo's log-rendering shapes: linear chain, merge,
# conflict, bookmarks, immutable ancestors, empty @, and a many-file commit.
# Usage: fixture-repo.sh [path]; prints the repo path as the last stdout line.
set -euo pipefail

repo="${1:-$(mktemp -d)}"

exec 3>&1 1>&2

export JJ_USER="Fixture Bot"
export JJ_EMAIL="fixture@example.com"
export JJ_TIMESTAMP="2024-01-01T00:00:00+00:00"

jj git init "$repo"
cd "$repo"

change_id() {
	jj log -r @ --no-graph -T 'change_id.short()'
}

echo a >a.txt
jj describe -m "add a"
c1=$(change_id)

jj new -m "add b"
echo b >b.txt
c2=$(change_id)

jj new -m "add c"
echo c >c.txt
c3=$(change_id)

jj bookmark create main -r "$c3"
jj bookmark create wip -r "$c2"

jj new "$c3" -m "branch one: add branch-one.txt"
echo one >branch-one.txt
b1=$(change_id)

jj new "$c3" -m "branch two: add branch-two.txt"
echo two >branch-two.txt
b2=$(change_id)

jj new "$b1" "$b2" -m "merge branch one and branch two"

jj new "$c1" -m "seed shared file"
echo "shared line one" >shared.txt
seed=$(change_id)

jj new "$seed" -m "edit shared file one way"
echo "edited by one way" >shared.txt
x=$(change_id)

jj new "$seed" -m "edit shared file another way"
echo "edited by another way" >shared.txt
y=$(change_id)

jj rebase -r "$y" -d "$x"

jj new "$c3" -m "large refactor across many files"
for i in $(seq 1 6); do
	seq 1 40 >"big_$i.txt"
done
big=$(change_id)

jj new "$big"

jj config set --repo 'revset-aliases."immutable_heads()"' '"wip"'

echo "$repo" >&3
