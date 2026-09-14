# Research: loupe v1

**Feature**: [spec.md](spec.md) | **Date**: 2026-09-13

These decisions were made with the product owner before this repository existed and are inputs to `/speckit-plan`, not open questions. Each entry records the decision, the rationale and the alternatives considered. A plan that departs from one MUST say why in Complexity Tracking.

## Language and TUI stack

**Decision**: Go 1.25+ (raised from 1.23 on 2026-09-13; see plan.md Complexity Tracking), single module `github.com/eriksaulnier/loupe`, one static binary. Terminal UI with `charmbracelet/bubbletea`, `charmbracelet/bubbles` and `charmbracelet/lipgloss`.

**Rationale**: One static binary satisfies SC-005 with no runtime on the agent's machine. Bubble Tea is the most complete terminal UI ecosystem available and comes with a test harness (`teatest`) that drives models with injected key messages, which principle VII requires. Startup time supports SC-004.

**Alternatives considered**: TypeScript with Ink (React model, slower startup, needs Node everywhere). TypeScript with a hand-rolled ANSI layer (the previous version did this and spent roughly 1,900 lines on it without a diff viewer).

## Runtime dependencies

Each dependency and its one-line reason, per constitution VI. Nothing else at runtime.

| Module | Reason |
| :--- | :--- |
| `spf13/cobra` | Subcommand help quality is the agent contract (FR-033); hand-rolled `flag` subcommands produce worse `--help`. |
| `charmbracelet/bubbletea`, `bubbles`, `lipgloss` | Chosen TUI stack, see above. |
| `charmbracelet/glamour` | Renders finding bodies as Markdown in the detail view. |
| `cli/go-gh/v2` | Reuses `gh`'s own token and host config and gives a typed REST client, so auth stays `gh`'s problem and no subprocess output is parsed. |
| `bluekeyes/go-gitdiff` | Unified diff parsing with the edge cases (no newline at end of file, renames, binary, mode changes) that a hand parser gets wrong. |
| `golang.org/x/term` | TTY detection and raw-mode probe for the `tty` refusal and plain-mode fallback. |

Git operations shell out to `git`; there is no Go Git library.

## GitHub access

**Decision**: `go-gh` REST client for pull request lookup, head recheck, review creation and review listing. Requires an existing `gh auth login` for github.com.

**Rationale**: Same auth as the user's `gh`, typed responses, testable by pointing the client at an `httptest` server through the `GH_HOST`/config mechanism go-gh honors.

**Alternatives considered**: Shelling out to `gh api` (parsing JSON from a subprocess, harder to fake). A personal access token flow (duplicates what `gh` already does).

## Run identity and storage

**Decision**: Root directory `$LOUPE_HOME`, else `$XDG_DATA_HOME/loupe`, else `~/.local/share/loupe`. One directory per run at `runs/<owner>/<repo>/<number>/<round>/`. A run reference is `owner/repo#123` (newest round) or `owner/repo#123@2`. Every command takes `--run <ref>` or the `LOUPE_RUN` environment variable; with neither, the current branch's pull request is resolved through the GitHub API and its newest round is used.

Files per run:

| File | Contents |
| :--- | :--- |
| `target.json` | owner, repo, number, url, author, viewer, baseSha, headSha, round, previousRound (optional), capturedAt, clonePath, baseRef, headRef, mergeBaseSha, diffSha256 |
| `pr.diff` | exact `git diff base...head` output (head against the merge base) |
| `draft.json` | the draft, see below |
| `.lock` | flock target for every mutation and for publish |
| `attempt.json` | in-flight or unknown publication with the embedded envelope and what was confirmed |
| `receipt.json` | review id, url, action, the envelope that was sent |

State is derived, never stored: captured if `target.json` exists, ready if readiness holds, published if `receipt.json` exists. There is no status file, no log file and no refusal log; refusals return to the caller.

`previousRound` is lineage only. `show --previous` walks rounds downward from `round - 1` and returns the findings of the first round with a `receipt.json`, skipping unpublished rounds; it refuses with `not-found` when none was published (clarified 2026-09-13).

**Rationale**: Pull-request-keyed directories make rounds natural (FR-005) and make `loupe review` with no argument possible (FR-037). Derived state removes a class of stale-record bugs.

**Alternatives considered**: Random run ids with an index file (the previous version; required copying ids between sessions). Per-repository `.loupe/` directories (pollutes clones and breaks when the same PR is reviewed from two clones).

## Capture mechanics

