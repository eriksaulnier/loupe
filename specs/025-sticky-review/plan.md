# Implementation Plan: Sticky review

**Branch**: `025-sticky-review` | **Date**: 2026-09-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/025-sticky-review/spec.md`

## Summary

`loupe publish --sticky` finds the viewer's (or, unattended, a bot's) newest sticky loupe review on the pull request. When there is none, it creates one, body only, `COMMENT`, with `sticky=1` in `loupe-meta`. When there is one, it reads the earlier rounds back out of that review's body and composes a body with the new round on top and each earlier round collapsed below. It then replaces the review's body with one `PUT`. `internal/render` owns both directions of the sticky body: writing it and reading it back. `internal/publish` owns finding the review, the drop-oldest fit, the recheck, the one write and reconciliation. `internal/github` gains `UpdateReview`, and `fakegh` serves it. A non-sticky review does not change by one byte.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: `attempt.json` and `receipt.json` keep schema 1. The envelope gains `editReviewId` (omitted when 0), and the receipt gains `edited` (omitted when false). A record written before this feature reads as a create, which is what it was.

**Testing**: Test-first. Render tests pin the sticky body and its read-back, with one new golden (`testdata/golden/sticky.md`). Publish tests drive `Run` against `fakegh` for the create, the edit, the recheck, the drop-oldest fit, the definite rejection and the lost response. CLI tests cover the flag refusals, the `edited` key and the help goldens (`go test ./internal/cli/ -update`, then the diff read). One integration test publishes three unattended sticky rounds from fresh data roots. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.1.0 (amended for this feature on 2026-09-24). `docs/comment-format.md` is a contract and is amended by this spec. `scripts/check-tests.sh` holds `CreateReview` to one reference in `internal/publish/publish.go`, and `UpdateReview` joins that rule.

**Scale/Scope**: One new client method and fake handler, two new render functions and one new input field, one new publish file (`sticky.go`), changes to `publish.go`, `envelope.go`, `reconcile.go`, `round.go` and `records.go`, a flag and a result key in `internal/cli/publish.go`, one line in each confirmation surface, three documents and one script rule.

## Research

- **Where the earlier rounds come from.** Decision: the edited review's body on GitHub. Rationale: an unattended pipeline starts from a fresh data root (`unattendedRound` counts GitHub reviews for the same reason), so local receipts cannot carry history there. Alternative rejected: rebuilding from local receipts, which works only attended and would give the two paths two sources of truth.
- **How a round boundary is found.** Decision: two hidden comment lines that only loupe writes, `<!-- loupe-earlier -->` before the `### Earlier rounds` heading and `<!-- loupe-round -->` before each collapsed round. The read-back sees only lines outside fences, through a new `markdown.StructuralLines` that shares `mapTagLines`' fence and HTML-block reading. Rationale: the allowlist refuses an HTML comment outside a fence in every authored field (body, impact, suggested fix, summary, message), so an authored line can never equal a delimiter where the reader looks. A finding that quotes the delimiters in a fence, which is likely when loupe reviews itself, is content. Alternative rejected: splitting on `### Earlier rounds` or on `---`, both of which the opening prose can contain at top level.
- **The body's tail is read by position.** Decision: a sticky body always ends `---`, blank, footer, blank, reconciliation marker, `loupe-meta`. The read-back checks that shape from the end and refuses `sticky` when it is not there. Rationale: the tail is entirely generated. Reading it by position needs no parser, and a hand-edited body that broke it is refused rather than guessed at.
- **What an earlier round holds.** Decision: its round part, a divider, its footer line and its reconciliation marker, inside `<details>` with `<summary>Round N · reviewed <code>SHA</code></summary>`. `N` is its `round=`, and `SHA` is the hex code span of its footer. Its `loupe-meta` is dropped. Rationale: the footer says which commit and who reviewed it. The kept reconciliation marker lets an attempt from that round still reconcile after a newer round edited over it (spec edge case). One `loupe-meta` per body keeps `strings.Contains(MetaPrefix)` round counting and every tool that reads the marker correct.
- **The dropped-rounds note needs no parsing.** Decision: when `K − 1` exceeds the number of collapsed rounds, `### Earlier rounds` opens with the line `The N oldest rounds were dropped to fit GitHub's length limit.` (`The oldest round was dropped …` for one). The read-back discards everything between the heading and the first `<!-- loupe-round -->`. Rationale: dropping always takes the oldest, so the count is all a reader needs, and `K` in the marker already carries it.
- **Drop-oldest fit lives in `Build`.** Decision: after composing, while the body is over 65,536 characters and collapsed rounds remain, `Build` drops the last (oldest) and composes again. With none left it refuses with the existing `limit` refusal. Rationale: FR-024. `Build` already owns the limit check, and the confirmation renders what `Build` returns, so the human sees what was dropped.
- **`sticky=K` is the last key.** Decision: ` sticky=K` follows `regraded=` only on a sticky body. Rationale: FR-012. New keys MAY be added, and a non-sticky body stays byte for byte as it is. A sibling feature adding keys at the same place will conflict textually, and the owner rebases, per the brief.
- **Finding the review.** Decision: `findSticky` in `internal/publish/sticky.go` keeps the reviews that are not `PENDING`, whose author matches the envelope's rule (`authorMatches`: the viewer attended, `[bot]` unattended) and whose `render.StickyRounds(body)` is above 0, and picks the highest id. `StickyRounds` reads the last structural `loupe-meta` line's `sticky=`. Rationale: FR-006. It reuses the author rule reconciliation already applies.
- **One list, two uses.** Decision: an unattended sticky round lists the reviews once and uses the list both to number the round and to find the review. `unattendedRound` takes the list rather than fetching it. Rationale: two lists could disagree, and the round number would then describe a different pull request state than the edit.
- **Unattended numbering counts `K`.** Decision: `unattendedRound` adds `StickyRounds(body)` for a sticky bot review and 1 for any other bot loupe review. Attended numbering is unchanged, since every sticky round writes its own receipt. Rationale: FR-013. `round=` keeps rising once per round on both paths.
- **The write.** Decision: `github.Client.UpdateReview(ctx, owner, repo, number, id, body) (Review, error)` sends `PUT repos/{owner}/{repo}/pulls/{number}/reviews/{id}` with `{"body": …}` and decodes the returned review. `send` calls exactly one of `CreateReview` and `UpdateReview`, chosen by `env.EditReviewID`. Its definite-rejection and unknown-outcome handling is shared. Rationale: FR-008 and FR-018. The one-write property stays visible in one function.
- **Reconciling an edit.** Decision: `Match` skips the `commit_id` check and requires `r.ID == env.EditReviewID` when the envelope edits. Author, `PENDING` and the whole-line marker rules are unchanged. Rationale: FR-016. An edit cannot change `commit_id`, and the review id is what the attempt knows.
- **The recheck.** Decision: after `y`, `recheckSticky` lists the reviews again. An edit needs its review present with a body equal to the one composed from, after CRLF normalization. A create needs `findSticky` still to find nothing. Any other state refuses `changed`. The recheck is attended only, because unattended has no gap between composing and sending. Rationale: FR-015 and Principle II. The human confirmed a body that includes the old one, so a changed old body means the approved body no longer describes what the edit would replace.
- **Flag rules sit in two places.** Decision: `internal/cli/publish.go` defaults `--action` to `comment` and `--inline` to `none` under `--sticky`, and refuses the other values with `usage` before resolving the run. `publish.Run` repeats the action and inline rule beside the send, as it does for `--unattended`. Rationale: FR-002 to FR-004. `--inline`'s default is `blocking` today, so the CLI checks whether the flag was set (`cmd.Flags().Changed`) before applying the sticky default.
- **The confirmation line.** Decision: `Preview.Edits` carries the edited review's URL. The full-screen confirmation shows `edits <url> in place` in its header, and the plain confirmation prints the same line before its prompt. Rationale: FR-014. The heading already names the other things a wrong key could change. `loupe review`'s built-in publish flow does not offer sticky mode, and that stays out of scope.
- **The `sticky` refusal code.** Decision: a new code `sticky`, message naming the review URL and what is missing, fix `loupe publish without --sticky to post a new review`. Rationale: FR-011. No existing code means "the review loupe would edit is not in a form loupe can read back".
- **Output.** Decision: `--json` gains `edited` (always present). The human line reads `edited` with the same glyph as `published` when the round edited. Rationale: FR-017.
- **`check-tests.sh`.** Decision: extend its `CreateReview` rule to `UpdateReview`: referenced only in `internal/publish/publish.go`, exactly once there. Rationale: FR-023. The script is what keeps a second write path from appearing unnoticed.
- **The review screenshot.** Decision: not rerun here. A non-sticky review does not change. The README's picture shows a non-sticky review, so a rerun is needed only if the owner wants a sticky one pictured. That goes on the pending list.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The contracts are `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md`, amended in place.

