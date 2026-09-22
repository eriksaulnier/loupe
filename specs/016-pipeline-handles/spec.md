# Feature Specification: Handles a pipeline can hold

**Feature Branch**: `016-pipeline-handles`

**Created**: 2026-09-21

**Status**: Draft

**Input**: Draft item "Draft spec: handles a pipeline can hold" on the loupe project board, created 2026-09-16, written after building `.github/workflows/review.yml` against spec 007 and running it live that day: "Three places the workflow had to reach past the CLI contract to get work done, and one question the contract answers in a way that suits a person at a terminal rather than a pipeline." Scoped on 2026-09-21 by the owner: the draft's diff story and its workflow requirements are dropped, for the reasons under Relationship.

## Relationship to earlier specifications

This specification extends `specs/001-loupe-v1/contracts/cli.md` and rests on `specs/007-unattended-publish`, which made a pipeline a first-class caller. Nothing here changes the publication state machine, the draft model, the review interface or the published format; specs 008 and 009 own what a review says and how its footer reads, and this one owns what a caller can get back from the commands.

Spec 007 FR-002 says capture is the only step that needs the clone, and FR-003 says publish works from a data root alone. Both hold. What they do not give a caller is any way to *name* the run directory inside that data root without knowing loupe's directory layout, or to learn from a publish result who posted.

- **`specs/010-captured-diff` already shipped the draft's first story.** The draft asked for `loupe show --diff`; spec 010 added it (`c8c2635`), and `contracts/cli.md` documents it. Spec 010 ruled differently from the draft on two points, and its rulings stand: `--diff --json` returns the diff as a `diff` string rather than refusing, and `--diff --previous` refuses with `usage`. This specification does not revisit either.
- **The workflow moved.** `.github/workflows/review.yml` is now a caller for the shared `eriksaulnier/loupe-workflows` reusable workflow. Whatever paths that workflow still builds out of loupe's layout live in that repository, so no requirement here names them. `loupe-workflows` is the consumer this specification serves; switching it to the new fields is its own change.
- **`specs/001-loupe-v1` FR-010** makes a batch store entirely or not at all. That rule stands for every caller (clarified 2026-09-21). This specification changes only what a refused batch reports.

### What prompted it

The workflow archives and restores the data root between jobs, and keeps a run's directory as the evidence of a publication. To name that directory it builds `$LOUPE_HOME/runs/<repo>/<pr>/<round>`, which is how `internal/run` happens to lay runs out today and which nothing in the contract promises. `.github/review-instructions.md` already tells reviewers that reaching into that layout from outside loupe is itself a finding.

Separately, a publish step's `--json` result carries `sent`, `reviewUrl` and `reviewId`. Whether the publication was unattended, and which account posted it, are both recorded in the receipt and neither is in the result.

## Clarifications

### Session 2026-09-21

- Q: Should a pipeline be able to ask `loupe add` to store the good entries of a batch and report the bad ones, instead of storing nothing? → A: No. All-or-nothing stays for every caller, but a refused batch names every bad entry, each with its own message, fix and details, so one retry can fix the whole batch.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pipeline can name the directory it must carry (Priority: P1)

A job captures, then hands the run to a later job, or archives it as the evidence of a publication. It gets the run's directory from loupe's result rather than assembling it from the pull request's parts.

**Why this priority**: Spec 007 FR-003 makes carrying the data root a supported flow, and naming the run inside it is the one step a caller currently has to do by knowing loupe's internals.

**Independent Test**: With a captured run, every agent-facing `--json` success result that carries `run` also carries an absolute path, and that path is the directory holding the run's `target.json`.

**Acceptance Scenarios**:

1. **Given** any command that resolves a run and prints a `--json` success result, **When** it succeeds, **Then** the result names the run's directory as an absolute path.
2. **Given** `loupe capture --json`, **When** it prints its result, **Then** the directory named is the one the run was just written to.
3. **Given** a data root restored at a different path, even on a different machine, **When** a command resolves the run, **Then** the directory named is under the restored root, not where capture first wrote it.
4. **Given** a command whose result has no `run` because none resolved, **When** it prints its result, **Then** the directory field is absent too.

---

### User Story 2 - A CI log says what was published and by whom (Priority: P2)

A publish step's output is read by whoever is looking at a failed or surprising run. For an unattended publication the two questions are whether it was unattended and which account posted it. Both are already in the receipt.

**Why this priority**: It saves a reader opening the review on GitHub or the receipt on disk, but no pipeline is blocked without it.

**Independent Test**: An unattended `loupe publish --json` against the fake GitHub returns a result that says it was unattended and names the login the fake returned for the review.

**Acceptance Scenarios**:

1. **Given** a successful unattended publish with `--json`, **When** the result is read, **Then** it says the publication was unattended and names the posting login.
2. **Given** a receipt replay of an unattended publication, **When** the result is read, **Then** it says the same as the publish that wrote the receipt.
3. **Given** a receipt written before loupe recorded the posting login, **When** it is replayed, **Then** the login field is absent rather than empty or guessed.
4. **Given** an attended publish, **When** the result is read, **Then** every field it carries today is present with the same meaning.

---

### User Story 3 - A refused batch is fixed in one retry (Priority: P2)

An unattended reviewer files ten findings and three have bad locations. loupe still stores none of them, but the refusal names all three, so the reviewer corrects the batch once and resubmits it.

**Why this priority**: The all-or-nothing rule is right; what costs a pipeline is that the refusal stops at the first bad entry (`internal/draft/mutate.go`), so a batch with three bad entries takes three retries. A reviewer spending turns on a bad retry is the cost the draft observed live.

