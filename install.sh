#!/bin/sh
# Installs the loupe binary from a GitHub release:
#
#   curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | sh
#
# LOUPE_VERSION pins a release. LOUPE_INSTALL_DIR replaces the default directory, $HOME/.local/bin.
# docs/install.md describes both.
#
# All logic runs from main on the last line, so a download cut short runs nothing.

set -eu

releases=https://github.com/eriksaulnier/loupe/releases
issues=https://github.com/eriksaulnier/loupe/issues
plugin_docs=https://github.com/eriksaulnier/loupe/blob/main/docs/install.md#the-agent-plugin

tmp=
staged=

fail() {
	printf 'error: %s\nfix: %s\n' "$1" "$2" >&2
	exit 1
}

warn() {
	printf 'warning: %s\n' "$1" >&2
}

cleanup() {
	[ -z "$tmp" ] || rm -rf "$tmp"
	[ -z "$staged" ] || rm -f "$staged"
}

fetch() {
	curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$2" "$1" ||
		fail "could not download $1" "Check your network, and check that the release exists at $releases. A release made in the last few minutes may still be uploading."
}

detect_platform() {
	system=$(uname -s)
	case "$system" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "loupe has no build for $system" "Run this on Linux or macOS." ;;
	esac

	machine=$(uname -m)
	case "$machine" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "loupe has no build for $machine" "Run this on amd64 or arm64." ;;
	esac
}

choose_dir() {
	dir=${LOUPE_INSTALL_DIR:-}
	if [ -z "$dir" ]; then
		[ -n "${HOME:-}" ] || fail "HOME is not set" "Set LOUPE_INSTALL_DIR to the directory to install into."
		dir=$HOME/.local/bin
	fi
	# mv would put the binary inside the directory and report success.
	if [ -d "$dir/loupe" ]; then
		fail "$dir/loupe is a directory" "Remove it, or set LOUPE_INSTALL_DIR to another directory."
	fi
}

# Sets version, archive and digest from the one line of checksums.txt that names this platform's archive. With no pin
# the archive name is what gives the version, so latest needs no API call and meets no rate limit.
find_archive() {
	pattern="^loupe_${version:-[^_]+}_${os}_${arch}[.]tar[.]gz$"
	entry=$(awk -v pattern="$pattern" '$2 ~ pattern { n++; line = $1 " " $2 } END { if (n == 1) print line }' "$1")
	[ -n "$entry" ] ||
		fail "checksums.txt does not name exactly one archive for ${os}_${arch}" "Check the release's files at $releases."
	digest=${entry%% *}
	archive=${entry#* }
	version=${archive#loupe_}
	version=${version%_"${os}"_"${arch}".tar.gz}
}

# Reads the file from stdin because both tools escape the first field of their output for a path with a backslash.
verify() {
	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum <"$1")
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 <"$1")
	else
		fail "neither sha256sum nor shasum is on PATH, so the download cannot be verified" "Install one of them, then run this script again."
	fi
	[ "${actual%% *}" = "$digest" ] ||
		fail "the checksum of $archive does not match checksums.txt" "Run this script again. If it fails again, report it at $issues."
}

# Runs the binary before the move, so one that cannot start never replaces one that can. The target directory holds
# the staged copy, because mv is atomic only inside one file system, and a noexec mount fails here too.
install_binary() {
	mkdir -p "$dir" || fail "could not create $dir" "Set LOUPE_INSTALL_DIR to a directory you can write to."
	# A relative or slash-ended setting would make the PATH checks and the suggested PATH line wrong.
	dir=$(CDPATH='' cd -- "$dir" && pwd) || fail "could not enter $dir" "Set LOUPE_INSTALL_DIR to a directory you can write to."
	staged=$(mktemp "$dir/.loupe.XXXXXX") ||
		fail "could not write to $dir" "Set LOUPE_INSTALL_DIR to a directory you can write to."
	cp "$tmp/loupe" "$staged" || fail "could not write to $dir" "Check the free space in $dir."
	chmod 755 "$staged" || fail "could not make $staged executable" "Check the permissions of $dir."
	installed=$("$staged" --version) ||
		fail "the new binary failed loupe --version" "Check that $dir allows executables, or set LOUPE_INSTALL_DIR to another directory."
	mv -f "$staged" "$dir/loupe" || fail "could not replace $dir/loupe" "Check the permissions of $dir."
	staged=
}

warn_about_path() {
	case ":$PATH:" in
	*":$dir:"* | *":$dir/:"*) ;;
	*) warn "$dir is not on your PATH. Add this line to your shell profile: export PATH=\"$dir:\$PATH\"" ;;
	esac

	found=$(command -v loupe || true)
	# command -v in dash and zsh prints a double slash for a PATH entry with a trailing slash.
	case "$found" in
	"" | "$dir/loupe" | "$dir//loupe") ;;
	*) warn "loupe resolves to $found, not $dir/loupe. Remove that copy, or put $dir first on your PATH." ;;
	esac
}

main() {
	detect_platform
	choose_dir

	# EXIT does not run on death by signal in every sh, so a signal exits through it.
	trap cleanup EXIT
	trap 'exit 1' HUP INT TERM
	# macOS mktemp ignores TMPDIR unless the template names the directory.
	tmp=$(mktemp -d "${TMPDIR:-/tmp}/loupe.XXXXXX") ||
		fail "could not create a temporary directory" "Set TMPDIR to a directory you can write to."

	version=${LOUPE_VERSION:-}
	version=${version#v}
	if [ -n "$version" ]; then
		fetch "$releases/download/v$version/checksums.txt" "$tmp/checksums.txt"
	else
		fetch "$releases/latest/download/checksums.txt" "$tmp/checksums.txt"
	fi
	find_archive "$tmp/checksums.txt"

	# By tag, not through latest: a release published since the checksums would have moved latest off this archive.
	fetch "$releases/download/v$version/$archive" "$tmp/$archive"
	verify "$tmp/$archive"
	tar -xzf "$tmp/$archive" -C "$tmp" loupe ||
		fail "could not extract loupe from $archive" "Run this script again. If it fails again, report it at $issues."

	install_binary

	printf 'Installed %s at %s/loupe\n' "$installed" "$dir"
	warn_about_path
	printf 'Next, add the agent plugin, which a human reviewer also needs: %s\n' "$plugin_docs"
}

main "$@"