### GitHub facts and the probes for each (FR-022)

None of these can be confirmed offline. The owner ran the probes marked observed on 2026-09-24 and 2026-09-25 on `eriksaulnier/loupe-probe#1` with a user token, and `docs/github-facts.md` records the results. The rest stay assumed.

| Fact | Status | Probe |
| :--- | :--- | :--- |
| `PUT /repos/{o}/{r}/pulls/{n}/reviews/{id}` with `{"body"}` replaces a submitted `COMMENTED` review's body, keeps its state and `commit_id`, and returns the review | Observed | Post a `COMMENT` review with `gh api`, then `gh api -X PUT …/reviews/<id> -f body=edited`. Compare `state`, `commit_id` and `body` before and after |
| The same works with an installation token (`github-actions[bot]`) | Assumed | Run the probe from a workflow step with `GITHUB_TOKEN` and `pull-requests: write` |
| The edit is not refused after time passes | Observed, 15 days | Repeat the `PUT` on a review older than a day, and on one older than a week |
| Editing sends no notification | Assumed | Watch the pull request's author's notifications and email during the probe |
| Editing a review the caller did not author is refused, and with which status (403 or 404) | Assumed | `PUT` from a second account, or from `GITHUB_TOKEN` against a human's review |
| A body near 65,536 characters is accepted, and one character more is refused with 422 | Observed otherwise: the limit is 262,144 UTF-8 bytes, which 65,536 characters never exceed | `PUT` a 65,536-character body, then a 65,537-character body |
| `<details>` nested 17 deep renders | Observed | Publish a sticky round whose collapsed round holds a finding body with 15 nested disclosures, and open it |

