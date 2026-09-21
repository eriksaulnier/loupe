# Feature Specification: One word for the set of findings that gets published

**Feature Branch**: `015-publishable-set`

**Created**: 2026-09-21

**Status**: Draft

**Input**: `specs/001-loupe-v1/tasks.md` T123 to T127, from the round 2 review of 2026-09-14, open since and never scheduled: "the repository says 'included' where it means the publishable set or what was published, and the readiness and dead-lock-holder wording do not match the code".

## Relationship to earlier specifications

This specification amends the two documents `CONTRIBUTING.md` names as contracts, `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md`, and closes T123 to T127 of `specs/001-loupe-v1`. It changes no behavior. Every refusal fires on exactly the same condition afterwards, and no published review renders differently.

- **Spec 001 T120** started this work and stopped halfway. It named the three sets apart in `data-model.md`, in `contracts/cli.md` and in the publish help, but it left one sentence of that help inconsistent with itself and left the prose everywhere else alone.
- **`specs/001-loupe-v1/data-model.md:151-153`** already defines the vocabulary. This specification does not invent a word; it makes the rest of the repository use the words that are written down there. It corrects one of them: the published set was defined as `accepted` alone before `specs/007-unattended-publish` let `--unattended` send pending findings, and the definition never caught up.
- The `Included` field keeps its name and its JSON key. Renaming a persisted field is a data migration and a different piece of work.
- Constitution 2.0.1 is unchanged. No principle is added, removed or redefined.

### Why this is worth a contract change

`included` is a boolean on a finding. It stays `true` on a finding the human excluded, so "included findings" in prose does not name the set a reader thinks it names. In `specs/001-loupe-v1/spec.md:238` it names a set that cannot exist: FR-019 derives readiness as "every included finding accepted", and an excluded finding is `included: true` and is never accepted, so readiness as written can never hold. The code does not do that. `internal/draft/derive.go` derives readiness as "no finding pending", which is the correct rule.

The three sets are distinct, and the distinction is the whole point:

| Term | Definition |
| :--- | :--- |
| `included` flag | a finding the human has not withdrawn |
| Publishable set | `included` and not human-excluded: disposition `accepted` or `pending` |
| Published set | what publish sends: disposition `accepted` when attended, the whole publishable set under `--unattended` |

The publishable and published sets differ until the moment publish sends. An attended publish reaches that moment only once readiness has gated out `pending`, so there they coincide; `--unattended` skips readiness and sends the publishable set whole. A reader who is told "included" for all three cannot tell which gate is being described, and `internal/cli/publish.go:31` currently says both words in one sentence: "no summary and no publishable findings; and, after you confirm, no included findings".

`docs/comment-format.md` is what a human reading a review on GitHub is pointed at. Seven of its lines say "included" where they mean a set: five mean the published set — what the body and its inline comments actually carry — and two mean the publishable set that the allowlist check at publish reasons over. A format contract that names the wrong set is describing a different document from the one loupe sends.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reader can tell which set is meant (Priority: P1)

Someone reading a refusal, the publish help, the format contract or spec 001 learns which of the three sets the sentence is about, without holding the definitions in their head and without checking the code.

**Why this priority**: It is the whole feature. The vocabulary already exists in `data-model.md`; the value is in the prose using it.

**Independent Test**: `grep -rn "included" docs/ internal/ specs/001-loupe-v1/` and read every hit. Each one either names the `included` flag, or is ordinary English, or has become `publishable` or `published`. No hit names a set with the flag's word.

**Acceptance Scenarios**:

1. **Given** the `empty` refusal before the confirmation, **When** its message is read, **Then** it says `publishable`, because the gate reasons over the publishable set.
2. **Given** the `empty` refusal after the confirmation, **When** its message is read, **Then** it says `published`, because readiness has already run and the gate tests what the body will carry.
3. **Given** the `blocking` refusal, **When** its message is read, **Then** it says `publishable`, because the gate reasons over the same set as the empty gate before it.
4. **Given** `loupe publish --help`, **When** the `empty` row is read, **Then** its two clauses use the two different words, and each matches the refusal it describes.
5. **Given** `docs/comment-format.md`, **When** a line naming a set is read, **Then** it says `published` for what the body and its inline comments carry, and `publishable` for the publish-time Markdown check and the refusal that check raises.
6. **Given** `specs/001-loupe-v1/spec.md` FR-019, **When** readiness is read, **Then** it states the rule the code derives: no finding pending and no note open.

