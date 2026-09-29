#!/usr/bin/env bash
# Tags the enterprise repository with this release's version, on the commit
# the enterprise submodule pins at HEAD, so that a host can require
# github.com/TykTechnologies/ai-studio-enterprise/v2 at the same version as
# github.com/TykTechnologies/midsommar/v2.
#
#   GH_TOKEN=<token with contents:write on ai-studio-enterprise> \
#     scripts/release/tag-enterprise.sh v2.3.0
#
# It refuses to tag a pin that is not on the enterprise repository's main
# branch (a core PR merged while pinned to an open enterprise PR's branch),
# and is idempotent: an existing tag on the same commit is fine, one on a
# different commit is an error.
set -euo pipefail

TAG=${1:?usage: $0 <tag>}
REPO=TykTechnologies/ai-studio-enterprise

SHA=$(git ls-tree HEAD enterprise | awk '$2 == "commit" {print $3}')
if [ -z "$SHA" ]; then
  echo "tag-enterprise: HEAD has no enterprise submodule entry" >&2
  exit 1
fi

status=$(gh api "repos/$REPO/compare/main...$SHA" --jq .status)
case "$status" in
  identical|behind) ;;
  *)
    echo "tag-enterprise: the pinned enterprise commit $SHA is not on $REPO main (compare status: $status); re-pin the submodule to an enterprise main commit before releasing" >&2
    exit 1
    ;;
esac

if existing=$(gh api "repos/$REPO/git/ref/tags/$TAG" --jq '.object.sha + " " + .object.type' 2>/dev/null); then
  read -r obj type <<< "$existing"
  if [ "$type" = tag ]; then
    obj=$(gh api "repos/$REPO/git/tags/$obj" --jq .object.sha)
  fi
  if [ "$obj" = "$SHA" ]; then
    echo "tag-enterprise: $REPO already has $TAG on $SHA"
    exit 0
  fi
  echo "tag-enterprise: $REPO already has $TAG on $obj, not the pinned $SHA" >&2
  exit 1
fi

gh api -X POST "repos/$REPO/git/refs" -f "ref=refs/tags/$TAG" -f "sha=$SHA" >/dev/null
echo "tag-enterprise: tagged $REPO $TAG on $SHA"
