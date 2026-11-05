# Stage 7: Critic Review

- Target: `specs/049-apply-mode/spec.md`, FeatureRange lines 1 to end, as of 2026-11-03
- Standard: `feature-spec-standard.md` for design and structural (EGM 0.1.0)
- Dispatch: direct, one round, three reviewers in parallel on Claude Opus 5.5, fresh contexts
- Status: design critic completed, structural completed, style completed

Each line below is one raw finding with its id (`<reviewer>-<n>`), severity, lines in the spec and a one-line statement. The full reviewer text is in the session record. `consolidated_review_report.md` maps each one to a decision.

## Design critic: 1 Blocker, 11 Major, 8 Nit

- `design-B1` 🔴 No Alternatives Considered section. Rejections lean on "owner, F<n>" with no link. Unweighed: separate sift plus shared contract, pseudo-PR target, `loupe local` namespace, reuse `suggestedFix`, one field versus `mode` and `kind`.
- `design-M1` 🟡 No Risks section. Untracked secrets copied into Git objects and the data root with no cleanup, unbounded storage, `--changed` hashing the whole worktree, and agent fidelity to the approved proposal.
- `design-M2` 🟡 Run selection without `--run` falls through to the GitHub API, breaking FR-002, and can pick a PR run on the same branch.
- `design-M3` 🟡 The capture update check (spec 048) calls the GitHub API, against FR-002, SC-004 and the line 46 claim.
- `design-M4` 🟡 `mode` and `kind` on every target change PR targets, bump them to schema 3, and break older `Walk`. SC-003 cannot hold, since `help`, `stats`, `handoff-help` and `list` goldens change.
- `design-M5` 🟡 SC-006 key counts contradict `x` with input, `t` with input, and the return confirmation.
- `design-M6` 🟡 FR-017's four hooks miss most mode differences (add and edit rules, handoff, cards, keys, wait, stats and list, refs).
- `design-M7` 🟡 The accept note and the exclude reason are new draft fields that schema 5 does not name. "Stored with the decision" is ambiguous.
- `design-M8` 🟡 The apply location check is undefined. Deleted lines cannot be anchored once `side` is refused.
- `design-M9` 🟡 Repository key not normalized. The reference has two shapes, unsafe directory names, and a weak key outside Git.
- `design-M10` 🟡 Contract changes missing for `list`, `stats`, PR-only commands on an apply run, and PR-only input fields (`suggestedFix`, `blocking`).
- `design-M11` 🟡 Return gates undefined. `empty` is wrong when every finding is excluded.
- `design-N1` 🟢 `--tree` has no story, and its hash and path meaning is undefined.
- `design-N2` 🟢 Separate `mode` and `kind` is speculative.
- `design-N3` 🟢 `t` and the `x` input need the typeahead guard (`detail.go:508`).
- `design-N4` 🟢 FR-004 is checked only at handoff.
- `design-N5` 🟢 It is unclear which findings the list marks as questions.
- `design-N6` 🟢 sift's applier rules (the thread counts, the suggestion wins) are not stated.
- `design-N7` 🟢 `--applier` and `--title` have no validation rule.
- `design-N8` 🟢 The constitution preamble scope is not addressed.

## Structural: 1 Blocker, 6 Major, 5 Nit

- `structural-B1` 🔴 No Alternatives Considered section. Rejections in the Clarifications have no reason (F2, F3, F8, F10, F11).
- `structural-M2` 🟡 Owner attributions are not linked (board item, sift round).
- `structural-M3` 🟡 No Risks & Mitigations section: security, scale and failure modes.
- `structural-M4` 🟡 No Problem, Goals or architecture note.
- `structural-M5` 🟡 Raw `<agent>` and `<applier>` at lines 9, 75 and 185 vanish when rendered.
- `structural-M6` 🟡 The reference is written two ways (lines 25, 172 and 218).
- `structural-M7` 🟡 The parity MUST at line 201 has no FR id.
- `structural-M8` 🟡 FR-001a, FR-001b, FR-007b, FR-010, FR-011 and FR-006a have no scenario or SC.
- `structural-N9` 🟢 The Session intro says all 14 were approved, but the answers come from three sources.
- `structural-N10` 🟢 Line 58 omits the FR-004a field that line 241 lists.
- `structural-N11` 🟢 "one for one" is inaccurate (8 commands against 7).
- `structural-N12` 🟢 The Out of scope rationale is wrong for three items.
- `structural-N13` 🟢 FR-000 numbering. The section order differs from 047 and 048.

## Style: 0 Blocker, 9 Major, 10 Nit

- `style-M1` 🟡 Line 56 "behaves exactly as it does today" is contradicted by FR-000, the target schema bump, FR-003 and FR-001a.
- `style-M2` 🟡 Constitution check claims overreach: line 50 against SC-004, line 46 against the update check, line 45 broader than FR-002.
- `style-M3` 🟡 "the return key" reads as Enter (lines 76, 161, 204, 233, 239).
- `style-M4` 🟡 Index MUST NOTs live only in the Constitution check (line 47), not in FR-001c.
- `style-M5` 🟡 Reference format conflict (FR-001a against Key Entities).
- `style-M6` 🟡 Untestable MUSTs: "no heavier than sift", "without bending", "small interface", "spread through".
- `style-M7` 🟡 "one for one" is wrong.
- `style-M8` 🟡 SC-006 against the confirmation step.
- `style-M9` 🟡 The Out of scope rationale is wrong for three items.
- `style-N1` 🟢 Missing or misused RFC 2119 keywords (lines 182, 191, 205, 210, 211, 241).
- `style-N2` 🟢 Passive voice (lines 11, 19, 28, 33, 38, 42, 149, 155, 191, 246).
- `style-N3` 🟢 Phrasal verbs and idioms (lines 36, 66, 136, 151, 177, 211, 239).
- `style-N4` 🟢 Term drift: reject/exclude, card/finding, good color, proposal callout, finding-id rule, type40, engineer, "Stats".
- `style-N5` 🟢 The hedge at line 238 contradicts the next sentence.
- `style-N6` 🟢 Ambiguous references (lines 45, 78, 153, 157, 195, 205).
- `style-N7` 🟢 The lists of new target fields differ (lines 58, 241).
- `style-N8` 🟢 Sentences start with lowercase placeholders (lines 38, 155, 172).
- `style-N9` 🟢 Several ideas per sentence, and nominalizations (lines 9, 13, 19, 27, 33, 45, 161).
- `style-N10` 🟢 SC-007 should name `mise run check`.
