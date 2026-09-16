# Feature Specification: A footer for the people reading the review

**Feature Branch**: `009-review-footer`

**Created**: 2026-09-16

**Status**: Draft

**Input**: Owner, 2026-09-16, after watching unattended and human reviews land on the same pull requests: "Maybe the fix is removing `loupe` from the footer. If my plan is to have co-workers using this, it could be good to have sorted. Some data could also just be meta-only."

## Relationship to earlier specifications

This specification amends the Footer section of `docs/comment-format.md`, which is a contract, and with it `specs/001-loupe-v1/spec.md` FR-042 and `specs/007-unattended-publish/spec.md` FR-016. Nothing about how a review is composed, published or reconciled changes: the hidden markers, the digest, the publication id and every gate stay exactly as they are.

- **FR-042** (spec 001) says the footer and `loupe-meta` both carry the review's number. Only `loupe-meta` carries it after this. How the number is worked out does not change, on either path.
- **FR-016** (spec 007) says ` · unattended` follows `round N` directly in the footer. It keeps its place in `loupe-meta` and moves in the footer, since there is no longer a round for it to follow.
- The `loupe-meta` and reconciliation markers are unchanged. `docs/comment-format.md` says the reconciliation marker's format MUST NOT change between versions that may need to reconcile each other's attempts, and this specification does not touch it.

### Why this is worth a contract change

The footer is read by people who did not install loupe and will not run it. Today it opens with the tool's name and the round number, which are the two things such a reader can do nothing with, and it ends with the thing they most want, which is who reviewed them.

The round number is worse than merely unhelpful. An attended review numbers itself from local run state and an unattended one from the pull request's bot reviews (spec 007 FR-015), so a pull request reviewed both ways shows two interleaved sequences: a human `round 1` can sit under a bot `round 2`, and neither is wrong. That is documented under Footer today as something a reader must understand. Moving the number into `loupe-meta` makes it what it always was in practice — a key for tooling — and the confusion stops being a reader's problem without changing a single count.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A co-worker reads a review (Priority: P1)

Someone who has never heard of loupe opens a pull request and finds a review on it. The footer tells them the commit it was written against and who wrote it, and says outright when nobody read it before it was posted. It does not ask them to care which tool composed it or which round it is.

**Why this priority**: It is the whole feature, and the audience is everyone the reviewer does not employ.

**Independent Test**: Render a published body for a run captured with a source and compare the footer against the golden; render one without a source; render one published unattended. No form names loupe or a round.

**Acceptance Scenarios**:

1. **Given** a run captured with no source, **When** its review is published attended, **Then** the footer reads ``reviewed `a1b2c3d` `` and nothing else.
2. **Given** a run captured with `--source claude-ci@1.2.0`, **When** its review is published, **Then** the footer reads ``reviewed `a1b2c3d` · via `claude-ci 1.2.0` ``.
3. **Given** a review published with `--unattended`, **When** it is read, **Then** the footer ends with ` · unattended`, after the source when there is one.
4. **Given** any published review, **When** its raw Markdown is read, **Then** `loupe-meta` still carries `round=`, and `unattended=1` when it applies, in the positions `docs/comment-format.md` gives them.
5. **Given** a pull request holding an attended and an unattended review, **When** both are read, **Then** no round number appears in either footer, and the two are told apart by their author and by ` · unattended`.

---

### User Story 2 - A tool reads the review (Priority: P2)

Something built on loupe's published format — a dashboard, a follow-up round, loupe's own reconciliation — still finds everything it needs in the markers, and finds it where it was.

**Why this priority**: The markers are the machine contract. A footer change that broke them would be a format break dressed as a wording change.

**Independent Test**: `go test ./internal/publish/ ./internal/render/` passes unchanged except for the footer goldens, and an unattended publish still numbers itself from the pull request's bot reviews.

**Acceptance Scenarios**:

