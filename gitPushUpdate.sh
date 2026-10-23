#!/bin/bash
set -euo pipefail

# Use this repository even when invoked from another directory.
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
git rev-parse --git-dir >/dev/null
branch=$(git symbolic-ref --quiet --short HEAD) || {
    echo "Check out a branch before pushing." >&2
    exit 1
}
git remote get-url origin >/dev/null

# SSH uses the identity configured for origin in ~/.ssh/config.
# Never clear the SSH agent or change global Git settings here.
last_commit=$(git log -1 --format=%s)
message=${1:-}
tag=""
if [[ -z "$message" ]]; then
    if [[ "$last_commit" =~ ^[0-9]+$ ]]; then
        number=$((10#$last_commit + 1))
    else
        number=1
    fi
    while git show-ref --verify --quiet "refs/tags/v1.0.$number"; do
        number=$((number + 1))
    done
    message=$number
fi
if [[ "$message" =~ ^[0-9]+$ ]]; then
    tag="v1.0.$message"
fi

git add -A
if ! git diff --cached --quiet; then
    if [[ -n "$tag" ]] && git show-ref --verify --quiet "refs/tags/$tag"; then
        echo "Tag $tag already exists; choose another commit number." >&2
        exit 1
    fi
    git commit -m "$message"
    if [[ -n "$tag" ]]; then
        git tag "$tag"
    fi
else
    echo "No new changes to commit; pushing existing commits."
fi

# Include version tags on HEAD so retrying a failed push also retries its tag.
refs=("$branch")
while IFS= read -r head_tag; do
    [[ -z "$head_tag" ]] || refs+=("refs/tags/$head_tag")
done < <(git tag --points-at HEAD --list 'v1.0.*')
git push --atomic --set-upstream origin "${refs[@]}"
