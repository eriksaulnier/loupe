#!/bin/sh
# Installs the loupe binary from a GitHub release:
#
#   curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | sh
#
# Then it installs or updates the loupe plugin, at the same release, for each of Claude Code, Codex and Pi on PATH.
#
# LOUPE_VERSION pins a release. LOUPE_INSTALL_DIR replaces the default directory, $HOME/.local/bin. LOUPE_NO_PLUGINS,
# like CI, skips the plugins. docs/install.md describes all three.
#
# LOUPE_UNINSTALL, which loupe uninstall sets, removes the plugins and then the binary instead, and downloads nothing.
#
# All logic runs from main on the last line, so a download cut short runs nothing.

set -eu

releases=https://github.com/eriksaulnier/loupe/releases
issues=https://github.com/eriksaulnier/loupe/issues
plugin_docs=https://github.com/eriksaulnier/loupe/blob/main/docs/install.md#the-agent-plugin

tmp=
staged=
failed_agents=0
plugin_action='set up the loupe plugin for'

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

# The binary is already in place and the other agents still run, so this returns instead of exiting.
agent_failed() {
	[ -z "$2" ] || printf '%s\n' "$2" >&2
	printf 'error: could not %s %s\nfix: run %s\n' "$plugin_action" "$1" "$3" >&2
	failed_agents=1
}

# Each agent prints JSON over many lines. Joined and split at each brace, an object's top-level fields share a line,
# so plain case patterns can read it without jq. The next line belongs to the entry only when this one ends in a
# colon, which opens a nested object such as Codex's source. In Claude Code's flat list it is another marketplace.
loupe_entry() {
	printf '%s' "$1" | tr -d ' \t\r\n' | tr '{' '\n' | awk '/"name":"loupe"/ { print; if (/:$/ && getline > 0) print; exit }'
}

sync_claude() {
	manual="claude plugin marketplace remove loupe; claude plugin marketplace add $plugin_source && claude plugin install loupe@loupe"
	out=$(claude plugin marketplace list --json 2>&1) || {
		agent_failed "Claude Code" "$out" "$manual"
		return 0
	}
	entry=$(loupe_entry "$out")
	case "$entry" in
	*'"source":"directory"'*)
		printf 'Claude Code: skipped, its loupe marketplace is a local directory\n'
		return 0
		;;
	esac
	case "$entry" in
	"") add=1 ;;
	*'"repo":"eriksaulnier/loupe"'*)
		case "$entry" in
		*"\"ref\":\"$tag\""*) add= ;;
		*) add=remove ;;
		esac
		;;
	*) add=remove ;;
	esac
	# Removing the marketplace also uninstalls the plugin, so only a source that has to change is removed.
	if [ "$add" = remove ]; then
		out=$(claude plugin marketplace remove loupe 2>&1) || {
			agent_failed "Claude Code" "$out" "$manual"
			return 0
		}
	fi
	if [ -n "$add" ]; then
		out=$(claude plugin marketplace add "$plugin_source" 2>&1) || {
			agent_failed "Claude Code" "$out" "$manual"
			return 0
		}
	fi
	out=$(claude plugin install loupe@loupe 2>&1) || {
		agent_failed "Claude Code" "$out" "$manual"
		return 0
	}
	printf 'Claude Code: loupe plugin %s. Restart running sessions to load it.\n' "$tag"
}

# Codex lists no ref, so the add itself says whether the source differs. Only that answer removes anything: a message
# this script does not know fails loudly instead of losing the plugin.
sync_codex() {
	manual="codex plugin marketplace remove loupe; codex plugin marketplace add $plugin_source && codex plugin add loupe@loupe"
	out=$(codex plugin marketplace list --json 2>&1) || {
		agent_failed "Codex" "$out" "$manual"
		return 0
	}
	case "$(loupe_entry "$out")" in
	*'"sourceType":"local"'*)
		printf 'Codex: skipped, its loupe marketplace is a local directory\n'
		return 0
		;;
	esac
	if ! out=$(codex plugin marketplace add "$plugin_source" 2>&1); then
		case "$out" in
		*"different source"*) ;;
		*)
			agent_failed "Codex" "$out" "$manual"
			return 0
			;;
		esac
		out=$(codex plugin marketplace remove loupe 2>&1) || {
			agent_failed "Codex" "$out" "$manual"
			return 0
		}
		out=$(codex plugin marketplace add "$plugin_source" 2>&1) || {
			agent_failed "Codex" "$out" "$manual"
			return 0
		}
	fi
	out=$(codex plugin add loupe@loupe 2>&1) || {
		agent_failed "Codex" "$out" "$manual"
		return 0
	}
	printf 'Codex: loupe plugin %s. Restart running sessions to load it.\n' "$tag"
}

