#!/usr/bin/env bash
# Publish checksums to a draft before archives become available to installers.
set -euo pipefail

check_only=false
if [[ "${1:-}" = --check ]]; then
	check_only=true
	shift
fi
version="${1:?usage: publish-release.sh [--check] VERSION}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || {
	printf 'invalid version\n' >&2
	exit 1
}
[[ -s dist/checksums.txt ]] || { printf 'checksums.txt is missing\n' >&2; exit 1; }
shopt -s nullglob
archives=(dist/*.tar.gz dist/*.zip)
((${#archives[@]} > 0)) || { printf 'no release archives found\n' >&2; exit 1; }
for archive in "${archives[@]}"; do
	name="${archive#dist/}"
	count="$(awk -v name="$name" '$2 == name { n++ } END { print n+0 }' dist/checksums.txt)"
	[[ "$count" = 1 ]] || { printf 'missing or duplicate checksum for %s\n' "$name" >&2; exit 1; }
done
(cd dist && sha256sum --strict -c checksums.txt)

if [[ "$check_only" = true ]]; then
	printf 'release artifacts and checksums verified for v%s\n' "$version"
	exit 0
fi

gh release create "v$version" dist/checksums.txt --draft --verify-tag --title "v$version" --generate-notes
gh release upload "v$version" "${archives[@]}"
gh release edit "v$version" --draft=false
