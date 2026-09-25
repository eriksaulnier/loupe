---

description: "Task list for quoted earlier rounds"
---

# Tasks: Quoted earlier rounds

**Input**: Design documents from `specs/030-quoted-rounds/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (3.0.0)

**Tests**: The published format changes, so every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). Goldens are regenerated only after a hand-written test pins the change, and every regeneration is followed by reading the diff. Only sticky goldens MAY change, and only in their collapsed rounds (SC-003). Every new read-back guard is confirmed by mutation: break it, check that exactly the intended test fails, restore it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — quoted lines read as GitHub reads them

- [X] T001 [P] Add tests to `internal/markdown/allowlist_test.go` for `OneDisclosure` over a quoted round: `<details>`, `<summary>`, a blank `>`, a quoted finding `<details>` holding a fenced `</details>` and a `> > ` location line, a quoted footer, then the reconciliation marker and `</details>` outside the quote, balances. A stray `> </details>` and an extra `> <details>` inside the quote each refuse.
- [X] T002 [P] Add failing tests to `internal/markdown/allowlist_test.go`: `OpenDetails` opens `> <details>` and `> > <details>` and leaves a `<details>` inside a quoted fence as written; `MapSummaryLines` maps `> <summary>` lines and not a quoted fenced one. Then make `mapTagLines` in `internal/markdown/allowlist.go` read each run of structural lines that start with `>` with one marker off, recursively. `Check` and `StructuralLines` MUST NOT change.

## Phase 2: User Story 1 — an opened round has an edge (P1)

**Goal**: the round a sticky round collapses is one blockquote with no divider.

**Independent Test**: publish two sticky rounds and compare the body with a golden in which round 1 is quoted.

- [X] T003 [US1] Change the failing expectations in `internal/render/sticky_test.go` to the quoted layout: `TestReadStickyDemotesTheCurrentRound` pins the whole demoted round (`> #### Must fix`, `>`, the quoted finding, `> ` and the footer, a blank line, the marker, `</details>`); add tests that a round with prose and both sections drops the dividers before each section and around the footer and keeps a divider in the prose; that a clean round with no prose quotes its footer alone; that a finding's location line reads `> > `; and that no `---` line sits between the demoted round's `<summary>` and its `</details>` unless the author wrote it. Update the other render tests that pin the old demotion (`TestStickyBodyKeepsEachFooterUnderItsRound`, `TestReadStickyNumbersRoundsInTheReview`, `TestReadStickyReadsAFullFooter`, `TestStickyMovesAV0110BodyToTheNewLayout`, `TestStickyV0110BodyKeepsFooterShapedProse`, `TestReadStickyKeepsProseThatLooksLikeASection`, `TestReadStickyReadsACleanRoundWithoutItsChip`).
- [X] T004 [US1] Implement the quoted demotion in `internal/render/sticky.go`: drop loupe's section dividers in `splitChips`, and quote the content in `ReadSticky`. Teach `keepsFooter` the quoted footer so the next round reads the body back.
- [X] T005 [US1] Add a three-round golden, `sticky-three.md`, to `TestBodyGoldens` in `internal/render/body_test.go`. Regenerate with `go test ./internal/render/ -update` and read the diff: `sticky.md` changes in round 1 only, and no other golden changes.
- [X] T006 [US1] Update the publish and integration tests that pin a collapsed round (`internal/publish/sticky_test.go`, `internal/publish/envelope_test.go`, `internal/integration/sticky_test.go`) to the quoted layout, and add an integration assertion that a third round carries round 1 byte for byte apart from its `Round N`.

## Phase 3: User Story 2 — existing sticky reviews keep working (P1)

**Independent Test**: read back each earlier-layout fixture, including the v0.12.0 one, and publish a round over it.

- [X] T007 [US2] Add `testdata/sticky-v0.12.0-layout.md`, a three-round body composed by v0.12.0 (`a489c82`), and a test in `internal/render/sticky_test.go`: it reads back, the round on top is quoted, rounds 1 and 2 are carried byte for byte apart from `Round N`, and the next body reads back again. Keep the v0.11.0 and pre-025 fixture tests green.
- [X] T008 [US2] Add failing cases to `TestReadStickyRefusesUnbalancedRounds`: a newest quoted round whose `> ` footer line was removed; a newest quoted round whose footer lost its `> `; a quoted round, newest or older, in which a prose line, a heading line or a finding line lost its `> `. Each refuses. A quoted round with `>text` and no space reads back. Implement `quotedRound` and the check in `internal/render/sticky.go`.
- [X] T009 [US2] Confirm each new guard by mutation: the quoted footer in `keepsFooter`, the quoted-round detection, and the lost-marker check. Break each, run `go test ./internal/render/`, check that exactly the intended cases fail, restore it.

## Phase 4: Polish and contracts

- [X] T010 [P] Amend the Sticky reviews section of `docs/comment-format.md`: the example's round 1 in the quoted layout, the collapsed round bullet (FR-001 to FR-003), the read-back refusals (FR-006), and the v0.12.0 layout among the earlier bodies that still read back (FR-005, FR-007).
- [X] T011 [P] Record in `docs/github-facts.md` that `<details>` and a nested `> >` quote render inside a blockquote (observed 2026-09-25 with `POST /markdown`), and that a code block indented by a tab does not survive `> `.
- [X] T012 Run `cleanup-comments` over the branch diff, then `mise run check`, and show its output.

## Dependencies

- Phase 1 blocks nothing in render, but T002 lands before the confirmation shows a quoted round.
- T003 and T004 block T005 to T008. T009 follows T008.
- T010 and T011 can run beside any story. T012 is last.

## Implementation Strategy

User Story 1 is the MVP. User Story 2 is P1 too, because a series that refuses after an upgrade is worse than the look this feature fixes, so both land before the contract is amended.

## Notes from implementation

- T006 needed no change in `internal/publish`: its tests pin no collapsed round. The integration test gained the byte-for-byte assertion.
- T003 moved `TestReadStickyReadsARoundWithoutItsClosingDivider` onto the v0.12.0 fixture, since a round collapsed by this loupe has no closing divider to remove. So the fixture landed with the demotion, and T007 added its own test after.
- T005's regeneration changed `sticky.md` in round 1 and in its findings record's `sha256=`, which covers the whole body (spec 027). The newest round's visible lines did not change. No other golden changed.
- A quoted round whose content is only blank lines panicked the first build of `quoteIntact`. A case in `TestReadStickyRefusesABrokenQuote` pins the fix.
- `TestPillsAsWordsReachesAQuotedRound` pins T002 at the render level. It fails against the markdown package before T002.
- T009's mutations: `keepsFooter` accepting any quoted last line fails the two newest-round footer-removed cases; ignoring a footer that lost its marker fails only `older round's footer lost its marker`; disabling the lost-marker check fails the five lost-marker cases; requiring `> ` rather than `>` fails the `>text` acceptance; detecting a quoted round by a leading `>` fails `TestReadStickyCarriesAnOlderRoundThatOpensOnAQuote` and two lost-marker cases.