**Decision**: Resolve the pull request through the API, refuse with `same-head` when the newest existing round is unpublished and already at the API's head sha (a published round at the same head gets a new round; clarified 2026-09-13), verify the clone's `origin` matches the pull request's repository, accepting a match on either the configured URL or its `insteadOf` expansion because `insteadOf` is the user's own config and not attacker input (ruled 2026-09-13), then run one fetch from the `origin` remote by name, never from a URL built from the pull request, of `refs/pull/N/head` and the base sha into `refs/loupe/<owner>/<repo>/<number>/<round>/{base,head}` with hooks, auto maintenance, tag following, submodule recursion, pruning and `FETCH_HEAD` writing disabled on the command line. Verify both refs resolve to the API's shas. Record `git merge-base base head` and produce the diff with `git diff -U3 --no-ext-diff --no-color base...head` in the clone, with `diff.interHunkContext=0` and `GIT_DIFF_OPTS` removed so the user's diff config cannot change it, and store it with its SHA-256 (three-dot ruled 2026-09-13: GitHub diffs the head against the merge base, so once the base branch moves a two-dot diff carries lines GitHub will not accept for comments). Print the two `git update-ref -d` commands that remove the refs; loupe never removes them.

**Rationale**: Objects stay reachable for `git show <headSha>:<path>` during review and for later rounds, without a worktree (constitution IV). Disabling submodule recursion and hooks keeps attacker-influenced input (the PR head) away from any program or credential the clone's config might name. Verifying the refs against the API's shas means the stored diff is the diff of exactly what GitHub shows.

**Alternatives considered**: A detached worktree per run (the previous version; slow, large, and a second place to leak changes). Fetching the diff from the API (`gh pr diff` output is not byte-stable and cannot be verified against shas).

## Diff model and location validation

**Decision**: Parse `pr.diff` once with go-gitdiff into files, hunks and lines with old and new line numbers. A location is valid when the path is a file in the diff, the line exists on the given side in some hunk, and a range's start and end fall in the same hunk. Refusals list the nearest valid lines on that side of that path, up to three below and three above. The same model serves the TUI: hunk-for-location and file diff with per-line finding markers.

**Rationale**: One parsed model for validation and display keeps them consistent (FR-003, FR-008, FR-022).

**Alternatives considered**: Validating against the API's comment-position rules (opaque, and would need a network call per finding).

## Draft model

**Decision**: `draft.json`:

```text
draft:    { schema: 1, version, summary, findings[], decisions{}, notes[], replies[] }
finding:  { id "f-001", rev, title, body, location?, general, label?, blocking, confidence?, severity?, suggestedFix?, by, included, createdAt, updatedAt, history[] }
location: { path, side "RIGHT"|"LEFT", line, startLine? }
history:  { at, by, changed: { field: previousValue } }
decision: { findingId, decision "accepted"|"excluded", findingRev, at }   keyed by findingId
note:     { id "n-001", findingId, body, at, status "open"|"resolved"|"dismissed", closedAt? }
reply:    { id "r-001", noteId, body, by, at }
```

- `rev` on a finding increments on any change to a publishable field (title, body, location, label, blocking, confidence, severity, suggestedFix) or to `included`. A decision is current iff `decision.findingRev == finding.rev`. A send-back deletes the finding's decision.
- Dispositions are derived: accepted (included, current accept), pending (included, no current accept), excluded (human excluded, current), withdrawn (not included, no current decision). Readiness: no pending finding and no open note.
- The publishable digest is SHA-256 over the summary and the publishable fields of the publishable set, sorted by id. The publishable set is every finding that is included and not human-excluded, that is, disposition accepted or pending. Accept decisions therefore do not change the digest, so `show` before review fingerprints what would be published; at publish, readiness makes the set identical to the accepted findings. It goes into the hidden marker and is the freshness check at publish (ruled 2026-09-13).
- Every mutation: take the flock, read, validate, apply, bump `version`, write to a temp file, rename. `--expect-version N` refuses inside the lock when stale. Batches are all-or-nothing.
- `by` is `agent` or `human`. Reporting commands default to `agent`; the TUI always writes `human`. `by` is an audit field only: a finding filed or edited with `--by human` is pending until accepted in the review interface like any other (clarified 2026-09-13).

**Rationale**: A per-finding revision counter gives the binding FR-018 asks for in one integer comparison. The previous version hashed finding content, target and diff digest together; the target is immutable per run, so only the finding's own changes matter.

**Alternatives considered**: Storing dispositions (drifts from the facts that define them). A separate feedback file (two files to lock and keep consistent).

## Markdown allowlist

