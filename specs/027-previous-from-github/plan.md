# Implementation Plan: Previous round from GitHub

**Branch**: `027-previous-from-github` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/027-previous-from-github/spec.md`

## Summary

Every published body gains one hidden line, the findings record, written by `internal/render` next to the reconciliation marker: a version, a checksum, and the published findings as base64 of deflated JSON. At capture, when no earlier local round holds a receipt, `internal/publish` lists the pull request's reviews, picks the publisher's newest loupe review by sticky mode's author rule, and reads the record back. Capture stores the outcome in the new run as `previous.json`, findings or a reason, and reports it in a new `previous` result key. `show --previous` prefers a local receipt as today, then answers from `previous.json`, and refuses `not-found` with the stored reason when there is none. Terminal surfaces show the record as a one-line stand-in.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. `compress/flate`, `encoding/base64`, `crypto/sha256` and `encoding/json` are standard library.

**Storage**: A new optional run file, `previous.json`, schema 1, written by capture inside `run.CreateRun`'s temporary directory so it appears with the run or not at all. A run without it is a run captured before this feature or after a local receipt, and reads as today.

**Testing**: Test-first. Render tests pin the record's line, its checksum, its read-back and each damaged form. Publish tests cover the author rule and each degrade reason against in-memory reviews. CLI and integration tests drive capture and `show --previous` against `fakegh` from empty data roots, attended and unattended, sticky and plain. Body goldens and CLI goldens are regenerated with `-update` and every diff is read. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.0.0. `docs/comment-format.md` is a contract, amended here. `contracts/cli.md` is adopted, and gains keys only. A parallel PR changes the sticky layout (each collapsed round keeps its footer, and the current round's footer sits directly under it). The record is found by its marker, never by position, so it reads the same in both layouts. Only the demotion step in `ReadSticky` depends on the layout, and it is rebased onto that PR when it merges.

**Scale/Scope**: One new render file (`record.go`), one new publish file (`previous.go`), small changes to `render/body.go`, `render/sticky.go`, `publish/envelope.go`, `publish/sticky.go`, `run/target.go`, `cli/capture.go`, `cli/show.go`, two TUI display sites, the `human-review` skill, and three documents.

## Research

- **Record, not parsing.** Decision: a hidden record (owner-side clarification, 2026-09-25). Rationale: the body never shows finding ids, `<summary>` titles lose backslash escapes, a body holding `**Suggested fix:**` makes the fields of a disclosure ambiguous, and a positional parse would break on the parallel sticky layout change. Alternatives rejected: parsing the visible body, and a record without bodies.
- **The line.** Decision: `<!-- loupe-findings v=1 sha256=<hex> <data> -->`, where `<data>` is standard base64 of raw DEFLATE (`compress/flate`, best compression) of compact JSON: an array of `{id, title, body, location, label, blocking}`, the envelope's findings in their order. When left out for length the line is `<!-- loupe-findings v=1 omitted=length -->`. Rationale: base64's alphabet holds no `-` or `>`, so no finding can close the comment (FR-003). The fields are exactly `publish.EnvelopeFinding`, so both sources give `show --previous` the same shape. DEFLATE brings the record to roughly 0.4 to 0.5 of the findings' text, where plain base64 would be 1.33. Alternatives rejected: readable JSON with escaping (as large as the findings, and a `--` escape rule to maintain); gzip (a header and a CRC the checksum already covers).
- **The checksum.** Decision: `sha256=` is SHA-256 over the body with CRLF normalized to LF and the record line removed, then a newline, then `<data>`. Rationale: FR-002. It detects an edit to any visible byte and to the data, so a review changed on GitHub degrades rather than lists findings its body no longer shows. The whole body is covered rather than the current round because the body is written whole at every publish, and that keeps the check independent of the sticky layout. The cost is that a typo fix in an earlier collapsed round also degrades. That is accepted: a missed round is recoverable, a wrong list is not. CRLF normalization matches `recheckSticky`'s `lf`.
- **Placement.** Decision: the record sits between the reconciliation marker and `loupe-meta`. `ReadSticky` accepts it at that position and drops it from the demoted round (FR-004). The reader finds it as the last structural line (outside a fence) that starts with `<!-- loupe-findings `, and refuses a body with two. Rationale: the tail is generated, so the record stays with the other markers, and `IsLoupe`, `StickyRounds`, `MetaSource` and the reconciliation marker's whole-line match keep reading the same lines. The allowlist refuses an HTML comment outside a fence in every authored field, so an authored line can never be taken for the record.
- **Record from render's own input.** Decision: `render.Body` derives the record from `Input.Findings`, sorted by id, and a new `Input.OmitRecord` writes the omission line. Rationale: the record MUST hold only what the body shows (FR-001). Deriving it from the same slice makes that true by construction.
- **Length.** Decision: in `publish.Build`, after the existing drop-oldest loop, a body still over 65,536 characters is composed again with `OmitRecord`. A body still over is refused as today. Rationale: FR-006. Collapsed rounds give way first, as today, so a sticky review keeps its record longest. An omission is marked so a later capture can say why.
- **Terminal stand-in.** Decision: `render.RecordAsNote` replaces a record line outside a fence with `<!-- loupe-findings: a copy of the N findings above -->` (`1 finding`, `no findings`), or leaves an omission line as is. The full-screen confirmation, the plain confirmation, the plain payload preview and the full-screen body view call it next to `PillsAsWords`. The payload view that shows exact bytes does not. Rationale: FR-007 and the clarification. It is the pill precedent.
- **Reading back.** Decision: `render.ReadRecord(body) ([]RecordFinding, error)` returns a typed error for each reason: no record, omitted for length, more than one, version not read, checksum mismatch, data that does not decode, inflate or unmarshal, and a record over 16 MiB inflated. It validates that every finding has a non-empty id and title and that ids are unique. Rationale: FR-013 and User Story 4. A cap on inflation keeps a crafted review from exhausting memory. Every failure is a reason, never a partial list.
- **Finding the review.** Decision: `publish.newestOwn(reviews, viewer, source)` holds the part of `findSticky` that is not about stickiness: skip `PENDING`, apply `authorMatches`, require `IsLoupe`, and for an installation token require the same `src=` name. `findSticky` calls it and then checks `sticky=`. Rationale: FR-010. The rule already exists and is the one the brief names. Two uses justify the extraction (Principle VI).
- **The current round of a sticky review.** Decision: nothing extra. The record lives on the round shown on top, and demotion drops it. Rationale: FR-012 falls out of placement.
- **`round=`.** Decision: `render.MetaRound(body)` reads `round=` from the line `MetaSource` reads. Rationale: FR-021.
- **Capture's read.** Decision: `publish.ReadPrevious(ctx, client, owner, repo, number, viewer, source) Previous` lists the reviews once and returns either `{reviewId, reviewUrl, round, findings}` or `{reason}`. A list failure is a reason, not an error. Capture calls it after the clone checks pass and only when `run.PreviousPublished` finds no local receipt (FR-011). It runs under the capture lock, which already covers the rest of capture. Rationale: FR-013. The previous round is an aid, so a failed read degrades.
- **Storage.** Decision: `publish.Previous` is encoded by capture and handed to `run.CreateRun` as one more optional file, `previous.json`, written inside the temporary directory. `publish.LoadPrevious(dir)` reads it and returns found false when it is absent. Rationale: `internal/draft` imports `internal/run`, so `run` cannot hold a type with `draft.Location`. `publish` already owns `receipt.json` and `attempt.json`, the other publication records in a run.
- **Capture's result.** Decision: `previous` is always present: `{"from": "receipt", "round": R}` when a local receipt exists; `{"from": "github", "round": N, "reviewUrl": …, "findingCount": n}` after a read; `{"from": "none", "reason": …}` otherwise. The human output adds one line under the refs. Rationale: FR-014. `from` is the one key a skill needs to branch on.
- **`show --previous`.** Decision: local receipt first; then `previous.json` with findings; else `not-found`. The message is today's when nothing was stored, and `no earlier round of X can be read back: <reason>` when a reason was stored. The result adds `from`. The human view prints `round N, published` and the review URL as today. Rationale: FR-020 to FR-022.
- **The skill.** Decision: step 2 of `plugin/skills/human-review/SKILL.md` keys on capture's `previous.from` being `receipt` or `github`. Rationale: FR-023. `target.previousRound` stays local lineage, and in CI it is absent.
- **The workflow.** Decision: out of scope. The shared workflow's prompt is `eriksaulnier/loupe-workflows`; the final report names the follow-up.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`, as in specs 014 and later. The contracts are `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md`, amended in place.

