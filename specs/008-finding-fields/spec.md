# Feature Specification: Richer findings and run provenance

**Feature Branch**: `008-finding-fields`

**Created**: 2026-09-15

**Status**: Draft

**Input**: Owner decisions on 2026-09-15, after the first unattended review on eriksaulnier/loupe#18: "The meta line rendered `**Confidence:** medium · severity \`minor\`` exactly as the contract says, and it reads as broken. Make severity an enum rendered like confidence. Add impact as its own section, verified as reproduced or plausible, and references as a list of URLs. Record the model per run from capture, in `loupe-meta` only, and keep it optional: if it is sometimes not known and it blocks findings, that is no good. The action always knows its model and fails without it."

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/spec.md` in two places and changes nothing else. FR-007 and the Finding entity gain three optional fields and narrow one; the capture command gains one optional flag. Constitution 2.0.0 is unchanged: no principle is added, removed or redefined, so its version stays. The amendment is required by the Boundaries section, because the published comment format changes.

Two pinned contracts change, and this specification lists the changes so `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md` can be updated from it:

- `docs/comment-format.md`: the finding vocabulary table, the meta-line rendering of severity, a verified part on the meta line, an Impact section and a References section inside the disclosure, and a `model=` key in `loupe-meta`. The footer is unchanged byte for byte.
- `specs/001-loupe-v1/contracts/cli.md`: `capture` gains `--model`; `add` and `edit` gain `impact`, `verified` and `references` with their flags and clears; `severity` is an enum.

The draft model's readiness rules, the send-back loop, the review interface's decisions and the publication state machine are unchanged. Everything a human does in `review` stays as it is; the new fields are shown, not decided.

### Why these fields

Every automated reviewer worth reading already reports more than a title and a body: a severity it can be sorted by, whether it ran the failure or reasoned about it, what actually goes wrong, and where it looked. Today two of those have no home in loupe and one is free text a reader cannot compare across findings. The result on #18 was a body that buried the impact in prose and a meta line that set an enum beside a code span.

- **Severity** is an enum because a reader compares it across findings and a marker counts it. Free text made it a code span; an enum makes it a word.
- **Impact** is a section because it is the one part of a finding a reader skims for: what breaks, with what input. It is Markdown, so it can hold the failing case.
- **Verified** is separate from confidence because they answer different questions. Confidence is how sure the reviewer is. Verified is whether it ran the failure or reasoned to it.
- **References** are a list because URLs in prose are lost, and a list of autolinks is the one form GitHub renders without escaping surprises.
- **Model** is per run, not per finding, because one run has one reviewer. It goes in `loupe-meta` and not the footer, because a pull request reader has the reviewer's name from `via` and the model is provenance for tooling.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reviewer files a finding with all of its fields (Priority: P1)

An agent files a finding with `severity: "major"`, `verified: "reproduced"`, an `impact` paragraph and two `references`. `loupe show` prints them, the review interface shows them, and the published review renders the meta block as `**Confidence:** high`, `**Severity:** major` and `**Verified:** reproduced` on three lines, an `**Impact**` section between the body and the suggested fix, and a `**References**` list after it.

**Why this priority**: It is the feature. The model and the compatibility rules exist to serve it.

**Independent Test**: Capture against a local Git remote and the fake GitHub, add one finding from JSON with every field, publish, and read the review body the fake received.

**Acceptance Scenarios**:

1. **Given** a finding input with `severity`, `verified`, `impact` and `references`, **When** `loupe add --from` runs, **Then** the finding is stored with all four and `loupe show` prints each.
2. **Given** a stored finding with confidence, severity and verified, **When** it is published, **Then** the meta block holds `**Confidence:** high`, `**Severity:** major` and `**Verified:** reproduced` on three lines after the location's, in that order, each separated by a hard break, with an absent part leaving no line.
3. **Given** a finding with `impact` and a suggested fix, **When** it is published, **Then** the disclosure holds the body, then `**Impact**` and the impact text, then `**Suggested fix**` and the fence.
4. **Given** a finding with two references, **When** it is published, **Then** the disclosure ends with `**References**` and one `- <url>` bullet per reference, in input order, after the suggested fix.
5. **Given** a finding with `severity: "P2"`, **When** `loupe add` runs, **Then** it refuses with `input` naming the four accepted words and stores nothing.
6. **Given** a finding with `verified: "yes"`, **When** `loupe add` runs, **Then** it refuses with `input` naming `reproduced` and `plausible`.
7. **Given** a finding with seven references, or one reference with a space, a no-break space, a bidi override, a `<`, a backtick, a control byte, no host or a scheme other than `http` or `https`, **When** `loupe add` runs, **Then** it refuses with `input` naming the failing entry and the rule.
8. **Given** an `impact` that fails the Markdown allowlist, **When** `loupe add` runs, **Then** it is refused exactly as a failing `body` is.
9. **Given** a stored finding, **When** `loupe edit` sets `impact`, `verified` or `references` to `null`, **Then** the field is cleared, the revision increments and the human's decision is removed, as for `severity`.
10. **Given** the same finding published with and without the new fields, **When** the two bodies are compared, **Then** the chips row, the headings, the summary lines, the footer and the markers are identical, and only the disclosure bodies differ.
11. **Given** an inline comment for a blocking finding, **When** it is published, **Then** its meta block carries the same parts in the same order as the body's, minus the location line, since the comment already sits on that line.

---

### User Story 2 - A run records which model reviewed (Priority: P2)

A pipeline captures with `loupe capture <url> --source my-reviewer@1.0.0 --model anthropic/claude-sonnet-5`. The published review's footer is unchanged, and `loupe-meta` ends `src=my-reviewer@1.0.0 model=anthropic/claude-sonnet-5`. A human capturing by hand passes neither, and the review is byte for byte what it was.

**Why this priority**: Provenance for automated reviews was the second thing #18 lacked, and it is a small change once the field plumbing exists.

**Independent Test**: Capture with `--model`, publish against the fake GitHub, and read `loupe-meta` in the body it received. Capture without it and diff the body against the golden.

**Acceptance Scenarios**:

1. **Given** `loupe capture --model anthropic/claude-sonnet-5`, **When** the capture result is read, **Then** `target.model` carries the value and `target.json` stores it beside `source`.
2. **Given** that run, **When** it is published, **Then** `loupe-meta` carries `model=anthropic/claude-sonnet-5` after `src=` when a source was given, or after `unattended=1` or `round=` when not, and the footer line has no model segment.
3. **Given** `--model` with `>`, `--`, an uppercase letter, a space, or more than 64 characters, **When** capture runs, **Then** it refuses with `input` before touching the clone, and the fix names the flag and the pattern.
4. **Given** a stored `target.json` whose `model` fails the pattern, **When** any command loads the run, **Then** it refuses as it does for a bad `source`.
5. **Given** a capture without `--model`, **When** it is published, **Then** the body is byte for byte what it was before this feature.
6. **Given** the example workflow with the `REVIEW_MODEL` variable unset, **When** the capture step runs, **Then** the step fails with a message naming the variable and nothing is captured.

---

### User Story 3 - Runs captured before this feature keep working (Priority: P3)

A human opens a run captured last week whose findings carry `severity: "P2"`. `loupe show`, `review` and `publish` all work, and the severity renders as `**Severity:** P2`. Editing that finding's title does not fail on the severity it did not touch.

**Why this priority**: Narrowing a field that existing runs hold as free text is the one way this feature can break something already on disk.

**Independent Test**: Write a draft with a free-text severity through the store, then load it, render it, edit its title and publish it against the fake GitHub.

**Acceptance Scenarios**:

1. **Given** a stored finding with `severity: "P2"`, **When** the run is loaded, shown, reviewed or published, **Then** nothing refuses and the meta line reads `**Severity:** P2`.
2. **Given** that finding, **When** `loupe edit` changes its title only, **Then** the edit succeeds and the severity is kept.
3. **Given** that finding, **When** `loupe edit` sets `severity: "P1"`, **Then** it refuses with `input` naming the four accepted words.
4. **Given** a stored draft with no `impact`, `verified` or `references` keys, **When** it is loaded, **Then** the fields are absent, and the finding renders as it did before this feature.

---

### Edge Cases

- A finding with `severity` alone, `verified` alone, or both without confidence: the meta block holds one line per part present and is omitted entirely when there is no location and no part.
- `references` given as an empty array on `add` is stored as absent, so `loupe show` and the digest treat it as no references. On `edit`, an empty array clears, like `null`.
- A reference that is valid but long is kept: the limit is bytes per entry and entries per finding, not line width.
- `impact` is subject to the same size limit as `body`, and the composed disclosure counts toward the inline comment limit, so a long body with a long impact can be refused with `limit` where it was not before. That refusal is unchanged in shape.
- A `model` that is a valid pattern but names nothing real is accepted. loupe MUST NOT check the value against any model list.
- The digest of a draft with no new fields changes because the encoding gains keys. No stored attempt is recomputed, so reconciliation is unaffected; only the pinned digest test moves.

## Requirements *(mandatory)*

### Functional Requirements

#### Finding fields

- **FR-001**: `severity` MUST be one of `critical`, `major`, `minor` or `trivial` when present on `add`, and on `edit` when the edit changes it. Any other value MUST refuse with `input`, naming the four words, and store nothing. An existing stored value outside the enum MUST load, render and publish, and MUST NOT make an edit that leaves it unchanged fail. Because nothing rechecks it at composition, it MUST render inside a code span, where before it was one too, so it never runs as Markdown.
- **FR-002**: `verified` MUST be one of `reproduced` or `plausible` when present. Any other value MUST refuse with `input` naming both words. It MUST NOT be computed, defaulted or inferred, as confidence is not.
- **FR-003**: `impact` MUST be Markdown checked against the same allowlist and size limit as `body`, and MUST be refused the same way when it fails.
- **FR-004**: `references` MUST be a list of at most six strings, each at most 200 bytes, each parsing as a URL with scheme `http` or `https` and a non-empty host, and each free of whitespace, control and format characters (Unicode `Zs`, `Cc` and `Cf`, so a bidi override cannot make a link display a host it does not point to), `<`, `>` and backticks, and carrying no userinfo, since `user@host` reads as the user part's host to a skimming reader. A failing entry MUST refuse with `input` naming its index and the rule. An empty list MUST be stored as absent. Composition MUST check the list again, as it rechecks `body`, so a draft another version wrote cannot unbalance the disclosure.
- **FR-005**: The `add` input MUST accept `impact`, `verified` and `references` as optional keys with flag equivalents `--impact`, `--verified` and a repeatable `--reference`. The `edit` input MUST accept the same, MUST accept `null` for each to clear it, and MUST accept `--clear-impact`, `--clear-verified` and `--clear-references`. Changing any of them is a publishable change: it increments the revision and removes the human's decision.
- **FR-006**: The three fields MUST enter the draft digest after `suggestedFix`, absent keys included as empty, so two drafts differing only in one of them have different digests. The digest encoding is append-only from this feature on: keys are added at the end and never removed or reordered.
- **FR-007**: `loupe show`, the review interface and its plain mode MUST display severity, verified, impact and references wherever they display confidence and the suggested fix today.

#### Rendering

- **FR-008**: On the meta line, severity MUST render as `**Severity:**` followed by the enum word, or by the one-line value in a code span when it is not an enum word, and verified as `**Verified:**` followed by its word, one part per line in the order confidence, severity, verified, each line separated by a backslash hard break.
- **FR-009**: `impact` MUST render inside the disclosure after the body and before `**Suggested fix**`, under a bold `**Impact**` line, its trailing newlines trimmed, as Markdown.
- **FR-010**: `references` MUST render inside the disclosure after the suggested fix, under a bold `**References**` line, as one `- <url>` autolink bullet per entry in input order.
- **FR-011**: An inline comment MUST carry the same meta-block parts, impact and references as the body's disclosure for that finding, without the location line: the comment sits on that line already.
- **FR-012**: The chips row, headings, summary lines, dividers, footer and reconciliation marker MUST be unchanged by this feature. A finding without the new fields and a run without a model MUST publish a body byte for byte identical to before.

#### Run provenance

- **FR-013**: `loupe capture` MUST accept `--model <id>`, optional, stored in the run's target as `model` and returned in the capture result as `target.model`. Its absence MUST leave the target file without the key.
- **FR-014**: A model MUST match `^[a-z0-9][a-z0-9._/:-]*$`, be at most 64 characters and contain no `--`. Capture MUST refuse a bad value with `input` before any Git or GitHub call, and loading a stored target MUST refuse one as it refuses a bad `source`.
- **FR-015**: `loupe-meta` MUST carry `model=<id>` when the target has a model, placed after `src=`, or where `src=` would go when there is no source. The footer MUST NOT change.
- **FR-016**: loupe MUST NOT require a model anywhere: not on capture, not on `publish --unattended`, not on any input. Requiring it would force a reviewer that does not know its model to guess or stop filing.

#### The example integration and the plugin

- **FR-017**: `.github/workflows/review.yml` MUST fail its capture step with a message naming `REVIEW_MODEL` when that variable is empty, and otherwise MUST pass `--source` naming the workflow and `--model` with the variable's value.
- **FR-018**: The `human-review` skill MUST document the three finding fields and the severity enum, and MUST tell the agent to pass `--model` with its own model identifier to capture when it knows it, and to omit the flag when it does not.

### Key Entities

- **Severity**: an ordered word, `critical` above `major` above `minor` above `trivial`. Reviewer-reported. Never maps onto a label and never decides section placement.
- **Verified**: whether the reviewer ran or observed the failure (`reproduced`) or reasoned to it (`plausible`). Orthogonal to confidence.
- **Impact**: Markdown describing what goes wrong and under what input. Part of the finding's content, so it is digested and its change clears a decision.
- **Reference**: one `http` or `https` URL the reviewer looked at. Kept verbatim, rendered as an autolink, never fetched.
- **Model**: the identifier of the model that produced a run's findings, as the caller names it. Per run, recorded at capture, provenance only.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A finding filed with every field renders all of them on the fake GitHub, in the documented positions, and `docs/comment-format.md`'s example block is that rendering.
- **SC-002**: Each rejected value for severity, verified, references and model is refused with `input` and zero writes, with a fix a reader can act on without loupe's source.
- **SC-003**: A run captured before this feature, with a free-text severity, loads, shows, edits by title and publishes unchanged.
- **SC-004**: A run without a model and findings without the new fields publish a body byte for byte identical to the goldens before this feature, footer included.
- **SC-005**: A capture with `--model` publishes a body whose `loupe-meta` carries the value and whose footer does not.
- **SC-006**: The example workflow's capture step fails without the model variable, and the next live run on a pull request the maintainer names carries `model=` in its marker. The live check is unverified until that run.
- **SC-007**: All automated repository checks pass after the final edit.

## Assumptions

- Four severity words are enough. A reviewer with a finer scale maps onto them; loupe does not offer a fifth.
- Two verified words are enough. Absent means the reviewer did not say, which is distinct from `plausible`.
- Six references and 200 bytes each cover any finding worth publishing. A reviewer with more puts the rest in the body.
- GitHub renders `- <https://…>` in a `<details>` body as an autolink. Recorded in `docs/github-facts.md` after the first live run; until then it is the CommonMark rule.
- The `/` and `:` in model identifiers such as `anthropic/claude-sonnet-5` are the only characters beyond the source pattern that real identifiers need.
- No push, pull request, release, live review or workflow dispatch is authorized by this specification.