**Decision**: A body or summary is accepted when: it is at most 64 KiB; every fenced code block opened with three or more backticks or tildes is closed by a matching fence; outside fences and code spans, the only raw HTML tags are `<details>`, `<details open>`, `<summary>`, `</summary>` and `</details>`, each alone on its line, with a summary directly after its details opener and details nesting balanced to at most 15 levels; no HTML comments anywhere outside fences. A refusal names the code (`limit`, `fence`, `html`, `depth`), the one-based line and the fix command.

**Rationale**: The renderer wraps each finding in a generated `<details>` block, so authored HTML that closes it early restructures the review. This check prevents that in a few dozen lines. The previous version parsed every field with two Markdown/HTML parsers and diffed rendered trees; the owner ruled that out.

**Alternatives considered**: goldmark-based validation (a parser dependency for a check that a line scanner covers). No validation (a stray `</details>` collapses the rest of the review).

## Command-line contract

**Decision**: See [contracts/cli.md](contracts/cli.md) for the full command, input and result contract. Summary: `capture`, `add`, `edit`, `summary`, `show`, `feedback`, `reply`, `list` are agent-facing; `review` and `publish` are human-only. `--json` prints exactly one result object; `--from <file>` or `--from -` takes JSON input; `--by`, `--expect-version` and `--run` are common.

## Terminal review interface

**Decision**: One Bubble Tea program with these views.