1. **Given** an unknown attempt, **When** the next publish reconciles it, **Then** it matches on the same marker line as before, since neither hidden comment changed.
2. **Given** a pull request with earlier bot reviews, **When** a pipeline publishes unattended, **Then** the round in `loupe-meta` is what it would have been before this change.
3. **Given** a reader that takes the highest `round=` as the live review, **When** it reads a pull request published by one path, **Then** it still works.

---

### Edge Cases

- **A pull request reviewed both ways.** The disjoint sequences survive in `loupe-meta`, so two reviews MAY still carry the same `round=`. That is now a curiosity for tooling rather than something a reader is asked to reconcile, and `docs/comment-format.md` says so where it documents the key.
- **A review with no source and no unattended marker.** The footer is one segment. It MUST NOT collapse to an empty line or lose the leading `reviewed`.
- **Someone looking for what produced the format.** The reconciliation marker still opens `<!-- loupe digest=…`, so the name is one raw-Markdown read away. That is where the answer belongs.
- **Earlier reviews already on GitHub.** They keep their old footers. Nothing rewrites a published review, and a reader will see both forms on a long-lived pull request.

## Requirements *(mandatory)*

- **FR-001**: The footer MUST NOT name loupe as the tool that composed the review. The ` · via ` segment names what filed the findings, as capture recorded it, so a reviewer that calls itself `loupe` still names itself there; that is the caller's own name, not loupe announcing itself.
- **FR-002**: The footer MUST NOT carry the review's round number. `loupe-meta` MUST keep `round=` unchanged, including how it is derived on both paths.
- **FR-003**: The footer MUST open with ``reviewed `SHA` ``, the abbreviated captured head commit, as a generated code span under the existing backtick rule.
- **FR-004**: The ` · via `NAME VERSION`` segment MUST follow it when capture recorded a source, on the same terms as today.
- **FR-005**: ` · unattended` MUST come last, and only on a review `loupe publish --unattended` sent. `loupe-meta` MUST keep `unattended=1` where it is.
- **FR-006**: The reconciliation marker and every other `loupe-meta` key MUST be untouched, so a version before this change and a version after it reconcile each other's attempts.
- **FR-007**: `docs/comment-format.md` MUST show the new forms under Footer, MUST move the numbering rules to the `loupe-meta` `round=` bullet, and MUST keep the shared-`N` explanation there rather than dropping it.
- **FR-008**: `specs/001-loupe-v1/spec.md` FR-042 and `specs/007-unattended-publish/spec.md` FR-016 MUST carry a pointer to this specification, as spec 001 FR-025 does for spec 007.
- **FR-009**: The render goldens and `testdata/golden/example.md` MUST be regenerated together, and `render.TestExampleGoldenMatchesDoc` MUST keep passing, so the document and the goldens cannot drift.

### Key Entities

- **Footer**: the last visible line of a published review. After this change it answers two questions — which commit, and who reviewed it — and flags the one case where nobody read the review first.
- **`loupe-meta`**: the hidden inventory. It gains nothing and loses nothing here; it simply becomes the only place the round lives.

## Success Criteria *(mandatory)*

- **SC-001**: A reader who has never used loupe can say which commit a review was written against from the footer alone, and who produced it from the footer when capture recorded a source, or from the review's author when it did not.
- **SC-002**: No footer names a tool the reader does not run, or a number they cannot act on.
- **SC-003**: Every published review is still reconcilable by a loupe built before this change, and every marker key keeps its meaning.
- **SC-004**: The round an unattended review records is what it would have recorded before this change, on the same pull request.
- **SC-005**: All automated repository checks pass after the final edit.

## Assumptions

- The audience for the visible footer is a co-worker reading a pull request, not the person who configured the pipeline. The person who configured it reads `loupe-meta`, or the run directory.
- `via <source>` is worth keeping visible because it names the reviewer. If a pipeline does not set `--source`, its reviews say only what commit they read, which is the honest result of not naming itself.
- Nothing rewrites reviews already published, and no migration is offered or needed.
- This is a wording and placement change. If it turns into a discussion about what the round number means, that is spec 001 FR-042's business, not this specification's.

