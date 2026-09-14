# Implementation Plan: loupe v1

**Branch**: `001-loupe-v1` | **Date**: 2026-09-13 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-loupe-v1/spec.md`, decisions from [research.md](research.md), command contract from [contracts/cli.md](contracts/cli.md), published format from `docs/comment-format.md`.

## Summary

A single static Go binary, `loupe`, that captures a pull request into a per-run directory of plain files, lets any shell-capable agent file review findings against the captured diff through a JSON-in, JSON-out command line, gives the human a Bubble Tea terminal interface to accept, exclude or send back each finding beside its diff hunk, and posts exactly one human-confirmed GitHub review through a refusal-first publication state machine with reconciliation for ambiguous outcomes. Git is a subprocess, GitHub is `go-gh`, run state is files under a user data directory, and nothing posts without an interactive `y`.

## Technical Context

**Language/Version**: Go 1.25+ (module `github.com/eriksaulnier/loupe`; the dev box has go1.25.8). `go.mod` declares `go 1.25.0`; see Complexity Tracking.

**Primary Dependencies**: `spf13/cobra` (command help is the agent contract), `charmbracelet/bubbletea` + `bubbles` + `lipgloss` (TUI), `charmbracelet/glamour` (Markdown body rendering in the detail view), `cli/go-gh/v2` (GitHub REST with `gh`'s auth), `bluekeyes/go-gitdiff` (unified diff parsing), `golang.org/x/term` (TTY detection and raw-mode probe). Test-only: `charmbracelet/x/exp/teatest` (drives Bubble Tea models with injected keys). Each runtime dependency carries its reason in research.md. Git is shelled out to; there is no Go Git library.

**Storage**: Plain files, one directory per run at `$LOUPE_HOME|$XDG_DATA_HOME/loupe|~/.local/share/loupe/runs/<owner>/<repo>/<number>/<round>/`: `target.json`, `pr.diff`, `draft.json`, `.lock`, `attempt.json`, `receipt.json`. Writes are temp-file-plus-rename under an exclusive `flock`. See [data-model.md](data-model.md).

**Testing**: `go test ./...` with `go vet ./...` and `golangci-lint run` as the repository check. Unit tests colocated per package; `teatest` for the TUI; an integration package with a local bare Git remote and an `httptest` fake GitHub. No real PTY, reviewer or GitHub write (constitution VII).

**Target Platform**: Linux and macOS terminals, amd64 and arm64. Windows untested. Distribution by goreleaser to GitHub Releases; `go install` also works. A Homebrew tap is deferred.

**Project Type**: Single CLI binary with an embedded TUI, plus a `plugin/` directory for the Claude Code host.

**Performance Goals**: SC-004: the review interface opens and shows the first finding's hunk in under 100 ms on a 500-file diff. Parse `pr.diff` once per process, lazily build per-file line indexes, never re-read the diff per finding.

**Constraints**: No daemon, no database, no network beyond `git` and the GitHub API (constitution III). Never touch the working tree, index, branch or non-loupe refs (constitution IV). `review` and `publish` refuse without a TTY on stdin and stdout. Every mutation is all-or-nothing under one lock. Body and summary limit 64 KiB; composed review body and each inline comment body limit 65,536 characters.

**Scale/Scope**: Ten commands, one TUI program with four views plus a plain mode, one publication state machine, one renderer for `docs/comment-format.md`, one plugin. Tens of runs per user, hundreds of findings per run at most, diffs up to a few thousand hunks.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Status |
| :--- | :--- | :--- |
| I. Tool for agents, not a tool that uses agents | No code path invokes, prompts or authenticates a reviewer; every workflow reachable from `loupe --help`; `plugin/` contains only instructions and a command, no code | PASS. The plugin is Markdown and JSON only. |
| II. Nothing posts on its own, nothing posts unread | Only `publish` calls the review-creation endpoint; it refuses without a TTY; readiness requires a per-finding accept made in a view that shows the whole finding; one confirmation sends one request | PASS. `github.Client.CreateReview` is called from exactly one function in `internal/publish`, reachable only from the `publish` command and the TUI's `p` flow, both TTY-gated. |
| III. Local files, no service | Files per run, `flock` for exclusion, no background process, no index database | PASS. `list` walks the `runs/` tree; state is derived from which files exist. |
| IV. Never touch the user's checkout | Capture writes only `refs/loupe/...` and objects; `git fetch` runs with hooks, maintenance, tag following, submodule recursion, pruning and `FETCH_HEAD` disabled; no worktree | PASS. Verified by an integration test that snapshots `git status --porcelain`, `HEAD`, the index checksum and `git for-each-ref` before and after capture. |
| V. Machine contract first | `--help` without run state; `--from <file>|-`; `--json` prints exactly one versioned object; refusals carry `code`, `message`, `fix`; stdout machine, stderr diagnostics | PASS. One envelope type and one error type in `internal/cli`; cobra's usage output routed to stderr under `--json`. |
| VI. Simplicity over ceremony | Eight runtime modules, each justified in research.md; version numbers over hashes (`schema: 1`, `loupe: 1`, draft `version`, finding `rev`); refusals over recovery; stdlib for locking, JSON, subprocesses, SHA-256, UUID | PASS. The only hash is the publishable digest, which the reconciliation marker requires. |
| VII. Verified means ran | Test plan uses a fake GitHub, local Git repositories and injected terminal input; the full capture-to-receipt flow runs in `go test` | PASS. See Testing in research.md and [quickstart.md](quickstart.md). |

**Development workflow gates**: Conventional Commits, `go vet && golangci-lint run && go test` before every commit, no push or live run without an explicit request naming the pull request. No violations to justify; Complexity Tracking is empty.

**Post-design re-check (after Phase 1)**: PASS. The data model adds no stored derived state, the contract is adopted unchanged apart from the two clarification-driven error codes, and the package layout keeps the draft and review domain independent of the TUI so the deferred browser surface stays possible without a rewrite.

## Project Structure

### Documentation (this feature)

```text
specs/001-loupe-v1/
├── plan.md              # This file
├── research.md          # Phase 0 output (seeded; extended in this plan only for clarification follow-through)
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── cli.md           # Adopted as the CLI contract
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
go.mod, go.sum
mise.toml                          # go, golangci-lint, goreleaser; tasks: check, test, lint, build
.golangci.yml
.goreleaser.yaml
lefthook.yml                       # pre-commit: check; commit-msg: conventional commit format
cmd/loupe/main.go                  # wires internal/cli and exits with its code

