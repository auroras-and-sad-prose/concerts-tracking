#!/usr/bin/env bash
# Holds a direct push to main to checked.json and nothing else.
#
# The concert-watch routine is allowed to push checked.json to main without a
# pull request (see "When the data was last checked" in CLAUDE.md), and nothing
# in git itself limits what such a push contains. This is the limit: a push
# that did not arrive by merging a pull request may change checked.json only.
# If it changed anything else, those changes are undone in a follow-up commit
# and the job fails, so the push shows red on main and nobody has to notice it
# by reading history.
#
# Usage: stamp-guard.sh <commit before the push> <commit after the push>
# Env:   MERGED=true when the pushed head came in through a merged pull request
#        (any such push is the normal, reviewed path and is left alone).
set -euo pipefail

before=${1:?commit before the push}
after=${2:?commit after the push}
allowed=checked.json

if [ "${MERGED:-false}" = true ]; then
  echo "Arrived through a merged pull request; nothing to guard."
  exit 0
fi

# A push that creates the branch has no "before" to compare with.
if [ "$before" = 0000000000000000000000000000000000000000 ]; then
  echo "::error::main was created or reset by a direct push; cannot tell what it changed."
  exit 1
fi
git cat-file -e "$before^{commit}" 2>/dev/null || {
  echo "::error::previous main ($before) is not in this clone; the push rewrote history."
  exit 1
}

# --no-renames lists both sides of a rename, so a file moved onto or off
# another path cannot hide behind the allowed one. -z keeps every path intact,
# whatever spaces or non-ASCII characters it holds.
others=()
while IFS= read -r -d '' f; do
  [ "$f" = "$allowed" ] || others+=("$f")
done < <(git diff -z --no-renames --name-only "$before" "$after")
if [ ${#others[@]} -eq 0 ]; then
  echo "Direct push changes only $allowed (or nothing)."
  exit 0
fi

echo "::error::A direct push to main changed files other than $allowed:"
printf '  %s\n' "${others[@]}"

# Only undo on top of the very commit that was pushed: if main has moved on
# since, someone built on this push, and rewinding files under them is not this
# job's call. Failing loudly is enough then.
git fetch --quiet origin main
if [ "$(git rev-parse origin/main)" != "$after" ]; then
  echo "::error::main has moved past $after; not reverting. Undo these changes by hand."
  exit 1
fi

git checkout --quiet --detach "$after"
for f in "${others[@]}"; do
  if git cat-file -e "$before:$f" 2>/dev/null; then
    git checkout "$before" -- "$f"
  else
    git rm --quiet -f -- "$f"
  fi
done
git config user.name "stamp-guard"
git config user.email "stamp-guard@users.noreply.github.com"
git commit --quiet -m "stamp-guard: undo changes outside $allowed from ${after:0:7}"
git push origin HEAD:main
echo "::error::Reverted. Changes to anything but $allowed must go through a pull request."
exit 1