### GitHub facts

| Fact | Status | Source |
| :--- | :--- | :--- |
| A review body reads back as sent, apart from line endings | Observed for user tokens | Sticky mode's recheck relies on it (`docs/github-facts.md`) |
| A single HTML comment line of tens of kilobytes is hidden in the rendered review and kept in the body | Assumed | A live round on this repository's own PR shows it; the final report lists it as unverified if no round runs |
| The CI capture token can list a pull request's reviews | Assumed | The review job already reads the pull request with it |

## Constitution Check

- **I. A tool for agents.** PASS. `capture` and `show --previous` stay reachable from `--help`, and the skill change is instructions only. No host-specific path.
- **II. Nothing posts unread under a human's name.** PASS. The record holds only findings the human accepted, copied from the same slice the visible body is rendered from, so it adds no words the human did not read. The confirmation shows a stand-in naming it and the payload view shows its exact bytes, as pills already work. An unattended review is unchanged in identity, event and marking. The draft's summary is never put in the record.
- **III. Local files, no service.** PASS. One new file in the run directory. The only network access is one GitHub API read at capture.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. `capture` and `show --previous` gain one key each, and `show --previous` keeps its `not-found` refusal with its fix.
- **VI. Simplicity over ceremony.** PASS. No dependency. `newestOwn` is extracted for its second use. A record in place of a parser, and a degrade in place of a fallback to older reviews.
- **VII. Verified means ran.** PASS. Every behavior is tested against `fakegh` and local repositories, goldens are regenerated deliberately and read, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

### Documentation (this feature)

```text
specs/027-previous-from-github/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md
```

### Source Code (repository root)

```text
internal/render/record.go        # new: write, read, stand-in, MetaRound
internal/render/body.go          # append the record line; Input.OmitRecord
internal/render/sticky.go        # ReadSticky accepts and drops the record
internal/publish/previous.go     # new: newestOwn, ReadPrevious, Previous, LoadPrevious
internal/publish/sticky.go       # findSticky calls newestOwn
internal/publish/envelope.go     # omit the record when the body is still too long
internal/run/target.go           # CreateRun writes previous.json when given
internal/cli/capture.go          # the read, the previous key, help
internal/cli/show.go             # the fallback, the from key, help
internal/tui/confirm.go, plain.go  # RecordAsNote beside PillsAsWords
plugin/skills/human-review/SKILL.md
docs/comment-format.md
specs/001-loupe-v1/contracts/cli.md
testdata/golden/**               # regenerated
```

**Structure Decision**: The existing single Go module. No new package.

## Complexity Tracking

None.