**Independent Test**: A batch of ten entries with bad entries at positions 2, 5 and 8 refuses once, stores nothing, and the refusal names 2, 5 and 8, each with the message, fix and details it would have got as the only bad entry.

**Acceptance Scenarios**:

1. **Given** a batch with several invalid entries, **When** `loupe add --json` runs, **Then** nothing is stored, the draft version does not change, and the refusal lists every invalid entry by zero-based position.
2. **Given** that refusal, **When** each listed entry is read, **Then** it carries the same code, message, fix and details that entry would get as the only invalid one in the same batch, including `nearest` for a bad location.
3. **Given** a batch whose entries fail for different reasons, such as one bad location and one unknown field, **When** it is refused, **Then** both are listed.
4. **Given** a batch with exactly one invalid entry, **When** it is refused, **Then** the refusal's existing code, message, fix and `details.entry` are what they are today.
5. **Given** a failure that belongs to the whole call rather than to an entry, such as a version mismatch or input that is not a finding or an array of findings, **When** it is refused, **Then** it refuses as it does today, with no entry list.

---

### Edge Cases

- **A batch with one bad location, filed by a pipeline.** `loupe add` stores a batch entirely or not at all (spec 001 FR-010), and that does not change: there is no opt-out for pipelines or anyone else. A caller that wants the good entries stored resubmits the batch with the bad ones fixed or removed. User Story 3 makes that one retry rather than one per bad entry.
- **Several refused entries where the first is the one that matters.** Code, message, fix and `details.entry` still describe the first bad entry, so a caller that reads only those behaves as it does today.
- **The run directory is not the layout.** Naming the directory tells a caller where the run is, not what is inside it. A caller that reads files out of the named directory is still reaching into loupe's internals.
- **Refusals.** A refusal that resolved a run already carries `run`. Whether it also names the directory is left to the plan; a caller archiving evidence of a failed step is the case for it.
- **`list` and `show --previous`.** `list` resolves many runs, not one, and `show --previous` reads a published round. Whether either names a directory is left to the plan, bounded by FR-001.

## Requirements *(mandatory)*

- **FR-001**: Every agent-facing `--json` success result that carries `run` MUST also name that run's directory. The field MUST be an absolute path, and MUST be absent whenever `run` is.
- **FR-002**: The directory named MUST be the directory of the run the result's `run` names, resolved under the data root in force for that invocation. For `show --previous` that is the selected run, not the earlier round the findings were read from; the payload's `round` names that.
- **FR-003**: Naming the directory MUST NOT make anything inside it part of the contract. `contracts/cli.md` MUST say so where it documents the field.
- **FR-004**: A successful `loupe publish --json` result, including a receipt replay or reconciliation, MUST say whether the publication was unattended, and MUST name the login that posted when the receipt records one. When the receipt does not record one, the login field MUST be absent.
- **FR-005**: An attended publish's `--json` result MUST keep every field it has today with the same meaning. New fields MAY be added.
- **FR-006**: `specs/001-loupe-v1/contracts/cli.md` and each affected command's `--help` MUST document the new fields and their absence rules. `docs/comment-format.md` is untouched.
- **FR-007**: No command MAY change how it selects a run, write a run file it did not write before, or make a network call it did not make before, to produce the new fields.
- **FR-008**: `loupe add` MUST keep storing a batch entirely or not at all. When it refuses a batch because of one or more invalid entries, it MUST validate every entry and name each invalid one by zero-based position, with that entry's own code, message, fix and details. The refusal's top-level code, message, fix and `details.entry` MUST keep describing the first invalid entry.
- **FR-009**: `contracts/cli.md` and `loupe add --help` MUST document the list of invalid entries and say that no entry of a refused batch is stored.

### Key Entities

- **Run directory**: the one directory holding a run's target, diff, draft and publication records. Already the unit of storage; this specification makes it a thing a caller can name, not a thing a caller can look inside.
- **Receipt**: the record of a sent review. It already holds the envelope, which says whether the publication was unattended, and `author`, the login GitHub returned for the review.

## Success Criteria *(mandatory)*

- **SC-001**: A pipeline can capture, carry the run to another job, archive its directory and publish using only documented commands and the fields of their results, with no path built from loupe's layout.
- **SC-002**: Every command whose `--json` success result carries `run` also names an absolute directory, checked against the fake GitHub and a local remote.
- **SC-003**: An unattended publish's `--json` result names the posting login and says it was unattended, on first send and on replay.
- **SC-004**: An attended publish's `--json` result keeps every field it has today.
- **SC-005**: A batch with any number of invalid entries, each with one defect, is correctable in one retry: a resubmission that fixes every entry the refusal listed stores the whole batch. An entry reports its first defect only, as it does today, so an entry with two defects can still take a second retry.
- **SC-006**: `mise run check` passes after the final edit.

## Assumptions

- The caller already knows the data root. It either set `LOUPE_HOME` or accepted the documented default, so the run directory is the only handle it lacks.
- The field sits in each result's payload or in the shared envelope; which one is a plan decision. Either way it is additive and no existing field moves.
- The login comes from the receipt's `author`, which is what GitHub returned for the review. loupe does not ask GitHub again to fill it for an old receipt.
- The batch rule keeps its all-or-nothing contract because the problem has been observed once, and one live run is not enough to trade a contract for. Reporting every bad entry is additive and needs no such evidence.
- No push, release, pull request or live review is authorized by this specification.
