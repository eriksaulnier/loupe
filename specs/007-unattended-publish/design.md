# Unattended publish for CI reviewers: design

This is the design agreed before running spec-kit. `spec.md`, `plan.md` and `tasks.md` in this directory come from it. Line references are against `main` at `b961939`.

## Context

Automated reviewers running in CI (GitHub Actions) should post their findings in loupe's published format. Today `loupe publish` refuses without a terminal and publishes only findings a human has accepted. Constitution Principle II forbids exactly this, so the change starts with a MAJOR amendment. The goal is to narrow II, not remove it: nothing posts under a human's name unread, and an unread review always posts as a bot and says so.

## Decisions

- **Gate on the token, not the environment.** `--unattended` refuses unless the resolved token is a GitHub App installation token (`ghs_` prefix). Actions' `GITHUB_TOKEN` qualifies. A local agent holding a human's `gh auth` token does not. PATs for bot user accounts are refused.
- **No `GITHUB_ACTIONS` check.** It adds no security, since any process can set it. An Actions-only path would also break Principle I ("MUST NOT add code paths that only one host can reach"). Any CI system holding an App token works.
- **Entry point:** `loupe publish --unattended`. It reuses publish's attempt, receipt and reconcile machinery, plus `--inline`, `--retry-unknown` and `--json`.
- **Event:** COMMENT only. Findings no one has read never block or approve a merge. `--unattended` with an `--action` other than `comment` is a usage error (exit 2).
- **Marker:** the footer gets ` · unattended` after the round, and `loupe-meta` gets `unattended=1`. No opening line; the bot author covers the rest.

```text
loupe · round 2 · unattended · reviewed `a1b2c3d` · via `claude-ci`
<!-- loupe-meta v=1 round=2 unattended=1 ... -->
```

## Design

**Constitution 2.0.0.** Rewrite II so that every finding reaching GitHub *under a human's identity* MUST be individually accepted and confirmed in one interactive command. An unattended publication MAY skip the terminal and per-finding decisions only with an App installation token. It posts as that App, sends COMMENT, and is marked unattended. Agents in a human's session remain forbidden from publishing. The plugin skill's ban (`plugin/skills/human-review/SKILL.md:16`) stays as written. In CI, publishing is a workflow step a human authored, not something the agent does.

**Token kind.** The resolved token never leaves `go-gh` today (`internal/github/client.go:127`, `newREST`). Add a seam so `internal/github` reports whether the client holds an installation token, using the same `auth.TokenForHost` call. It has two consumers, capture and publish, which satisfies VI's "second concrete use".

**Identity without `GET /user`.** Installation tokens get 403 on `GET /user`. `Viewer()` is load-bearing in several places:

- Capture (`internal/cli/capture.go:111`): with an installation token, skip the call and record `viewer: ""` in `target.json`.
- Gates (`internal/publish/gates.go:39`, the own-PR check): moot under COMMENT-only, so unattended skips it.
- The account-change recheck after confirmation (`internal/publish/publish.go:193`): unattended has no confirmation window, so it skips the recheck.
- Reconcile (`internal/publish/reconcile.go:19`): when the envelope viewer is empty, match on the digest and publication marker, and require an author login ending in `[bot]`. Record the actual login from the CreateReview response in the receipt.
- The token kind at publish MUST match the kind at capture, or publish refuses with the existing `viewer` code. That stops a human publishing a CI-captured run, and CI publishing a human-captured one.

**Finding selection.** Unattended publishes `draft.PublishableSet`, which is accepted plus pending findings, and skips `ReadinessRefusal` for pending ones. Excluded and withdrawn findings still stay out, and `empty` still refuses.

**Round N.** `publishedRound` (`internal/publish/round.go:16`) counts local run state. A CI job starts with a fresh `LOUPE_HOME`, so every CI review would say round 1. For unattended, derive N from `ListReviews`: 1 plus the number of the PR's reviews authored by a bot whose body carries a `loupe-meta` marker. Publish already calls `ListReviews`.

**Gates for `--unattended`.** Keep head-moved, empty and attempt/replay. Drop TTY, confirmation, readiness, own-PR and the viewer recheck. Add one refusal code for a non-installation token, with a fix line naming `GITHUB_TOKEN` and `permissions: pull-requests: write`.

**Contracts touched:**

- `specs/001-loupe-v1/contracts/cli.md`: the error codes table at :35 and the publish section at :146.
- `docs/comment-format.md`: the Footer and Markers sections.
- publish `--help` text and its goldens.
- README: a minimal Actions example.