internal/
├── refusal/                       # leaf package: Error{Code, Message, Fix, Details} and the code table from contracts/cli.md
├── cli/                           # cobra root and subcommands, result envelope, error codes, --json/--from/--run handling
│   ├── root.go                    # top-level help text (workflow, run refs, review/publish are human-only)
│   ├── envelope.go                # {"loupe":1,"ok":...} success and refusal shapes, exit codes
│   ├── errors.go                  # maps *refusal.Error to exit codes and the refusal envelope
│   ├── capture.go, add.go, edit.go, summary.go, show.go, feedback.go, reply.go, list.go, review.go, publish.go
│   └── input.go                   # --from file|stdin decoding with DisallowUnknownFields and forbidden-field refusal
├── run/                           # data root resolution, run refs, directory layout, target.json, list, branch-to-PR resolution
│   ├── root.go                    # LOUPE_HOME / XDG_DATA_HOME / ~/.local/share
│   ├── ref.go                     # owner/repo#123[@round] parse and format
│   ├── target.go                  # Target record, load/save, derived state
│   ├── resolve.go                 # --run, LOUPE_RUN, current-branch lookup; newest round; newest published earlier round
│   └── lock.go                    # flock with timeout, holder pid, stale-lock refusal text
├── draft/                         # the editable review and its rules
│   ├── model.go                   # Draft, Finding, Location, Decision, Note, Reply, History
│   ├── store.go                   # load, mutate-under-lock, expect-version, temp-file rename
│   ├── mutate.go                  # add batch, edit, withdraw/restore, summary-with-expected-count, decide, send back, resolve/dismiss, reply
│   ├── derive.go                  # dispositions, readiness, counts
│   └── digest.go                  # publishable digest (SHA-256 over summary and the publishable set's publishable fields, sorted by id)
├── diff/                          # parsed pr.diff and location validation
│   ├── parse.go                   # go-gitdiff wrapper into Files/Hunks/Lines with old and new numbers
│   ├── locate.go                  # validate Location; nearest valid lines (3 below, 3 above); hunk containing an anchor
│   └── view.go                    # file diff with per-line finding markers, for the TUI and plain mode
├── markdown/                      # allowlist: size, fences, details/summary, comments, nesting depth
├── gitenv/                        # leaf package: strips repository-selecting GIT_* variables before gitx and test repositories run git
├── gitx/                          # git subprocess: origin URL with insteadOf expansion, fetch into refs/loupe with safety flags, verify shas, diff, show, current branch
├── github/                        # go-gh wrapper behind an interface: PR lookup by URL and by branch, list reviews, create review, viewer login
├── render/                        # docs/comment-format.md: body composition, sections, chips, meta block, footer, markers, inline comments, escaping, code-span rules
├── publish/                       # state machine: receipt replay, attempt reconciliation, gates, envelope build, confirmation callback, send, outcome recording
└── tui/                           # Bubble Tea program and plain mode
    ├── app.go                     # model, view routing, key map, width-aware layout, glyph selection (UTF-8 or ASCII)
    ├── list.go, detail.go, filediff.go, confirm.go
    ├── plain.go                   # line mode: one finding at a time, single-letter answers, y/N confirmation
    └── term.go                    # TTY detection, size check, TERM=dumb, raw-mode probe