# pi install replaces any earlier loupe package, whatever form its source took.
sync_pi() {
	manual="pi install git:github.com/$plugin_source"
	out=$(pi install "git:github.com/$plugin_source" 2>&1) || {
		agent_failed "Pi" "$out" "$manual"
		return 0
	}
	printf 'Pi: loupe plugin %s\n' "$tag"
}

# Pins each agent's plugin to the tag of the binary just installed, so a skill never names a flag the binary lacks.
sync_plugins() {
	if [ -n "${CI:-}" ] || [ -n "${LOUPE_NO_PLUGINS:-}" ]; then
		printf 'Skipped the agent plugins. To add them, see %s\n' "$plugin_docs"
		return 0
	fi
	tag=v$version
	plugin_source=eriksaulnier/loupe@$tag
	found=
	if command -v claude >/dev/null 2>&1; then
		found=1
		sync_claude
	fi
	if command -v codex >/dev/null 2>&1; then
		found=1
		sync_codex
	fi
	if command -v pi >/dev/null 2>&1; then
		found=1
		sync_pi
	fi
	[ -n "$found" ] || printf 'No Claude Code, Codex or Pi found. To add the agent plugin later, see %s\n' "$plugin_docs"
	[ "$failed_agents" = 0 ] || exit 1
}

remove_claude() {
	manual="claude plugin marketplace remove loupe"
	out=$(claude plugin marketplace list --json 2>&1) || {
		agent_failed "Claude Code" "$out" "$manual"
		return 0
	}
	entry=$(loupe_entry "$out")
	case "$entry" in
	"")
		printf 'Claude Code: no loupe plugin\n'
		return 0
		;;
	*'"source":"directory"'*)
		printf 'Claude Code: skipped, its loupe marketplace is a local directory\n'
		return 0
		;;
	esac
	out=$(claude plugin marketplace remove loupe 2>&1) || {
		agent_failed "Claude Code" "$out" "$manual"
		return 0
	}
	printf 'Claude Code: removed the loupe plugin\n'
}

remove_codex() {
	manual="codex plugin marketplace remove loupe"
	out=$(codex plugin marketplace list --json 2>&1) || {
		agent_failed "Codex" "$out" "$manual"
		return 0
	}
	entry=$(loupe_entry "$out")
	case "$entry" in
	"")
		printf 'Codex: no loupe plugin\n'
		return 0
		;;
	*'"sourceType":"local"'*)
		printf 'Codex: skipped, its loupe marketplace is a local directory\n'
		return 0
		;;
	esac
	out=$(codex plugin marketplace remove loupe 2>&1) || {
		agent_failed "Codex" "$out" "$manual"
		return 0
	}
	printf 'Codex: removed the loupe plugin\n'
}

# pi remove matches the package whatever form its source took, and fails when there is none, so the list decides.
remove_pi() {
	manual="pi remove git:github.com/eriksaulnier/loupe"
	out=$(pi list 2>&1) || {
		agent_failed "Pi" "$out" "$manual"
		return 0
	}
	case "$out" in
	*github.com/eriksaulnier/loupe*) ;;
	*)
		printf 'Pi: no loupe plugin\n'
		return 0
		;;
	esac
	out=$(pi remove git:github.com/eriksaulnier/loupe 2>&1) || {
		agent_failed "Pi" "$out" "$manual"
		return 0
	}
	printf 'Pi: removed the loupe plugin\n'
}

# loupe uninstall runs this mode. The plugins go first, so a failure leaves the binary that can retry.
uninstall() {
	choose_dir
	plugin_action='remove the loupe plugin from'
	if [ -z "${CI:-}" ] && [ -z "${LOUPE_NO_PLUGINS:-}" ]; then
		! command -v claude >/dev/null 2>&1 || remove_claude
		! command -v codex >/dev/null 2>&1 || remove_codex
		! command -v pi >/dev/null 2>&1 || remove_pi
	fi
	[ "$failed_agents" = 0 ] || exit 1
	# -L too, because mise run link leaves a symlink whose target may be gone.
	if [ -e "$dir/loupe" ] || [ -L "$dir/loupe" ]; then
		rm -f "$dir/loupe" || fail "could not remove $dir/loupe" "Check the permissions of $dir."
		printf 'Removed %s/loupe\n' "$dir"
	else
		printf 'No loupe binary at %s/loupe\n' "$dir"
	fi
}

main() {
	if [ -n "${LOUPE_UNINSTALL:-}" ]; then
		uninstall
		return
	fi
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
	sync_plugins
}

main "$@"
