# Feature Specification: The model in the review footer

**Feature Branch**: `023-model-footer`

**Created**: 2026-09-24

**Status**: Draft

**Input**: Owner, 2026-09-24: a reader of an AI review cannot see which model wrote it. The footer MUST show the model whenever capture recorded one with `loupe capture --model`, raw, in a code span, after `via` and before ` · unattended`, with no opt-in flag and no word such as `by`. This gates the Traackr shared review workflow's switch to loupe, which passes `--model` and expects the id after the sha and the source.

## Relationship to earlier specifications

This specification amends the Footer section of `docs/comment-format.md`, which is a contract (constitution, Boundaries). With it, it amends `specs/008-finding-fields/spec.md`, which put the model in `loupe-meta` only, and `specs/009-review-footer`, whose footer forms gain one optional segment.

- Spec 008 kept the model out of the footer because it read the model as provenance for tooling and the reader already had the reviewer's name from `via`. For an AI review that reason is weak. The model is part of how a reader decides how far to trust the findings, and `via` names a pipeline, not a model.
- Spec 009's rule that ` · unattended` comes last holds. The model segment sits before it.
- Spec 009's "Nothing else" rule gains one exception: the model capture was given. A PR reader can act on it, because it tells them what produced the findings.
- `model=` in `loupe-meta` does not change. Neither marker changes, and the digest does not change, because it covers the publishable draft and not the rendered body.
- `specs/001-loupe-v1/contracts/cli.md` says the footer does not carry `--model`. That sentence changes to match. No flag, refusal, exit code or result envelope changes.
- Constitution 2.0.2 is unchanged. Principle II holds: the footer is composed by loupe on both paths, and the model id is not prose anyone writes.

## Clarifications

### Session 2026-09-24

- Q: Does the model segment get a word or label (`by`, `model`)? → A: No (owner, 2026-09-24). On the attended path the human is the author, so `by` would claim authorship the model does not have, and a model id names itself.
- Q: Does loupe turn ids into display labels such as `Claude Sonnet 4.5`? → A: No. That needs rules that guess at id shapes. The id renders exactly as recorded.
- Q: Is there a flag to opt in or out? → A: No. The segment appears whenever capture recorded a model. A caller who does not want it omits `--model`, which also drops `model=` from `loupe-meta`.
- Q: Does the segment need escaping beyond the code span? → A: No. Capture refuses any model outside `^[a-z0-9][a-z0-9._/:-]*$`, and every command refuses a `target.json` that carries one, so a stored model holds no backtick, whitespace or newline. It still goes through the same code-span and one-line helpers as every generated footer field, per the comment-format rule that applies to every generated code span.
- Q: Does a run captured before this change need migrating? → A: No. The model has been stored in `target.json` since spec 008. A run captured with `--model` before this ships shows the segment when it is published after, and that is correct: the footer describes the review as published.
- Q: Does the `human-review` skill's advice on `--model` change? → A: Its wording changes to say the model shows in the published footer, since an agent choosing whether to pass `--model` should know a reader will see it. What it tells the agent to pass does not change.
- Q: Does the publish confirmation preview need its own change? → A: No. The preview and the review request both show the one body the publication envelope renders (`internal/publish`), so a footer change reaches both.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reader sees which model wrote an AI review (Priority: P1)

A pipeline captures a pull request with `--source claude-ci@1.2.0 --model anthropic/claude-sonnet-5` and publishes unattended. A co-worker opens the pull request and reads the footer: ``reviewed `a1b2c3d` · via `claude-ci 1.2.0` · `anthropic/claude-sonnet-5` · unattended``. They know the commit, the pipeline, the model and that nobody read it first.

**Why this priority**: It is the feature, and it gates the Traackr shared review workflow's switch to loupe.

**Independent Test**: Render a published body for a run captured with a source and a model, published unattended, and assert the exact footer line. The attended form with a source is also a body golden.

**Acceptance Scenarios**:

1. **Given** a run captured with `--source claude-ci@1.2.0 --model anthropic/claude-sonnet-5`, **When** it is published unattended, **Then** the footer reads ``reviewed `a1b2c3d` · via `claude-ci 1.2.0` · `anthropic/claude-sonnet-5` · unattended``.
2. **Given** the same run published attended, **When** the review is read, **Then** the footer reads ``reviewed `a1b2c3d` · via `claude-ci 1.2.0` · `anthropic/claude-sonnet-5` ``.
3. **Given** either review, **When** its raw Markdown is read, **Then** `loupe-meta` carries `model=anthropic/claude-sonnet-5` in the same position and form as before this change.

---

### User Story 2 - A model without a source (Priority: P2)

