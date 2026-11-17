#!/usr/bin/env bash
#
# Zev uninstaller for Linux and macOS — removes the binary and optionally user data.
# Set ZEV_REMOVE_CONFIG=1 to explicitly remove ~/.zev without prompting.
#
#   curl -fsSL https://raw.githubusercontent.com/ra1phdd/zev/main/deploy/uninstall.sh | bash
#   PREFIX=$HOME/.local/bin curl -fsSL .../uninstall.sh | bash

set -euo pipefail


info() { printf '\033[0;36m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[0;33mwarning:\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[0;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# Find the installed binary. An explicit PREFIX is authoritative; otherwise look in
# the system-wide directory first and fall back to the user-local one, so an install
# made without root is still found when sudo happens to be available now.
binary=""
if [ -n "${PREFIX:-}" ]; then
	binary="${PREFIX}/zev"
else
	for candidate in /usr/local/bin/zev "$HOME/.local/bin/zev"; do
		if [ -f "$candidate" ]; then
			binary="$candidate"
			PREFIX="$(dirname "$candidate")"
			break
		fi
	done
fi

if [ -z "$binary" ] || [ ! -f "$binary" ]; then
	warn "zev is not installed in /usr/local/bin or $HOME/.local/bin — nothing to remove"
	exit 0
fi

# Only the system-wide directory needs elevated privileges to modify.
if [ "$PREFIX" = /usr/local/bin ] && [ ! -w "$PREFIX" ] && command -v sudo >/dev/null 2>&1; then
	SUDO="sudo"
else
	SUDO=""
fi

config_dir=""
if [ -n "${HOME:-}" ]; then
	config_dir="$HOME/.zev"
fi
remove_config=false
if [ -n "$config_dir" ] && [ -d "$config_dir" ]; then
	if [ "${ZEV_REMOVE_CONFIG:-}" = "1" ]; then
		remove_config=true
	elif { exec 3<> /dev/tty; } 2>/dev/null; then
		printf 'Remove configuration and user data in %s? This deletes history, aliases and scripts [y/N] ' "$config_dir" >&3
		if IFS= read -r answer <&3; then
			case "$answer" in
				y|Y|yes|YES|Yes) remove_config=true ;;
				*) info "Keeping $config_dir" ;;
			esac
		else
			info "Keeping $config_dir (no confirmation received)"
		fi
		exec 3>&-
	else
		info "Keeping $config_dir (non-interactive uninstall)"
	fi
fi

info "Removing $binary"

if [ -n "$SUDO" ]; then
	$SUDO rm -f "$binary"
else
	rm -f "$binary"
fi

if [ "$remove_config" = true ]; then
	info "Removing $config_dir"
	rm -rf -- "$config_dir"
fi

info "Uninstalled"

# Remove the installation directory if it's now empty.
if [ -d "$PREFIX" ] && [ -z "$(ls -A "$PREFIX")" ]; then
	info "Installation directory is empty, removing $PREFIX"
	if [ -n "$SUDO" ]; then
		$SUDO rmdir "$PREFIX" 2>/dev/null || true
	else
		rmdir "$PREFIX" 2>/dev/null || true
	fi
fi