## Constitution Check

- **I. A tool for agents.** PASS. `--sticky` is a flag of an existing command, reachable from `loupe publish --help` on both paths. No host-specific path.
- **II. Nothing posts unread under a human's name.** PASS under 2.1.0. The one request edits a review the viewer published, and the author rule restricts it to one. The preview is the whole replacement body. The recheck after `y` refuses if the body being replaced changed. The new round's findings are accepted and its prose is typed at the confirmation, as today. Earlier rounds' findings and prose were accepted and typed in their own rounds, and they are shown again in full. Unattended keeps App identity, `COMMENT` only and the unattended marking, and edits only a `[bot]` review. An installation token cannot read its own login, so a `[bot]` review by another App can be chosen. The edit request is then expected to be refused by GitHub, which ends as a definite rejection that changes nothing. That refusal is a listed probe, not an observation.
- **III. Local files, no service.** PASS. No new file kinds and no network access beyond the GitHub API.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. `--help` documents the flag, the refusals name their fix, and the `--json` result gains one always-present key. The new `sticky` code is added to the contract table.
- **VI. Simplicity over ceremony.** PASS. No new dependency. One new file in `internal/publish`. The read-back is a line scanner over the structure loupe itself writes, not a Markdown parser. There is a refusal in place of recovery for a body loupe cannot read.
- **VII. Verified means ran.** PASS. Every behavior is tested against `fakegh` and local repositories, the goldens are regenerated deliberately and read, and `mise run check` runs after the last edit. Each live API fact above is marked observed only where the owner's probe ran, and the rest stay assumed.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/025-sticky-review/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/markdown/allowlist.go        # StructuralLines, sharing mapTagLines' reading of fences and HTML blocks
internal/render/body.go               # Input.Sticky, the Earlier rounds section, sticky=K
internal/render/sticky.go             # ReadSticky, StickyRounds, the delimiters
internal/render/*_test.go             # read-back, fenced delimiters, CRLF, bad tails, golden
testdata/golden/sticky.md             # a two-round sticky body
internal/github/client.go             # UpdateReview
internal/testutil/fakegh/fakegh.go    # PUT handler, QueueUpdate, UpdateCount, author check
internal/publish/sticky.go            # findSticky, recheckSticky
internal/publish/publish.go           # Options.Sticky, find/compose/recheck, one of two writes
internal/publish/envelope.go          # BuildInput.Sticky, drop-oldest fit
internal/publish/records.go           # Envelope.EditReviewID, Receipt.Edited
internal/publish/reconcile.go         # Match on review id for an edit
internal/publish/round.go             # unattendedRound over a given list, counting K
internal/cli/publish.go               # --sticky, defaults, usage refusals, edited, help
internal/refusal/                     # the sticky code
internal/tui/                         # the edits line in both confirmations
internal/integration/                 # three unattended sticky rounds from fresh roots
testdata/golden/cli/publish-help.*    # regenerated with -update
scripts/check-tests.sh                # UpdateReview under the CreateReview rule
docs/comment-format.md                # Sticky reviews section, sticky= key
docs/github-facts.md                  # the endpoint: observed facts, and the ones still assumed
specs/001-loupe-v1/contracts/cli.md   # --sticky, sticky code, widened changed, edited
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| Constitution amended to 2.1.0 | 2.0.2's "posts exactly one GitHub review per publication" | Owner, 2026-09-24, in clarify: amend rather than read the preamble loosely. Committed as its own change before planning |
| A collapsed round nests its findings one `<details>` level deeper than the allowlist's bound assumes | `docs/comment-format.md` depth rule (15 in a body, 16 in a summary) | Owner's draft asks for one `<details>` per round. The allowlist still checks authored text at its own bounds. GitHub was observed to render 17 levels |
| `changed` also covers the edited review's body changing during confirmation | `contracts/cli.md` `changed`: the draft changed | Same meaning to the human (what you confirmed is no longer what would be sent), same fix. A new code would add a second name for one situation |