A human captures with `--model anthropic/claude-sonnet-5` and no `--source`, then publishes attended. The footer reads ``reviewed `a1b2c3d` · `anthropic/claude-sonnet-5` ``. No word stands in for the missing `via`.

**Why this priority**: The draft settles it, but in practice a pipeline that passes `--model` also passes `--source`, so this form is rarer.

**Independent Test**: Render a body for a run with a model and no source, attended and unattended, and compare against goldens.

**Acceptance Scenarios**:

1. **Given** a run captured with `--model anthropic/claude-sonnet-5` and no source, **When** it is published attended, **Then** the footer reads ``reviewed `a1b2c3d` · `anthropic/claude-sonnet-5` ``.
2. **Given** the same run published unattended, **When** the review is read, **Then** the footer reads ``reviewed `a1b2c3d` · `anthropic/claude-sonnet-5` · unattended``.

---

### User Story 3 - A run with no model is unchanged (Priority: P1)

A reviewer captures without `--model`. The published review is the same, byte for byte, as the one loupe publishes today.

**Why this priority**: The footer is a contract. Every review that does not opt in to a model by passing it MUST stay as it is.

**Independent Test**: The existing footer and publish goldens for runs without a model pass unchanged.

**Acceptance Scenarios**:

1. **Given** a run captured with no `--model`, in each of the four existing footer forms, **When** it is published, **Then** the body is byte-identical to what the same draft produced before this change.

### Edge Cases

- A model id at the 64-character limit renders in full. The footer MUST NOT truncate it.
- A model id containing `/`, `:` or `.` renders as recorded, because those are inside a code span.
- A `target.json` carrying a model that fails validation is refused before rendering, as today. This feature adds no new path to render an unvalidated model.
- A model id that looks like a source, such as `claude-ci`, still renders as a model segment. loupe does not compare the two or drop one.
- A retry with `--retry-unknown` renders the model from the same `target.json` as the first attempt, so the footer is the same on both.
- A run captured with `--model` before this change and published after it shows the segment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When capture recorded a model, the published footer MUST carry the segment `` · `MODEL` ``, where `MODEL` is the recorded id, exactly as stored, in a generated code span subject to the backtick rule in `docs/comment-format.md`.
- **FR-002**: The model segment MUST follow the ` · via ` segment when there is one, and otherwise the SHA. ` · unattended` MUST remain last.
- **FR-003**: The segment MUST carry no label or word (`by`, `model`, `using`), with or without a source.
- **FR-004**: loupe MUST NOT transform the id: no display names, no case change, no trimming of a provider prefix, no truncation.
- **FR-005**: When capture recorded no model, the footer and the whole body MUST be byte-identical to what the same draft renders before this change.
- **FR-006**: `loupe-meta`, the reconciliation marker and the digest MUST NOT change. `model=` keeps its position and form.
- **FR-007**: The segment MUST render on both publication paths, attended and unattended, and in every place loupe composes the published body, including the body `loupe publish` shows at confirmation, which is the same rendered body the review request sends.
- **FR-008**: `docs/comment-format.md` MUST list the new footer forms, state the placement and no-label rules, drop "never appears in the footer" from the `model=` rule, and name the model as the one exception to the "Nothing else" rule, citing this specification.
- **FR-009**: The `--model` flag's help, `specs/001-loupe-v1/contracts/cli.md` and the `human-review` skill MUST say the model shows in the published footer as well as in `loupe-meta`.
- **FR-010**: Render tests and goldens MUST cover: source, model and unattended together; model without source, attended and unattended; and model with source, attended. The existing goldens without a model MUST pass unchanged.

### Key Entities

- **Captured target**: `target.json`, which already records the model as `model` when capture was given one. This feature reads it and adds no field.
- **Footer**: the last visible line of the review body. Its segments, in order: the SHA, the source, the model, unattended.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every published review whose run recorded a model shows that model's id in its footer, verbatim, in 100% of the footer forms covered by FR-010.
- **SC-002**: Every existing golden for a run without a model passes unchanged, with zero byte differences.
- **SC-003**: `loupe-meta` in every golden is byte-identical before and after this change.
- **SC-004**: A reader who has never used loupe can name the model that produced a review from its rendered footer alone, without opening raw Markdown.

## Assumptions

- `loupe-workflows` passes `--model` on every capture, so every unattended review, loupe's own included, gains the segment when this ships. That is intended.
- GitHub renders a code span in a review body the same way it renders the SHA and source spans that the footer already carries (`docs/github-facts.md`). No new GitHub behavior is relied on.
- The README's picture of a published review (`docs/assets/review.png`) shows the footer and goes stale. Its rerun with `mise run review-screenshot` is the owner's to run, since it posts a GitHub review.
