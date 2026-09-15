# Specification Quality Checklist: loupe handoff and agent plugins

**Purpose**: Validate specification completeness and quality before proceeding to planning

**Created**: 2026-09-15

**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
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

- loupe's commands, Herdr's CLI and each harness's install command are the product surface, not implementation choices, so the spec names them as specs 003 and 004 do. The package that holds Herdr, the command runner and the test fakes are left to the plan.
- FR-010 names `cmd/` and `internal/` because the containment boundary replaces a constitution-backed guard and must stay checkable.
- The three choices that would have been clarifications were settled by the owner on 2026-09-15: stack on 004, plugins for every harness with no `npx skills` route, and the skill name `loupe-handoff`. Later the same day the owner ruled that loupe ships no review skill or command, so the two skills folded into one workflow skill, first named `loupe` and then, because Claude Code and Codex showed it as `loupe:loupe`, `human-review`.
