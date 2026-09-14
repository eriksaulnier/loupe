#!/usr/bin/env bash
# Fails when tests could reach a real terminal or network, or when a review could be posted from outside the one path
# that records an attempt first. git grep sees only tracked and staged files, so a new file must be added to be checked.
#
# Test files are *_test.go and everything under internal/testutil/. URL literals in them MUST be one of:
#   - http://127.0.0.1 or http://localhost, optionally with a numeric port and a path (httptest servers)
#   - https://github.com/<owner>/<repo>..., which names a repository and is never fetched: Git reaches a local bare
#     remote through insteadOf, and the GitHub client talks to fakegh. Owners that are GitHub site paths and repo
#     paths that download content (archive, raw, releases, ...) are not allowed
# Matching ignores case and refuses any URL with userinfo, since http://localhost:x@host/ reaches host.
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

# Go splits a call across lines freely, so each test file is flattened to one line before matching. A match names the
# file and the matched text.
pty=""
while IFS= read -r -d '' file; do
	hits=$(tr '\n\t' '  ' <"$file" | tr -s ' ' | grep -oE \
		'"[^"]*(creack/pty|/x?pty)(/[^"]*)?"|"/dev/(ptmx|pts)[^"]*"|exec\.Command(Context)?\( ?([A-Za-z_][A-Za-z0-9_.]* ?, ?)?"([^"]*/)?(script|unbuffer|expect|socat)"' ||
		true)
	if [ -n "$hits" ]; then
		pty+="$file: $hits"$'\n'
	fi
done < <(git ls-files -z -- "${tests[@]}")
if [ -n "$pty" ]; then
	violation "tests MUST NOT use a pseudo-terminal:" "${pty%$'\n'}"
fi

# An import outside the test files reaches tests too, so the whole test build graph is checked for pseudo-terminal
# modules. A graph go list cannot load is a failure, not a pass.
if ! deps=$(go list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./... 2>&1); then
	violation "go list cannot load the test build graph:" "$deps"
elif pty_deps=$(printf '%s\n' "$deps" | grep -iE '(^|/)[^/]*(pty|expect|termtest)[^/]*(/|$)'); then
	violation "the test build MUST NOT depend on a pseudo-terminal module:" "$pty_deps"
fi

urls=$(tracked_grep -noiE "https?://[^[:space:]\"'\`)<>]*" -- "${tests[@]}")
bad_urls=""
while IFS= read -r hit; do
	[ -n "$hit" ] || continue
	file=${hit%%:*}
	url=${hit#*:*:}
	lower=$(printf '%s' "$url" | tr '[:upper:]' '[:lower:]')
	if [[ $lower != *@* ]]; then
		if [[ $lower =~ ^http://(127\.0\.0\.1|localhost)(:[0-9]+)?(/.*)?$ ]]; then
			continue
		fi
		if [[ $lower =~ ^https://github\.com/([a-z0-9%._-]+)/([a-z0-9%._-]+)(/([^/]*).*)?$ ]]; then
			case "${BASH_REMATCH[1]}" in
			login | logout | orgs | settings | apps | marketplace | sponsors | features | site | sessions | api) ;;
			*)
				case "${BASH_REMATCH[4]}" in
				archive | raw | releases | blob | tarball | zipball | info | git-upload-pack) ;;
				*) continue ;;
				esac
				;;
			esac
		fi
		if [[ $lower == https://api.github.com* ]]; then
			case "$file" in internal/github/* | internal/testutil/fakegh/*) continue ;; esac
		fi
	fi
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

# A word match also catches method values (post := c.CreateReview) and unformatted calls. The client declares it, and
# only the client's and the fake's own tests call it directly.
creates=$(tracked_grep -nw 'CreateReview' -- '*.go' ':(exclude)internal/publish/publish.go' \
	':(exclude)internal/github/client.go' ':(exclude)internal/github/client_test.go' \
	':(exclude)internal/testutil/fakegh/fakegh_test.go')
if [ -n "$creates" ]; then
	violation "CreateReview MUST be called only from internal/publish/publish.go:" "$creates"
fi
sends=$(tracked_grep -cw 'CreateReview' -- internal/publish/publish.go)
sends=${sends##*:}
if [ "${sends:-0}" != 1 ]; then
	violation "internal/publish/publish.go MUST reference CreateReview exactly once:" "found ${sends:-0}"
fi

exit "$failed"
