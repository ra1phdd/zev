#!/usr/bin/env bash
#
# Zev installer for Linux and macOS — downloads a precompiled binary and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
#
#   PREFIX=$HOME/.local/bin curl -fsSL .../install.sh | bash   # install without root
#
# See install.ps1 for Windows.

set -euo pipefail

REPO="ra1phdd/zev"
BRANCH="main"
GITHUB_URL="https://github.com/${REPO}"
RAW_URL="https://raw.githubusercontent.com/${REPO}/${BRANCH}"

info() { printf '\033[0;36m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[0;33mwarning:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[0;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

usage() {
	cat <<'EOF'
Zev installer for Linux and macOS — downloads a precompiled binary and installs it.

Usage:
  curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/install.sh | bash -s -- -v 1.0.0

Options:
  -v <version>   Install a specific version (default: read from the VERSION file
                 in the repository).
  -h             Show this help.

Environment:
  PREFIX         Installation directory. By default installs into /usr/local/bin
                 with sudo, or $HOME/.local/bin when there is no sudo.
  ZEV_VERSION    Same as -v, takes precedence over VERSION when set.

Windows users: run deploy/install.ps1 instead.
EOF
	exit 0
}

VERSION=""
while [ $# -gt 0 ]; do
	case "$1" in
		-v|--version) VERSION="${2:?--version needs a value}"; shift 2 ;;
		-h|--help) usage ;;
		*) die "unknown argument: $1 (try --help)" ;;
	esac
done

if [ -z "$VERSION" ]; then
	VERSION="${ZEV_VERSION:-}"
fi

# Prefer curl, fall back to wget, and fail loudly when neither is available.
DOWNLOADER=""
if command -v curl >/dev/null 2>&1; then
	DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
	DOWNLOADER="wget"
else
	die "neither curl nor wget is installed - install one of them and retry"
fi

download() {
	case "$DOWNLOADER" in
		curl) curl -fsSL "$1" -o "$2" ;;
		wget) wget -qO "$2" "$1" ;;
	esac
}

# Resolve the version from the repository VERSION file, so the installer and the
# release workflow can never disagree.
if [ -z "$VERSION" ]; then
	info "Resolving latest version from ${RAW_URL}/VERSION"
	VERSION="$(download "${RAW_URL}/VERSION" /dev/stdout 2>/dev/null | tr -d '[:space:]')" \
		|| VERSION=""
	if [ -z "$VERSION" ]; then
		die "could not read VERSION from ${RAW_URL}/VERSION — pass a version explicitly: bash -s -- -v <version>"
	fi
fi

info "Installing zev ${VERSION}"

# Resolve the archive name for this platform. Note: macOS ships bash 3.2, so no
# bash 4 features such as ${var,,}.
arch=""
case "$(uname -s)" in
	Linux)  os_slug=linux ;;
	Darwin) os_slug=darwin ;;
	*)      die "unsupported OS: $(uname -s) (Windows users: run deploy/install.ps1)" ;;
esac

case "$(uname -m)" in
	x86_64|amd64)  arch=amd64 ;;
	aarch64|arm64) arch=arm64 ;;
	armv7l|armv7)  arch=armv7 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
esac

# GoReleaser publishes armv7 for Linux only.
if [ "$arch" = armv7 ] && [ "$os_slug" != linux ]; then
	die "no precompiled binaries for ${os_slug}/${arch}"
fi

tarball="zev_${VERSION}_${os_slug}_${arch}.tar.gz"
base_url="${GITHUB_URL}/releases/download/v${VERSION}"

# Pick the install directory: an explicit PREFIX wins. Without it, install into the
# system-wide /usr/local/bin using sudo, or fall back to a user-owned directory when
# running without root.
SUDO=""
if [ -z "${PREFIX:-}" ]; then
	if [ "$(id -u)" -eq 0 ]; then
		PREFIX=/usr/local/bin
	elif command -v sudo >/dev/null 2>&1; then
		PREFIX=/usr/local/bin
		SUDO="sudo"
	else
		PREFIX="$HOME/.local/bin"
	fi
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "Downloading ${tarball}"
download "${base_url}/${tarball}" "${tmp}/${tarball}" \
	|| die "download failed: ${base_url}/${tarball}"

# Verify the archive against the published checksums before unpacking.
if download "${base_url}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null; then
	if ! (cd "$tmp" && grep " ${tarball}\$" checksums.txt > "${tarball}.sha256"); then
		die "no checksum published for ${tarball}"
	fi

	if command -v sha256sum >/dev/null 2>&1; then
		(cd "$tmp" && sha256sum -c "${tarball}.sha256" >/dev/null) \
			|| die "checksum mismatch for ${tarball} — refusing to install"
	elif command -v shasum >/dev/null 2>&1; then
		(cd "$tmp" && shasum -a 256 -c "${tarball}.sha256" >/dev/null) \
			|| die "checksum mismatch for ${tarball} — refusing to install"
	else
		warn "no sha256sum or shasum found — skipping integrity check"
	fi
	info "Checksum verified"
else
	warn "checksums.txt unavailable — skipping integrity check"
fi

info "Extracting"
tar -xzf "${tmp}/${tarball}" -C "$tmp" zev || die "failed to extract zev from ${tarball}"

# Install through sudo when we picked a system-wide directory, otherwise write
# directly (an explicit PREFIX is always honoured as-is).
if [ -n "$SUDO" ]; then
	$SUDO install -d -m 0755 "$PREFIX"
	$SUDO install -m 0755 "${tmp}/zev" "${PREFIX}/zev"
else
	mkdir -p "$PREFIX" || die "cannot create ${PREFIX} - retry with: PREFIX=\$HOME/.local/bin"
	install -m 0755 "${tmp}/zev" "${PREFIX}/zev"
fi

info "Installed: $(command -v zev 2>/dev/null || echo "${PREFIX}/zev")"

case ":${PATH}:" in
	*":${PREFIX}:"*) ;;
	*) warn "${PREFIX} is not in your PATH — add it: export PATH=\"${PREFIX}:\$PATH\"" ;;
esac

cat <<EOF

Next steps:

  zev setup                          configure your LLM provider
  zev "find large files here"        get a command suggestion

EOF
