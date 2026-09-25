---

description: "Task list for previous round from GitHub"
---

# Tasks: Previous round from GitHub

**Input**: Design documents from `specs/027-previous-from-github/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (3.0.0)

**Tests**: The published format and two agent-facing results change, so every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). Goldens are regenerated only after a hand-written test pins the change, and every regeneration is followed by reading the diff. A body golden MUST differ by the record line only (SC-005). New tests MUST pass `scripts/check-tests.sh`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the record in every body

- [X] T001 Add failing tests to `internal/render/record_test.go` for the record `Body` writes:
  - every body ends with the reconciliation marker, then one line `<!-- loupe-findings v=1 sha256=<64 hex> <base64> -->`, then `loupe-meta`, on the plain, unattended and sticky paths.
  - the base64 data inflates (raw DEFLATE) to compact JSON: an array of `{"id","title","body","location","label","blocking"}` in id order (`findingid.Compare`), `location` null for a general finding and `{"path","side","line"}` plus `"startLine"` only when set, the same encoding `publish.EnvelopeFinding` marshals to.
  - a body with no findings carries a record of `[]`.
  - `Input.OmitRecord` writes `<!-- loupe-findings v=1 omitted=length -->` in the same place.
  - a finding whose title and body hold `-->`, `--`, `<!--` and a newline cannot close or split the line: the record line is a single line matching `^<!-- loupe-findings v=1 sha256=[0-9a-f]{64} [A-Za-z0-9+/=]+ -->$`.
  - `sha256` equals SHA-256 over the body with CRLF as LF and the record line removed, then `\n`, then the data field.
- [X] T002 Add failing tests to `internal/render/record_test.go` for `ReadRecord(body string) ([]RecordFinding, error)`, a round trip over `Body` for each path in T001, and one error per reason, each distinguishable with `errors.Is` against exported sentinels: no record (`ErrNoRecord`), omitted for length (`ErrRecordOmitted`), two records outside fences, a version other than `v=1`, a checksum mismatch after one visible byte changes, a checksum mismatch after the data changes, data that is not base64, does not inflate, or is not the JSON array, a record that inflates past 16 MiB, and a finding with an empty id or title or a repeated id. A record line inside a fenced finding body is not read. A CRLF body reads as LF.
- [X] T003 Add failing tests to `internal/render/record_test.go` for `RecordAsNote(body string) string`: a record line outside a fence becomes `<!-- loupe-findings: a copy of the N findings above -->` (`the 1 finding above`, `no findings` for `[]`), an omission line and every other line are unchanged, and a line inside a fence is unchanged. Add a test for `MetaRound(body string) int`: the `round=` value on the line `MetaSource` reads, 0 when there is none.
- [X] T004 Implement T001 to T003 in a new `internal/render/record.go` and in `internal/render/body.go` (`Input.OmitRecord`; the record line between the reconciliation marker and `loupe-meta`, derived from `Input.Findings`). Locate lines only through `markdown.StructuralLines`. Use `compress/flate` at `flate.BestCompression`, `encoding/base64.StdEncoding` and a `json.Encoder` with `SetEscapeHTML(false)`. Cap inflation with an `io.LimitReader` at 16 MiB plus one byte.
- [X] T005 Add failing cases to `internal/render/sticky_test.go`: `ReadSticky` reads a body carrying a record, and the demoted round holds no record line; a round trip over three sticky rounds leaves exactly one record, on the current round, and `ReadRecord` returns the newest round's findings. Then make `ReadSticky` in `internal/render/sticky.go` accept an optional record line between the reconciliation marker and `loupe-meta`, and drop it. A body written before this feature still reads back.
- [X] T006 Add a failing test to `internal/publish/envelope_test.go`: a draft whose body fits without its record but not with it is composed with the omission line, and a sticky draft drops collapsed rounds before it drops the record. Then implement it in `internal/publish/envelope.go` after the drop-oldest loop.
- [X] T007 Regenerate the body goldens with `go test ./internal/render/ ./internal/publish/ -update` and read the diff. Every body golden MUST differ by the record line only. Fix any test that pins a body tail by position.
- [X] T008 [P] Call `render.RecordAsNote` beside every `render.PillsAsWords` call on a review body in `internal/tui/confirm.go` and `internal/tui/plain.go`, with a failing test first in each package's existing confirmation tests that the stand-in shows and the base64 does not. The exact-bytes payload view MUST NOT change.

**Checkpoint**: every review loupe publishes carries a record that reads back to its findings.

## Phase 2: User Story 1 — a CI reviewer sees the last round (P1)

**Goal**: capture from an empty data root stores the publisher's last round, and `show --previous` lists it.

**Independent Test**: publish unattended with `--source` from one data root against `fakegh`, capture at a new head from a second empty root with an installation token, run `show --previous --json`.

- [X] T009 [US1] Add failing tests to a new `internal/publish/previous_test.go` for `newestOwn(reviews, viewer, source)` and `ReadPrevious(ctx, client, owner, repo, number, viewer, source) Previous`: with an empty viewer, a `[bot]` review whose `src=` name matches, version ignored, is chosen over an older one; a newer `[bot]` review with another source name, a person's review, a `PENDING` review and a non-loupe review are skipped; a `DISMISSED` review is chosen; a sticky review yields its current round; the result carries the review id, URL, `MetaRound` and the findings. Refactor `findSticky` in `internal/publish/sticky.go` to call `newestOwn`, and keep every sticky test green.
- [X] T010 [US1] Implement `Previous` (`{schema, reviewId, reviewUrl, round, findings []EnvelopeFinding}` or `{schema, reason}`), `ReadPrevious`, and `LoadPrevious(dir) (Previous, bool, error)` in `internal/publish/previous.go`. `LoadPrevious` refuses a damaged file with the `record` refusal as `LoadReceipt` does.
- [X] T011 [US1] Add a failing test to `internal/run/target_test.go` that `CreateRun` writes `previous.json` inside the new run when given bytes and writes nothing when given nil, then add the parameter in `internal/run/target.go` and update its callers.
- [X] T012 [US1] Add failing tests to `internal/cli/capture_test.go`: a capture with no local receipt lists the reviews once, stores `previous.json`, and its `--json` result carries `previous` `{"from":"github","round":N,"reviewUrl":…,"findingCount":n}`; the human output adds one `previous` line. Then implement it in `internal/cli/capture.go` after the clone checks, using the viewer and source capture already holds, and document `previous` in `captureHelp`.
- [X] T013 [US1] Add failing tests to `internal/cli/show_test.go`: with no local receipt and a stored previous round, `show --previous --json` returns `{from: "github", round, reviewUrl, findings}` with the stored findings, and makes no request to `fakegh`; the human view prints the round, `published` and the review URL as for a receipt. Then implement the fallback in `runShowPrevious` in `internal/cli/show.go`, and document `from` in `showHelp`.
- [X] T014 [US1] Add an integration test to `internal/integration/rounds_test.go`: round 1 publishes unattended with `--source ci-review@1.0.0` from one data root; round 2 is captured at a new head with `--source ci-review@2.0.0` from a new empty data root; `show --previous --json` lists round 1's findings with every field equal to round 1's receipt envelope. Repeat with `--sticky` over three rounds, and assert round 3 sees round 2's findings.

## Phase 3: User Story 3 — local receipts stay authoritative (P1)

**Independent Test**: the existing `--previous` tests pass, and a capture after a local receipt makes no review-list request.

- [X] T015 [US3] Add failing tests to `internal/cli/capture_test.go` and `internal/cli/show_test.go`: after a local round with a receipt, capture makes no `GET …/reviews` request, stores no `previous.json`, and reports `{"from":"receipt","round":R}`; `show --previous --json` gives today's result plus `"from":"receipt"`; with only unpublished local rounds, capture reads GitHub. Then implement the branch in `internal/cli/capture.go` with `run.PreviousPublished`, and add `from` to the receipt path in `internal/cli/show.go`.

## Phase 4: User Story 4 — an unreadable round degrades (P1)

**Independent Test**: seed `fakegh` with each unreadable review, capture, and check the result and the refusal.

- [X] T016 [US4] Add failing tests to `internal/cli/capture_test.go` and `internal/cli/show_test.go`, one per cause: the newest own review has no record, with an older one that has; a record changed on GitHub (`fakegh.EditReview`); an omitted record; a record of another version; `fakegh.Fail` on the reviews list with 502; no own loupe review at all. Each capture succeeds and reports `{"from":"none","reason":…}` naming the cause and the review URL when there is one. `show --previous` refuses `not-found`, its message carries the reason, and its fix is `loupe show --run <ref>`. Then implement the reasons in `internal/publish/previous.go` and the refusal message in `internal/cli/show.go`.
- [X] T017 [US4] Add a test to `internal/cli/show_test.go` that a run without `previous.json`, as captured by an older loupe, refuses exactly as today.

## Phase 5: User Story 2 — a human on a fresh machine (P2)

- [X] T018 [US2] Add an integration test to `internal/integration/rounds_test.go`: an attended round published from one data root with a user token, then a capture from an empty data root with the same viewer lists its findings; another user's newer loupe review is ignored.

## Phase 6: Polish and contracts

- [X] T019 [P] Amend `docs/comment-format.md`: the record line in the body example; a Markers entry for `loupe-findings` (placement, fields, encoding, checksum, omission line, at most one per body); the Sticky reviews rule that a collapsed round drops its record; the terminal stand-in beside the pill rule.
- [X] T020 [P] Amend `specs/001-loupe-v1/contracts/cli.md`: capture's `previous` key, and `show --previous`'s source order, `from` key and reason-bearing `not-found`.
- [X] T021 [P] Change step 2 of `plugin/skills/human-review/SKILL.md` to run `loupe show --previous` when capture's `previous.from` is `receipt` or `github`, and keep `internal/cli/plugin_test.go` green.
- [X] T022 Regenerate the CLI goldens with `go test ./internal/cli/ -update` and read every diff. Only the capture and show help and results may change.
- [X] T023 Run `mise run check` and show its output.

## Dependencies

- Phase 1 blocks every story: the record must exist before anything reads it.
- US1 blocks US3, US4 and US2, which extend capture's branch and the reasons US1 introduces.
- T019 to T021 can run beside any story. T022 and T023 are last.

## Parallel Example

```text
T001–T003 are one test file, written together; T008 can start once T004 lands.
T019, T020 and T021 touch separate documents.
```

## Implementation Strategy

Phase 1 and US1 are the MVP: they close the CI gap. US3 and US4 are P1 because a regression or a wrong list is worse than the gap. US2 adds no code beyond US1, only a test.

## Notes from implementation

- T012, T013 and T015 to T018 test capture end to end, so they live in `internal/integration/previous_test.go`, which has a clone and `fakegh`, rather than in `internal/cli`.
- T022 regenerated no CLI golden: the capture and show help are not goldened. The body goldens (T007) and the two hand-kept publish goldens changed by the record line only.
