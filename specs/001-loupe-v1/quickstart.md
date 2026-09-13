# Quickstart: validating loupe v1

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Contract**: [contracts/cli.md](contracts/cli.md)

Two ways to prove the feature works: the automated suite, which never touches GitHub, and a manual walk against a pull request the owner names. Run the suite first; the manual walk is only for a pull request the owner explicitly asks about (constitution, Development Workflow).

## Prerequisites

- `mise trust && mise install` provides go, golangci-lint and goreleaser as pinned in `mise.toml`.
- `git` on `PATH`.
- For the manual walk only: `gh auth login --hostname github.com` completed, and a clone of the pull request's repository.

## Automated validation

```sh
mise run check          # go vet ./... && golangci-lint run && go test ./...
```

Expected: all three exit 0. The suite MUST include the scenarios below; each maps to a spec story or clarification. A task is not done until the row it implements is green.

| Scenario | Where | Proves |
| :--- | :--- | :--- |
| Capture leaves the clone untouched: `git status --porcelain`, `HEAD`, index checksum and `for-each-ref` identical before and after, apart from `refs/loupe/...` | `internal/integration` | US1 AS1, FR-002 |
| Located finding accepted, off-diff finding refused with nearest lines, batch with one bad entry stores nothing | `internal/diff`, `internal/draft`, `internal/integration` | US1 AS2 to AS4, FR-008, FR-010 |
| `summary --expect-findings 3` with two included findings refuses and lists ids and titles | `internal/draft` | US1 AS5, FR-013 |
| Every command's `--help` renders with no data root and shows the input shape | `internal/cli` | US1 AS6, FR-033 |
| TUI: list, open detail with hunk, accept, exclude, send back with note, quit, reopen with same dispositions | `internal/tui` (teatest) | US2 AS1 to AS7 |
| TUI: agent edits a finding while it is open; the decision is refused and the finding redisplays | `internal/tui` | US2 AS6, FR-023 |
| Plain mode under `TERM=dumb`, a 40x10 terminal, or `--plain`, with the same decisions from an injected reader | `internal/tui` | US2 AS8 |
| Publish: confirmation view, `v` toggles payload, `y` sends exactly one request, receipt written, second publish prints URL and sends nothing | `internal/integration` with `fakegh` | US3 AS1 to AS3, FR-027 to FR-029 |
| Publish refusals: no TTY, head moved, own PR with approve, approve with a blocking finding, pending finding, open note, empty draft, draft changed after display | `internal/publish`, `internal/integration` | US3 AS4 to AS9, FR-025, FR-028 |
| Send-back loop: note created in review, `feedback` lists it, `reply` attaches, edit clears acceptance, human resolves, readiness true; reply carrying a decision field refused | `internal/integration` | US4 |
| `review` with no argument resolves the current branch's pull request; `list` shows both runs newest first with state and counts | `internal/integration` | US5 |
| Round two after a published round one: linked, `show --previous` returns round one's published findings, footer names round two | `internal/integration` | US6 AS1 to AS3 |
| Capture at the same head: refused with `same-head` when the newest round is unpublished; new round when it is published | `internal/integration` | US6 AS5, AS6, clarification 1 |
| `show --previous` skips an unpublished round two and returns round one; refuses when nothing earlier was published | `internal/run`, `internal/integration` | US6 AS2, clarification 2 |
| Fake GitHub records the request then returns 500: attempt marked unknown; next publish matches the marker and writes a receipt without sending | `internal/integration` | US7 AS1, AS2 |
| Fake drops the request: next publish refuses with the PR URL; `--retry-unknown` shows fresh confirmation and sends once | `internal/integration` | US7 AS3, AS4 |
| 422 "one pending review": refusal's fix line says submit or discard on GitHub | `internal/publish` | US7 AS5, FR-030 |
| Rendered body and inline comments match golden files reproducing `docs/comment-format.md` examples; control and bidi characters escaped in previews only | `internal/render` | FR-026, FR-027 |
| Markdown allowlist: size, unclosed fence, stray `</details>`, HTML comment, nesting depth 16, each refused with code and line | `internal/markdown` | FR-009 |
| Two processes mutate one draft: second waits for the lock, both batches land, versions strictly increase; timeout refusal names the holder | `internal/draft` | FR-038 |
| Plugin files parse as the host's plugin format; SKILL.md contains the workflow and the prohibitions | `plugin` test in `internal/cli` or a script | US8 |
| No test imports a PTY library or reaches a non-loopback address | `go vet` custom check or a grep in `mise run check` | SC-007 |

Performance (SC-004): a benchmark in `internal/diff` parses a 500-file fixture and locates a hunk; the TUI open path is measured in `internal/tui` with a synthetic 500-file draft and asserted under 100 ms on the dev box.

## Manual walk (only on request, against a named pull request)

```sh
loupe --help
loupe capture https://github.com/<owner>/<repo>/pull/<n> --json
cat > /tmp/findings.json <<'EOF'
[{"title":"Example","body":"Evidence and correction.","location":{"path":"<path in diff>","line":<line>},"label":"suggestion"}]
EOF
loupe add --from /tmp/findings.json --json
loupe summary --body "One suggestion." --expect-findings 1 --json
loupe review            # accept the finding, quit
loupe publish --action comment --inline all   # inspect, press v, then y only if the owner said to publish
```

Expected: capture prints the refs and the two `git update-ref -d` commands; `add` returns `f-001`; `review` shows the hunk; `publish` shows the rendered review and the JSON envelope, and after `y` prints the review URL. Running `publish` again prints the same URL and sends nothing. Do not press `y` unless the owner asked for a live publish on that pull request.