- List: header with pull request, round and readiness counts (accepted, pending, excluded, withdrawn, open notes); the summary above the rows, collapsible; one row per finding with disposition glyph, id, blocking marker, label, title and `path:line`. Keys: `j`/`k` and arrows move, `enter` opens, `p` publish when ready, `?` help, `q` quit. Quitting keeps every decision.
- Detail: top region with title, label/blocking/confidence chips, body rendered by glamour, suggested fix in a fence, notes and replies; bottom region with the hunk containing the anchor, anchored lines highlighted, a few lines of context, or "general finding" when unlocated. Keys: `a` accept (included only), `x` exclude, `s` send back (one-line note input), `u` restore an excluded finding to pending, `r`/`d` resolve or dismiss the open note, `f` file diff, `J`/`K` scroll a hunk taller than its region (hidden rows are counted above and below it), `n`/`N` next and previous finding, `esc` back. Every decision carries the displayed draft version; a stale write is refused and the finding redisplays with a notice.
- File diff: the whole file's unified diff, scrollable, gutter marker on every line with a finding, `]`/`[` jump between findings, `enter` opens the finding under the cursor, `esc` back. Added and removed coloring only; no syntax highlighting in v1.
- Publish from `p`: pick action (comment only on the viewer's own pull request), pick inline mode, then the confirmation view described under Publication. A refusal shows reason and fix and returns to the list.
- Plain mode: `--plain`, or automatic when `TERM=dumb`, stdin cannot enter raw mode, or the terminal is under 60 columns by 12 rows. One finding at a time on stdout with its hunk, single-letter answers, same decisions and semantics.
- No lock is held while waiting for input. All rendering goes through one width-aware layer. Non-UTF-8 locales fall back to ASCII glyphs.

**Rationale**: Decisions are made only where the whole finding and its code are visible (FR-022). The file diff answers "what else changed here" without building a diff browser first.

**Alternatives considered**: A split-pane diff browser with findings as gutter markers (about double the TUI work; a follow-up once v1 is in use). Hunk only, no file diff (leaves the human blind to context outside the hunk).

## Publication

**Decision**: The state machine, in order.

1. Take the lock (flock on `.lock`) for steps 1 and 2, then release it; no lock is held through the gates and the confirmation view, so agent commands do not time out while the human reads. If `receipt.json` exists, print the URL and exit 0.
2. If `attempt.json` exists, list the pull request's reviews and look for one by the saved viewer at the saved commit whose body contains the saved marker. Found: write `receipt.json` from the saved envelope, delete the attempt, print the URL. Not found: refuse with the pull request URL until `--retry-unknown` is given. The attempt survives refusals and declined confirmations.
3. Refuse without a TTY on stdin and stdout (and stderr under `--json`, where the view draws), before reading the draft or credentials. Refetch the pull request; refuse if the head moved (fix: `loupe capture <url>`). Refuse approve or request-changes when the viewer is the author. Refuse approve when any included finding is blocking (code `blocking`; fix: `--action comment` or `request-changes`, or exclude or unblock the finding in `loupe review`; clarified 2026-09-13). Refuse unless ready (fix: `loupe review`).
4. Build the envelope: target, viewer, action, commitId, draft version and digest, inline mode, a UUID publicationId, body and comments. The body follows `docs/comment-format.md`: verdict callout derived from the action, count chips, summary, sections Blocking, Issues, Suggestions, Questions, Other with a `<details>` block per finding, `---` dividers, the footer `` loupe · round N · reviewed `sha` ``, then the hidden `<!-- loupe digest=… publication=… -->` and `<!-- loupe-meta … -->` comments. Inline mode `none`, `blocking` (default) or `all` selects which located findings also become review comments; general findings are never inline.
5. Confirmation view: the body and each inline comment rendered as they will read, control and bidirectional characters escaped, collapsed sections shown open. `v` or Tab toggles the exact JSON envelope. Only `y` sends; any other key, Esc, Ctrl-C or end of input cancels. Plain mode prints both blocks and asks `y/N` on the line.
6. On `y`: recheck head and viewer, retake the lock, and refuse if `receipt.json` or `attempt.json` appeared while the lock was released (under `--retry-unknown`, if the attempt is no longer the same unknown attempt seen in step 2), so two publishers cannot both pass step 1, both confirm and both send (ruled 2026-09-13); re-read the draft, refuse if version, digest or readiness changed, write `attempt.json` with state `in-flight`, the envelope and the confirmed dispositions, then POST `/repos/{owner}/{repo}/pulls/{number}/reviews` with `commit_id`, `body`, `event` and `comments`. 2xx: write `receipt.json`, delete the attempt. 4xx: delete the attempt and report; a message mentioning a pending review gets the submit-or-discard fix line. Anything else: mark the attempt `unknown`, reconcile once as in step 2, otherwise keep it and refuse.
7. Hold the draft lock until the outcome is written. A failed publish never mutates the draft.

**Rationale**: The owner kept this state machine from the previous version because GitHub review creation has no idempotency key and the ambiguous-outcome case is real. Everything before the send is a refusal, not a repair (constitution VI).

**Alternatives considered**: Receipt-only idempotency with a plain "attempt exists, check the PR" refusal (rejected by the owner: loses reconciliation).

## Claude Code plugin

**Decision**: `plugin/` in this repository, installable from a marketplace entry, containing `.claude-plugin/plugin.json`, `skills/loupe/SKILL.md` and `commands/loupe.md`. The skill is the workflow: capture; when the run has a previous round, read `show --previous`; investigate with `git show <headSha>:<path>` and never check out; write findings to a file and run `add --from`; `summary --expect-findings N`; then answer `feedback` with `edit` and `reply`; finish by telling the user to run `loupe review`. It states that the agent MUST NOT run `review` or `publish`, allocate a pseudo-terminal, pipe confirmation, or write GitHub reviews by any other route. `/loupe <pr-url>` invokes the skill with the URL. No hooks in v1.

**Rationale**: Convenience for the primary host without host-specific code paths (constitution I).

**Alternatives considered**: An MCP server (a second surface to keep in sync; deferred). Hooks that block `loupe publish` in agent allowlists (a guard the tool cannot verify; deferred).

## Distribution

**Decision**: goreleaser builds for linux and darwin, amd64 and arm64, published to GitHub Releases; `go install` also works. A Homebrew tap is deferred, not rejected (ruled 2026-09-13). Windows is untested in v1.

**Alternatives considered**: `gh extension` as the only channel (prefixes every agent command with `gh`). Both from day one (deferred; the same binary renamed `gh-loupe` can be added later).

## Testing strategy

**Decision**:

- Unit: diff parsing and location validation on fixture diffs; draft store (version, rev, decisions, readiness, batch atomicity, lock contention with two processes); markdown allowlist; envelope composition against golden files that reproduce the accepted output in `docs/comment-format.md` for the same input.
- TUI: `teatest` drives the models with key messages and asserts on state and rendered output, including small-terminal fallback and stale-write refusal.
- Integration: a local bare Git remote plus an `httptest` fake GitHub that go-gh is pointed at; runs capture → add → summary → scripted review decisions → publish → receipt, plus head-moved refusal, 422 pending review, and 5xx → unknown → reconcile.
- Never a real PTY, a real reviewer or a real GitHub write (constitution VII).

## Deferred, not rejected

- Browser feedback surface with Plannotator-style commenting: same draft, same decisions, no publish endpoint. The draft model and the review domain MUST stay separable from the TUI so this can be added.
- MCP server mode wrapping the same commands.
- `gh extension` alias.
- Homebrew tap.
- Syntax highlighting in the file diff view.
- GitHub Enterprise hosts.