**Record in `docs/github-facts.md` once observed:**

- the `GITHUB_TOKEN` prefix
- the 403 on `GET /user`
- the author login `github-actions[bot]`
- fork PRs under `pull_request` get a read-only token, so CreateReview fails with 403. This goes in the spec as an edge case with no workaround; `pull_request_target` is out of scope.

## Dog-food: automatic Claude reviews on this repo

A new `.github/workflows/review.yml` is the last task group in this spec, landing after `--unattended`. It is the live check for the Unverified items below.

- **Trigger:** `pull_request` types `opened` and `ready_for_review`, plus `workflow_dispatch` with a PR number for re-runs after big pushes.
- **Model access:** through OpenRouter. The step sets `ANTHROPIC_BASE_URL=https://openrouter.ai/api` and passes the `OPENROUTER_API_KEY` secret as both `anthropic_api_key` and `ANTHROPIC_AUTH_TOKEN` ([OpenRouter's guide](https://openrouter.ai/docs/cookbook/coding-agents/claude-code-integration)). The model comes from the repo variable `LOUPE_REVIEW_MODEL`. The first run uses `anthropic/claude-opus-5`, so the only new part is the endpoint.
- **Rollout:** the first commit ships `workflow_dispatch` only, so the first live run is on a pull request the maintainer names, per AGENTS.md. A follow-up enables the `pull_request` trigger once that run looks right.
- **Kill switch:** the job's `if:` requires the repo variable `LOUPE_AUTO_REVIEW == 'true'`, so it turns off in settings without a commit. The same `if:` skips drafts, fork PRs (read-only token, no secrets), release-please PRs and Dependabot.
- **Build from `main`, never the PR head.** A PR MUST NOT be able to swap the binary that holds the write token.
- **Permissions:** `contents: read`, `pull-requests: write`.
- **Steps:**
  1. Build `dist/loupe` from `main`.
  2. `anthropics/claude-code-action` runs an inline prompt: "run `loupe --help`, capture PR N with `--source claude-ci`, review, file findings with `loupe add`, do not publish". Its allowed tools cover `loupe` subcommands except `publish`, plus Read/Grep/Glob. The agent has only `--help` to go on, so this also tests Principle I. Confirm the exact action inputs against its docs at build time.
  3. A separate step runs `loupe publish --unattended --json`.
  4. Upload the run directory as an artifact whether the job passes or fails. The runner is torn down, and that directory is the publication evidence.
- **Spec edge case:** an attempt left unknown in CI fails the job. A re-run captures fresh with a new publication id, so reconcile cannot match the earlier attempt and the review may post twice. The spec states this rather than recovering state across jobs.
- **Later, not in this spec:** try non-Claude models by changing `LOUPE_REVIEW_MODEL`; nobody has checked how well they handle Claude Code's tool loop. Try a non-Claude harness too (e.g. Codex or opencode), to prove loupe isn't Claude-specific.

## Next steps

Spec-first, per the repo's spec-kit flow. No code until the spec is reviewed.

1. Amend `.specify/memory/constitution.md` from 1.0.1 to 2.0.0, stating what changed and why.
2. On this branch, run `speckit-specify` from this design to write `spec.md` here, mirroring the headings of `specs/005-edit-in-review/spec.md`. The branch and directory already exist, so pass `--allow-existing-branch --number 7 --short-name unattended-publish` to `create-new-feature.sh`. Without that flag the script moves the feature to 008.
3. Stop for review. `speckit-plan` and `speckit-tasks` follow only after that.

## Verification for the implementation

- `fakegh` gains an installation-token mode: `/user` returns 403 and reviews are authored by `github-actions[bot]`.
- Integration tests in `internal/integration` cover:
  - the unattended happy path
  - refusal with a user token
  - the capture/publish kind mismatch
  - an ambiguous send, reconciled via `--retry-unknown`
  - round N derived from existing bot reviews
- Goldens: publish help, and a body with ` · unattended`. Regenerate with `go test ./internal/cli/ -update` and read the diff.
- `mise run check` passes: vet, golangci-lint, tests and `scripts/check-tests.sh`.
- Unverified until the first `workflow_dispatch` run on a named PR:
  - the token prefix
  - the `/user` 403
  - the bot author
  - job cancellation during send, since publish holds signals while sending

  Add these to the validation Unverified list.
