#!/usr/bin/env bash
# Run every release gate before publishing the tag or GitHub release.
set -euo pipefail

version="${1:?usage: release.sh VERSION}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || {
	printf 'invalid version\n' >&2
	exit 1
}
tag="v$version"

make fmt
make lint
make test
make test-race
make build VERSION="$version"
# Formatters and lint fixes must not change the commit being released.
git diff --exit-code HEAD

if git rev-parse "$tag" >/dev/null 2>&1; then
	test "$(git rev-list -n 1 "$tag")" = "$(git rev-parse HEAD)"
else
	git tag -a "$tag" -m "Release $tag"
fi

goreleaser release --clean --skip=publish
# Hooks must not silently change the sources after tests have passed.
git diff --exit-code HEAD
bash deploy/publish-release.sh --check "$version"

git push origin "$tag"
bash deploy/publish-release.sh "$version"
