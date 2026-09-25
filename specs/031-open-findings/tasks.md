# Tasks: Open findings across rounds

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task is test first: the failing test, then the change, then `mise run check`.

## Phase 1: Foundational

- [ ] T001 `draft`: `EarlierFinding`, `FiledIn`, `Assessment`, `Draft.Assessments` (omitted when empty), load validation, and `SetAssessments`.
- [ ] T002 `render`: record version 2 written only with assessments; `ReadRecord` reads both versions; the stand-in names the open and addressed counts.
- [ ] T003 `publish`: `Envelope.Assessments` from the draft into the record; `Previous` gains `commit` and `assessments`; `ReadPrevious` reads both; `Earlier` builds the list.

## Phase 2: User Stories 1 and 3

- [ ] T010 `cli`: `show --previous` adds `earlier`, from a receipt and from the stored round; the human view lists carried findings when there are any.
- [ ] T011 Integration: three unattended sticky rounds from empty data roots, then the same three from one data root.

## Phase 3: User Story 2

- [ ] T020 `cli`: `assess` with refs and `--status`, and with `--from`; refusals; result counts; help and golden.

## Phase 4: Contracts

- [ ] T030 `docs/comment-format.md`, `specs/001-loupe-v1/contracts/cli.md`, `show --help`.
