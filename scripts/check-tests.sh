#!/usr/bin/env bash
# Fails when tests could reach a real terminal or network, or when a review could be posted from outside the one path
# that records an attempt first. git grep sees only tracked and staged files, so a new file must be added to be checked.
#
# Test files are *_test.go and everything under internal/testutil/. URL literals in them MUST be one of:
#   - http://127.0.0.1 or http://localhost, with any port and path (httptest servers)
#   - https://github.com/<owner>/<repo>..., which names a repository and is never fetched: Git reaches a local bare
#     remote through insteadOf, and the GitHub client talks to fakegh
#   - https://api.github.com..., only under internal/github/ and internal/testutil/fakegh/, where the fake's transport
#     rewrites it to the local server
# and these exact literals, which are inputs to parsers and never fetched:
#   - internal/cli/capture_test.go: http://github.com/o/r/pull/12 and https://gitlab.com/o/r/pull/12 (rejected URLs)
#   - internal/gitx/gitx_test.go: https://gitlab.com/o/r, https://github.com/o, and https://github.com/.insteadOf (a Git
#     config key)
#   - internal/github/client_test.go: https://github.com/<owner>/<repo>/pull/<number> (a fix text placeholder)
#   - internal/markdown/allowlist_test.go: https://example.com (a link in Markdown input)
# A bare api.github.com is allowed only in the refusal fix text "check network access to api.github.com".
set -eu

cd "$(git rev-parse --show-toplevel)"

failed=0

# git grep exits 1 when nothing matches; anything above that is a real failure.
tracked_grep() {
	local status=0
	git grep "$@" || status=$?
	if [ "$status" -gt 1 ]; then
		echo "check-tests: git grep $* failed with status $status" >&2
		return 2
	fi
}

violation() {
	printf 'check-tests: %s\n%s\n' "$1" "$2" >&2
	failed=1
}

tests=('*_test.go' 'internal/testutil/')

pty=$(tracked_grep -nE '"[^"]*creack/pty[^"]*"|"github\.com/[^"]+/pty(/[^"]*)?"|exec\.Command(Context)?\([^)]*"script"' -- "${tests[@]}")
if [ -n "$pty" ]; then
	violation "tests MUST NOT use a pseudo-terminal:" "$pty"
fi

urls=$(tracked_grep -noE "https?://[^[:space:]\"'\`)<>]*" -- "${tests[@]}")
bad_urls=""
while IFS= read -r hit; do
	[ -n "$hit" ] || continue
	file=${hit%%:*}
	url=${hit#*:*:}
	case "$url" in
	http://127.0.0.1 | http://127.0.0.1[:/]* | http://localhost | http://localhost[:/]*) continue ;;
	https://github.com/?*/?*) continue ;;
	https://api.github.com*)
		case "$file" in internal/github/* | internal/testutil/fakegh/*) continue ;; esac
		;;
	esac
	case "$file $url" in
	"internal/cli/capture_test.go http://github.com/o/r/pull/12" | \
		"internal/cli/capture_test.go https://gitlab.com/o/r/pull/12" | \
		"internal/gitx/gitx_test.go https://gitlab.com/o/r" | \
		"internal/gitx/gitx_test.go https://github.com/o" | \
		"internal/gitx/gitx_test.go https://github.com/.insteadOf" | \
		"internal/github/client_test.go https://github.com/" | \
		"internal/markdown/allowlist_test.go https://example.com")
		continue
		;;
	esac
	bad_urls+="$hit"$'\n'
done <<<"$urls"
if [ -n "$bad_urls" ]; then
	violation "tests MUST NOT name a network host other than loopback or a github.com repository:" "${bad_urls%$'\n'}"
fi

api=$(tracked_grep -n 'api\.github\.com' -- "${tests[@]}" ':(exclude)internal/github/' ':(exclude)internal/testutil/fakegh/')
api=$(printf '%s' "$api" | sed '/check network access to api\.github\.com/d')
if [ -n "$api" ]; then
	violation "api.github.com MUST appear in tests only under internal/github/ and internal/testutil/fakegh/:" "$api"
fi

creates=$(tracked_grep -n 'CreateReview(' -- '*.go' ':(exclude)internal/publish/publish.go' ':(exclude)internal/github/' \
	':(exclude)internal/testutil/fakegh/')
if [ -n "$creates" ]; then
	violation "CreateReview MUST be called only from internal/publish/publish.go:" "$creates"
fi

exit "$failed"
