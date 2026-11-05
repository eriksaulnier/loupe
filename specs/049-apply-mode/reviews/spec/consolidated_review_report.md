# Consolidated spec review, spec 049 apply mode

- Stages run: 5 (zoom-out), 6 (comprehension) and 7 (design, structural, style). Stage 8 is held until the decisions below land in the spec. Stage 9 does not apply, because the spec has no implementation plan.
- Reports: `zoom_out_report.md`, `comprehension_report.md`, `critic_report.md` in this directory.
- First sift round: `spec-049-apply-mode-egm-spec-review-20261103-173602`, 36 cards. The owner approved 30 without reading them and asked for a shorter round, so this report does not count it as a decision.
- Second sift round: 9 cards. Cards 1 to 8 are owner calls. Card 9 approves D9 to D36 below in one decision.

## Decisions

The source ids refer to the reports above.

| Id | Decision | Sources | Round 2 |
| :--- | :--- | :--- | :--- |
| D1 | State why apply mode belongs in loupe and amend the constitution preamble | zoom-out M1, design N8, structural M4 | card 1 |
| D2 | Store only the target kind and derive the mode from it | zoom-out M2, design N2, comprehension M4 | card 2 |
| D3 | One global `local#<n>` sequence | zoom-out M5, design M9, structural M6, style M5, comprehension N9 | card 3 |
| D4 | Drop the FR-010 file-change flag and hashes | zoom-out M7 | card 4 |
| D5 | Drop the FR-004 handoff gate; no proposal means a question | zoom-out N1, design N4, design N5 | card 5 |
| D6 | Move local-run stats out of scope | zoom-out M9, design M10, structural M8 | card 6 |
| D7 | Drop `capture --tree` from the contract | zoom-out M4, design N1, structural M8 | card 7 |
| D8 | Keep `x` with a reason and restate SC-006 | design M5, style M8, zoom-out M3, comprehension N14 | card 8 |
| D9 | Add an Alternatives considered section | design B1, structural B1, structural M2 | card 9 |
| D10 | PR target.json stays at schema 2 with no new fields | design M4 | card 9 |
| D11 | Rewrite FR-017 to list the real mode surface | zoom-out M3, design M6, style M6 | card 9 |
| D12 | State `--changed` safety as outcomes in FR-001c | zoom-out M4, style M4, comprehension M5 | card 9 |
| D13 | `--files` accepts paths outside the repository | zoom-out M6 | card 9 |
| D14 | Never select a local run implicitly | design M2 | card 9 |
| D15 | Local capture makes no network call; amend spec 048 FR-001 | design M3, style M2 | card 9 |
| D16 | `return.json` and the seal, with a new code `returned` | zoom-out M8, comprehension N10 | card 9 |
| D17 | Return gates: decided, no open notes, at least one finding | design M11 | card 9 |
| D18 | Plain accept on a question means go ahead as the body says | comprehension M2 | card 9 |
| D19 | FR-012 names withdraw through `edit --exclude` | comprehension M3 | card 9 |
| D20 | Roles table: reviewer, applier, human | comprehension M1, zoom-out N2 | card 9 |
| D21 | Today's location rule on apply runs; deleted lines anchor nearby | design M8 | card 9 |
| D22 | Refuse `suggestedFix` and `blocking` on apply runs | design M10 | card 9 |
| D23 | PR-only commands refuse with a new code `wrong-mode` | design M10 | card 9 |
| D24 | `list` shape for local runs | zoom-out M9, design M10 | card 9 |
| D25 | Risks section; skip files over 1 MiB as `too-large` | design M1, structural M3 | card 9 |
| D26 | Accept note and exclude reason live in `decisions[].note`; schema 5 names both | design M7, zoom-out N3 | card 9 |
| D27 | `t` and the `x` input join the settle guard | design N3 | card 9 |
| D28 | Validation rules for `--applier` and `--title` | design N7 | card 9 |
| D29 | Rename `--local` to `--no-files` | zoom-out N4 | card 9 |
| D30 | Plugin test pins the rules both skills share | zoom-out N5 | card 9 |
| D31 | `human-decide` states that the thread counts and the suggestion wins | design N6 | card 9 |
| D32 | Number the parity rule and make it testable | structural M7, style M6 | card 9 |
| D33 | Replace "a PR run behaves exactly as today"; name the goldens that change | style M1, design M4 | card 9 |
| D34 | Terms section | comprehension M7, M8, N10, N12 | card 9 |
| D35 | Acceptance scenarios for FR-001a, FR-007b, FR-006a | structural M8 | card 9 |
| D36 | Wording pass | structural M5, N9 to N13, style M3, M7, M9, N1 to N10, comprehension N11, N13, N14 | card 9 |

The full text of each proposal is the matching card in the first sift round.

## Outcome

- Second round `spec-049-the-9-calls-that-matter-20261103-175348`: all 9 cards approved on 2026-11-03. Card 3 took the prefix form after a discussion. D1 to D36 are in the spec, the constitution is at 3.2.0, and spec 048 FR-001 is amended.
- Stage 8 ran on the revised spec (`intermediate_zoom_out_report.md`): no Blockers, 3 Majors, 7 Nits.
- Third round `spec-049-stage-8-follow-ups-3-cards-20261103-180313`: all 3 cards approved. The prefix is display-only, `return.json` holds only the time and the sealed draft version, and FR-030 and the seven Nits are applied.