---

### User Story 2 - Nothing moves (Priority: P1)

Every refusal fires on exactly the same input it fired on before, and every published review is byte for byte what it was.

**Why this priority**: A wording change that quietly moved a gate would be worse than the wording it fixed. The duplicate set helper in `internal/publish/gates.go` is deleted in favor of `draft.PublishableSet`, and that is the only code change with any risk.

**Independent Test**: `go test ./internal/publish/` passes unchanged, and `git diff --stat testdata/golden/` names only the two `publish-help` files.

**Acceptance Scenarios**:

1. **Given** the publish gates, **When** the suite runs, **Then** every refusal test passes without its input or its expectation changing, except the two that assert the reworded strings.
2. **Given** `go test ./internal/render/`, **When** it runs **without** `-update`, **Then** it passes, so no rendered body moved and `docs/comment-format.md` still matches the example golden.
3. **Given** the goldens under `testdata/golden/`, **When** the suite runs, **Then** only `publish-help.80.txt` and `publish-help.100.txt` need regenerating, and only on line 20.
4. **Given** a body golden that changed, **When** the diff is read, **Then** the change was not wording-only and this specification's premise is broken.

---

### Edge Cases

- **A sentence where the flag is genuinely meant.** `IncludedCount`, `--include` and `--exclude`, the `included` key in the `edit` result, `summary --expect-findings`, and `accept is for included findings only` all name the flag. They are correct and stay. The rule is not "replace the word"; it is "name the set you mean".
- **A sentence where the word is ordinary English.** "side included inside the span", "bidi overrides included", "including history", "including the hunk". Untouched.
- **The confirmation's head-moved list.** `Touched` is computed before readiness runs, so the computation reads the publishable set and the comment says so. The confirmation that displays it is only ever reached after readiness, so FR-043, which describes what the human sees, says published. Two words for one list, because they are two moments.
- **`internal/publish/envelope.go`'s local `included`.** It holds the publishable set when unattended and the accepted set when attended. It is a local variable in the one function that computes both, it is never read by a human outside that function, and renaming it is a code change with no prose to fix. Out of scope, and named here so a later reader knows it was seen.
- **Earlier specifications that use the word.** `specs/005`, `specs/013` and `specs/001-loupe-v1/research.md` record what was decided at the time. They are history, not contracts, and are not rewritten. Spec 001's `spec.md`, `plan.md` and `contracts/cli.md` are amended because the tasks in that directory name them.

## Requirements *(mandatory)*

