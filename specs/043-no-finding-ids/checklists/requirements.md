# Specification Quality Checklist: Published findings never cite finding ids

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-28
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [ ] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- "Written for non-technical stakeholders" is left unchecked on purpose. The users are agents and the maintainer, and the spec amends two contracts (`docs/comment-format.md` and the `markdown` row of `specs/001-loupe-v1/contracts/cli.md`), so it names commands, fields, `details` keys and the refusal code. In a CLI tool those are what the user sees, not implementation.
- The owner's decisions of 2026-10-28 are recorded under Clarifications: code spans are not exempt, run references are out of scope, linking prior-round ids is deferred, current-round ids are refused too, the summary is checked on every run, the sticky round note is checked, and the allowlist refusal names the real field.
- The round note is written only by `loupe publish --unattended --sticky --note` and never stored, so FR-019 checks it at publish, where the allowlist already checks it, before any request is sent. FR-009 names it as the one exception to "publish does not re-check".
- FR-015 adds the rule to `loupe publish --help`, so the `publish-help` goldens change deliberately. SC-005 says so.
- One choice is still made in Assumptions without an owner decision: only lowercase ids match.
- The id-shape rule and the exemptions are stated precisely enough to test (FR-004, FR-005). The plan decides where the check lives, beside `internal/markdown` or in `internal/draft`.
