# Specification Quality Checklist: loupe stats rolls up the human gate across runs

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-28
**Feature**: [spec.md](../spec.md)

## Content Quality

- [ ] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [ ] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [ ] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [ ] No implementation details leak into specification

## Notes

- No markers remain. The owner decided both on 2026-10-28, as recorded under Clarifications: `precision` and `medianCaptureToPublish` use attended published runs only, with `runs.unattended` per group (FR-007, FR-007a), and repositories are positional arguments with no `--repo` flag (FR-012).
- Implementation details are named on purpose, and the unchecked items above say so. The command is a machine contract (Principle V), so its flags, result shape and refusal codes are the specification. FR-005 names `draft.GateCountsOf` and `internal/draft` because agreement with the published `loupe-meta` marker is the requirement, and only a shared derivation guarantees it. FR-004, FR-006, FR-007a and FR-009 name receipt fields because the board draft named fields that do not exist (`publishedAt`, `draftDigest`).
- SC-002 and SC-004 name `loupe-meta`, goldens and `mise run check`. In this repository those are the observable outcomes, not internals.
- The audience is the owner and agents working in the repository, not non-technical stakeholders.