- **FR-001**: `internal/publish/gates.go`'s private `included()` helper MUST be deleted, and its three call sites MUST use `draft.PublishableSet`, which already applies the same predicate and is already exported and used.
- **FR-002**: The `empty` refusal in `internal/publish/gates.go` and the `blocking` refusal in `ActionRefusal` MUST say `publishable`, because both gates reason over the publishable set.
- **FR-003**: The `empty` refusal in `internal/publish/publish.go`, raised after the confirmation, MUST say `published`, because it tests the accepted-only envelope after readiness has run.
- **FR-004**: `loupe publish`'s long help MUST mirror FR-002 and FR-003 in its `empty` row, with the two clauses carrying the two different words. The sentence MUST NOT use one word for both gates.
- **FR-005**: `docs/comment-format.md` MUST say `published` where it names what the body or its inline comments carry, and `publishable` where it names the publish-time Markdown check and the refusal that check raises. The `--inline` table and the body lines MUST use the same word, because both are filled from one slice and the contract says `--inline` never changes what the body contains. Lines where the word is ordinary English MUST be left alone.
- **FR-006**: `specs/001-loupe-v1/spec.md` FR-019 MUST state readiness as the code derives it: no finding pending and no note open.
- **FR-007**: `specs/001-loupe-v1/spec.md`, `plan.md` and `contracts/cli.md` MUST say `publishable` or `published` wherever they name a set, and MUST keep `included` wherever they name the flag or a count of it. Where spec 001's meaning is corrected rather than reworded — FR-019 and the lock edge case — it MUST carry a dated pointer to this specification, as FR-025 does for spec 007.
- **FR-008**: `specs/001-loupe-v1/spec.md`'s dead-lock-holder edge case MUST match `internal/run/lock.go` and `plan.md`: the kernel releases a flock when its holder dies, so a timeout means a live holder; the refusal names the holder's pid and command and says to wait for it or stop it; loupe never removes the lock file. `plan.md`'s "stale-lock refusal text" MUST be corrected for the same reason.
- **FR-009**: No behavior MAY change. Every refusal MUST fire on exactly the same condition, no refusal code MAY change, and no exit code MAY change.
- **FR-010**: No published review MAY render differently. The only goldens that MAY move are `testdata/golden/cli/publish-help.80.txt` and `publish-help.100.txt`, and only the line carrying the reworded help.
- **FR-011**: `specs/001-loupe-v1/tasks.md` MUST check off T123 to T127, and its "Remaining gaps" paragraph MUST stop linking `github.com/eriksaulnier/loupe/issues/22`, which does not exist: the repository has no issues, and the link survived the republish from `loupe-archive`. It MUST point at `specs/015-publishable-set/` instead.
- **FR-012**: The `Included` field MUST keep its name and its JSON key, and no stored run MAY need migrating.
- **FR-013**: `specs/001-loupe-v1/data-model.md` MUST define the published set as what publish sends — disposition `accepted` when attended, the whole publishable set under `--unattended` — with a dated pointer to this specification, and MUST limit its claim that readiness makes the two sets equal to an attended publish, as MUST the same claim in `plan.md`'s digest bullet. FR-026 MUST say that under `--unattended` the published findings include pending ones.

### Key Entities

- **`included`**: a boolean on a finding saying the human has not withdrawn it. Unchanged, and after this change it is the only thing the word names.
- **Publishable set**: the findings the human could still publish, `included` and not human-excluded. What the gates before the confirmation reason over.
- **Published set**: the findings publish sends: `accepted` when attended, the whole publishable set under `--unattended`. What the body and its inline comments carry.

## Success Criteria *(mandatory)*

- **SC-001**: No sentence in `docs/`, `internal/` or `specs/001-loupe-v1/` uses `included` to name a set of findings.
- **SC-002**: A reader of `loupe publish --help` can tell the pre-confirmation gate from the post-confirmation gate by the word alone.
- **SC-003**: `go test ./internal/publish/` passes with no test input or expectation changed except the two asserting the reworded strings.
- **SC-004**: `go test ./internal/render/` passes without `-update`, and no body golden changes.
- **SC-005**: `git diff --stat testdata/golden/` names only the two `publish-help` files.
- **SC-006**: `mise run check` passes after the final edit.

## Assumptions

- `draft.PublishableSet` and `internal/publish/gates.go`'s `included()` compute the same thing. Both walk `d.Findings` in order and keep dispositions `accepted` and `pending`; `Dispositions` is a map built by calling the same private `disposition` over the same slice. Deleting one is a deduplication, not a change. The one input on which they differ is a draft holding two findings with the same id: `Dispositions` is keyed by id, so the last one's disposition applied to both, while `PublishableSet` judges each finding on its own. loupe's commands cannot write such a draft and `draft.Load` does not refuse a hand-edited one; the gates now agree with `Digest` and `Build`, which already used `PublishableSet`.
- The three sets are the right vocabulary, with the published set's definition brought up to date with spec 007. They are already written down in `data-model.md`, already used in `envelope.go` and the publish help, and adding a fourth word would be a specification of its own.
- Whether the words make a reader faster is not claimed. It is a prose change with no test that can stand in for a reader.
