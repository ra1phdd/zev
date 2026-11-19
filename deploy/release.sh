#!/usr/bin/env bash
# Run every release gate before publishing the tag or GitHub release.
set -euo pipefail

version="${1:?usage: release.sh VERSION}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || {
	printf 'invalid version\n' >&2
	exit 1
}
file_version="$(tr -d '[:space:]' < VERSION)"
[[ "$file_version" == "$version" ]] || {
	printf 'VERSION file (%s) does not match requested release (%s)\n' "$file_version" "$version" >&2
	exit 1
}
tag="v$version"
# Make release inputs independent of ignored workspace/vendor state.
export GOWORK=off
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-mod=mod"

check_release_tree() {
	git diff --exit-code HEAD
	untracked_source="$(git ls-files --others --exclude-standard -- '*.go' '*.sh' '*.yml' '*.yaml' '*.toml' '*.txt' 'Makefile' 'go.mod' 'go.sum')"
	if [[ -n "$untracked_source" ]]; then
		printf 'untracked release source:\n%s\n' "$untracked_source" >&2
		exit 1
	fi
}

check_release_tree
make fmt
make lint
make test
make test-race
make test-e2e
make build VERSION="$version"
# Formatters and lint fixes must not change the commit being released.
check_release_tree

if git rev-parse "$tag" >/dev/null 2>&1; then
	test "$(git rev-list -n 1 "$tag")" = "$(git rev-parse HEAD)"
else
	git tag -a "$tag" -m "Release $tag"
fi

goreleaser release --clean --skip=publish
# Hooks must not silently change the sources after tests have passed.
check_release_tree
bash deploy/publish-release.sh --check "$version"

git push origin "$tag"
bash deploy/publish-release.sh "$version"