internal/testutil/
├── fakegh/                        # httptest GitHub: PR lookup, reviews list/create with scripted outcomes (2xx, 422 pending, 5xx after recording)
└── gitrepo/                       # builds a bare remote plus a clone with a base and a PR head, refs/pull/N/head

internal/integration/              # capture → add → summary → scripted decisions → publish → receipt; head-moved; 422; 5xx→unknown→reconcile; same-head; previous-round walk-back

testdata/
├── diffs/                         # fixture unified diffs (renames, binary, no-newline, mode changes, multi-hunk)
└── golden/                        # rendered review bodies and inline comments reproducing docs/comment-format.md examples

plugin/
├── .claude-plugin/plugin.json
├── skills/loupe/SKILL.md
└── commands/loupe.md
```

**Structure Decision**: One Go module, `internal/` packages split by responsibility so that `draft`, `diff`, `render` and `publish` have no import of `tui` or `cli`. That separation is what keeps the deferred browser feedback surface and MCP mode possible without restructuring (research.md, "Deferred, not rejected"). `cli` depends on everything; `tui` depends on `draft`, `diff`, `render`, `publish`; `publish` depends on `draft`, `render`, `github`, `run`. Refusals are `*refusal.Error` values from the leaf `internal/refusal` package, because `cli` imports every domain package and a refusal type in `cli` would be an import cycle. Tests live beside their packages; the integration package is the only one that wires the full stack.

## Design Notes

These are the decisions the plan makes on top of research.md and contracts/cli.md. Each is small enough to live here rather than in research.md.

- **Newest-round and same-head lookup**: `run.Resolve` lists round directories under the pull request's directory, sorts numerically, and reads the newest `target.json`. Capture compares the API's `head.sha` with the newest round's `headSha`; equal and no `receipt.json` refuses with `same-head` naming that run's reference; equal with a receipt proceeds to a new round. `show --previous` walks rounds downward from `round-1` and returns the first with a `receipt.json`.
- **Approve-with-blocking gate**: `publish` computes the included findings and refuses `--action approve` with code `blocking` when any is blocking, after the `own-pr` gate and before the readiness gate. The TUI's action picker greys out approve with the same reason.
- **GitHub client injection**: `github.Client` is an interface with `PullRequest(owner, repo, number)`, `PullRequestsForBranch(owner, repo, headOwner, branch)`, `Viewer()`, `ListReviews(...)`, `CreateReview(...)`. The production implementation builds a go-gh REST client with `api.ClientOptions{Host: "github.com", AuthToken from gh}`. The fake in `testutil/fakegh` is an `httptest.Server`, and tests build the real go-gh client against it with a `Transport` that rewrites the scheme and host, so request encoding is exercised end to end. `PullRequestsForBranch(owner, repo, headOwner, branch)` uses `GET /repos/{owner}/{repo}/pulls?state=open&head={headOwner}:{branch}`, where `owner/repo` is the base repository asked and `headOwner` owns the head branch, which differs for a fork. Branch resolution reads `branch.<b>.remote` and `branch.<b>.merge` and goes in this order. (1) `merge` is `refs/pull/<N>/head`, as `gh pr checkout` writes for a fork's pull request: look up pull request N on the repository parsed from `remote`, which is a remote name whose raw or `insteadOf`-expanded URL is parsed, or a URL given directly; a `remote` that does not parse to github.com refuses with `no-run`. (2) `merge` is `refs/heads/<name>` and `remote` parses to github.com: `headOwner` is that remote's owner and the branch is `<name>`, asked of origin and of a remote named `upstream` when one exists and parses to github.com, since in a fork clone origin is the fork; the results are unioned. (3) Otherwise `headOwner` is origin's owner and the branch is the local branch name, asked of origin. Zero results refuses with `no-run`. More than one result is possible, since one head branch can have an open pull request against each of several bases; that also refuses with `no-run`, listing each pull request's `repo`, `number` and `base` in `details` and naming `--run <ref>` as the fix.
- **Optional-versus-null in `edit` input**: the edit payload decodes into a struct of `*json.RawMessage` fields so that an absent key, a `null` and a value are distinguishable. `null` is accepted only for `location`, `label`, `confidence`, `severity` and `suggestedFix`, per the contract.
- **Locking**: `syscall.Flock` on the run's `.lock`, non-blocking with retry until `LOUPE_LOCK_TIMEOUT_MS`. The lock file holds the holder's pid and command. The kernel releases a flock when the holder dies, so a timeout means a live holder, and removing the file would let two writers run at once; the refusal names the pid and command from the file and says to wait for that process or stop it. loupe never removes it.
- **Decisions and the lock in the TUI**: each decision opens the lock, re-reads the draft, checks the displayed version, applies, writes, and releases; no lock is held while waiting for keys. The TUI re-reads the draft after every refused write and on returning to the list.
- **Plain mode and TTY detection**: `term.IsTerminal` on both stdin and stdout for the `tty` refusal. Plain mode triggers on `--plain`, `TERM=dumb`, a failed raw-mode probe, or a size under 60 by 12. Plain mode reads answers line by line from stdin so it can be tested with an injected reader.
- **Digest and stale-send check**: the publishable digest covers the publishable set (included and not human-excluded: dispositions accepted or pending), so accept decisions do not change it; at publish, readiness makes the set identical to the accepted findings. It goes into the envelope and the hidden marker. `publish` holds the lock only for the receipt and attempt check, releases it through the gates and confirmation, and after `y` retakes it and refuses if `receipt.json` or `attempt.json` appeared meanwhile (under `--retry-unknown`, if the attempt is not the same unknown one) or if `version`, digest or readiness differ from what was displayed. Without the receipt and attempt recheck, two publishers could both pass the first check, both confirm and both send. Under `--retry-unknown`, once the retaken lock confirms the same unknown attempt, `publish` reconciles it again before overwriting it: a matching review writes the receipt from the saved envelope, deletes the attempt and reports a replay with nothing sent, and a failed listing refuses with `github` and keeps the attempt, since the review may have reached GitHub during confirmation.
- **Origin check**: capture accepts a match on the configured origin URL or its `insteadOf` expansion, since `insteadOf` is the user's own config. The fetch always names the `origin` remote and never a URL built from the pull request.
- **Escaping for display**: one function in `render` replaces C0/C1 controls and Unicode bidi overrides (U+202A to U+202E, U+2066 to U+2069) with visible escapes for the confirmation view and the TUI; the payload itself is sent verbatim, since GitHub rendering is the reader's concern and the human saw the escaped form.
- **Toolchain**: `mise.toml` pins go, golangci-lint and goreleaser and defines `check` (vet, lint, test), `test`, `lint`, `build`. lefthook runs `check` pre-commit and a Conventional Commit subject check on commit-msg. `golangci-lint` is not installed on the dev box today; `mise install` provides it.

## Complexity Tracking

No constitution violations.

| Departure | From | Why |
| :--- | :--- | :--- |
| Review and inline comment body limit is 65,536 characters, not 256 KiB | data-model.md "Envelope", docs/comment-format.md "Refusals" | Lowered on 2026-09-13 because GitHub is believed to reject longer review and comment bodies with 422. The owner will confirm this observation on the live walk. |
| `go.mod` declares `go 1.25.0`, not `go 1.23` | research.md "Language and TUI stack", this plan's Technical Context | Current releases of the chosen dependencies require newer toolchains (go-gh v2.16 needs 1.25; bubbletea, bubbles, glamour and teatest need 1.24). Pinning every dependency to its last 1.23 release was rejected by the owner on 2026-09-13. `golang.org/x/term` is held at v0.45.0 because v0.46.0 requires go 1.26. Go 1.21+ fetches a newer toolchain on `go install`, so older installed toolchains still work. |
