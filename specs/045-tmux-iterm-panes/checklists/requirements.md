# Specification Quality Checklist: tmux and iTerm2 pane hosts

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

- The one marker, in FR-005, was settled by the owner on 2026-10-28 (option B): a tmux probe that succeeds with zero attached clients skips tmux, and detection goes on to Orca, then iTerm2. A failed probe still refuses. The spec's Clarifications, FR-001, FR-005, FR-012, User Story 1 scenario 8, User Story 3 scenario 4, the edge case and SC-003 carry it.
- The implementation-detail items are unchecked on purpose. Like `specs/019-pane-hosts`, this spec names the host commands, flags and environment variables, because they are the contract with each host and what the tests pin. A reader who is not a terminal user will not follow FR-004 to FR-011.
- SC-003 to SC-005 name tests, `mise run check` and the host guard. They are measurable, but not technology-agnostic.
- Every iTerm2 behavior rests on iTerm2's scripting page and sift, not on a check that can run here. FR-018 puts all of it on the Unverified list. User Story 2 can ship after User Story 1.
- Claims from documentation rather than tests: tmux(1) for split, focus, format and lifetime behavior, iTerm2's scripting page for the session commands. Claims from sift and 019's research only: the `TMUX` socket prefix and the `ITERM_SESSION_ID` format. The plan MUST record a live sample of each.
