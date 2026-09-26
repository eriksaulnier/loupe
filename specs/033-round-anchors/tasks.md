# Tasks: Sticky rounds anchored in data

**Input**: Design documents from `specs/033-round-anchors/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (3.0.0)

**Tests**: Every behavior is pinned by a failing test before the code that makes it pass.

## Phase 1: Setup

- [ ] T001 Add `testdata/sticky-v0.13.0-layout.md`, a three-round unattended body the v0.13.0 layout wrote, with 032's note on the round on top, composed by the code before this feature.
- [ ] T002 Move `ReadSticky` and the helpers only it uses into `internal/render/sticky_legacy.go` as `readLegacy`, unchanged otherwise. Move the tests that exercise it into `internal/render/sticky_legacy_test.go`, feeding them bodies through a test helper that converts an anchored body to the v0.13.0 layout. No behavior changes in this task.

## Phase 2: Foundational — the anchor

- [ ] T003 Add failing tests in `internal/render/anchor_test.go`: an anchor round-trips through write and parse; a malformed anchor, a missing `n` or `commit`, a `v` other than 1 and a `sha256` that is not last are refused; the checksum covers the fields and the round's text, and reads CRLF as LF.
- [ ] T004 Add `internal/render/anchor.go` with the anchor's grammar, writer, parser and checksum.

## Phase 3: User Story 1 — a layout change adds no read-back branch (P1)

- [ ] T005 [US1] Add failing tests: `Body` opens a sticky body on the top anchor and writes one anchor before each collapsed round; a non-sticky body carries none; the note follows the footer with no delimiters and is counted; a round demoted from an anchored body is byte for byte the v0.13.0 reader's; a footer with a segment no pattern knows demotes; collapsed rounds are carried byte for byte; each refusal in FR-006.
- [ ] T006 [US1] Change `StickyInput.Earlier` to rounds, write anchors in `Body`, build chips and summary pills from one set of counts, and add the anchored reader and `ReadSticky`'s dispatch.
- [ ] T007 [US1] Update `internal/publish/envelope.go` and its tests: the length limit drops a round with its anchor, and the compare link reads the newest round's `n` and `commit`.
- [ ] T008 [US1] Regenerate `testdata/golden/sticky.md` and `sticky-three.md` and read the diff: only anchor lines change.

## Phase 4: User Story 2 — a hand edit is named (P1)

- [ ] T009 [US2] Add failing tests: a one-byte edit to a collapsed round carries its block byte for byte and marks it edited; an edit to the round on top collapses it whole; the next read after that finds no edit; an edited round that is no longer one disclosure refuses.
- [ ] T010 [US2] Carry edited rounds per FR-007.
- [ ] T011 [US2] Add failing tests that the confirmation shows anchors as `<!-- loupe-round N -->`, keeps the message slot inline, and names each edited round, in both the full-screen and plain confirmations, and that an unattended round names them on stderr. Add `render.AnchorsAsNotes`, `publish.Preview.Edited` and the notices.

## Phase 5: User Story 3 — old bodies continue (P2)

- [ ] T012 [US3] Add failing tests: each of the four fixtures reads back through the legacy reader, composes an anchored body, and that body reads back through the anchored reader and composes the next with every collapsed round byte for byte. A body with both an anchor and the legacy delimiter refuses.
- [ ] T013 [US3] Anchor the legacy reader's result in `ReadSticky`.
- [ ] T014 [US3] Add an integration test in `internal/integration/sticky_test.go`: three unattended sticky rounds with a note against the fake GitHub, every round anchored in the final body.

## Phase 6: Contracts

- [ ] T015 Amend the Sticky reviews section of `docs/comment-format.md`, keeping the rest byte for byte, and record the render probe in `docs/github-facts.md`.
- [ ] T016 Run `GOFLAGS=-buildvcs=false mise run check` and confirm every non-sticky golden is unchanged.
