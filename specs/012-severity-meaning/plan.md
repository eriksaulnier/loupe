# Implementation Plan: What loupe's four severity words mean

**Branch**: `012-severity-meaning` | **Date**: 2026-09-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/012-severity-meaning/spec.md`

## Summary

`docs/comment-format.md` gains a meaning per severity word, phrased as what goes wrong if the finding ships, plus one line saying that severity is how bad the consequence is and `blocking` is whether merge waits. The workflow skill and `loupe add`'s long help carry the same four meanings, because that is where a reviewer picks the word. Nothing else moves: no accepted value, no glyph, no rendered byte, no golden.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. No file loupe writes gains or loses a key.

**Testing**: The assertion is that nothing moved. `go test ./internal/render/ ./internal/cli/` passes without `-update`, so the published body and the CLI goldens are byte for byte what they were. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.1 Boundaries — the comment format is a contract, so this specification is the amendment. The reconciliation marker, the `loupe-meta` keys and the body composition rules are out of scope and untouched.

**Scale/Scope**: Documentation only. One table and two lines in `docs/comment-format.md`, one bullet in the workflow skill, one clause in a Go string literal, one pointer in spec 001 FR-007, and one row on the Unverified list.

## Research

- **Consequence, not disposition.** Decision: each word says what goes wrong if the finding ships — `critical` data loss, a security hole, an outage; `major` a real defect on a normal path; `minor` an edge case, or a cost paid later; `trivial` cosmetic. Rationale: FR-003. The rejected levels named what to do ("resolve / ticket / defer"), which is `blocking`'s job and the human's call, so a severity word phrased that way becomes a second name for a field that already exists. Phrasing by consequence keeps the axes apart without needing a rule, because no word names an action.
- **Ordered rows, highest wins.** Decision: the contract says the rows are ordered and a finding takes the highest row it satisfies, and the skill and the long help carry the rule in a clause. Rationale: FR-011. The rows grade different kinds of consequence — `critical` by kind, `major` by path, `minor` by scope — so overlap is normal rather than a wording slip: an outage on a normal path fits `critical` and `major`, and data loss confined to an edge case fits `critical` and `minor`. Alternative rejected: rewriting the four onto one axis, which removes the overlap by dropping "on a normal path" and "an edge case", the two phrases that make `major` and `minor` distinguishable in the first place. A ladder with a stated precedence keeps both.
- **The word travels alone.** Decision: the meanings are not published to the author. Rationale: FR-008. The author sees `**Severity:** minor` inside a collapsed disclosure, with no legend and — after spec 009 — possibly not even the tool's name. A consequence reads correctly bare; an instruction needs the legend. Alternative rejected: a `<details>` legend on every review, which would define four ordinary English words at a per-pull-request cost forever and would turn this into a rendering change with goldens to move.
- **Three places, because three readers.** Decision: the contract, the workflow skill and `loupe add`'s long help each carry the meanings. Rationale: FR-006 and FR-007. The contract is what a human reviewer and a format reader are pointed at; the skill is what the filing agent has loaded; `--help` is what Principle I promises is sufficient on its own. Leaving any one of them a bare list leaves one reader guessing.
- **The short strings stay short.** Decision: `internal/cli/add.go:80`, `internal/cli/edit.go:31`, `internal/cli/edit.go:92`, `internal/draft/mutate.go:101`, `internal/render/body.go:278` and `internal/draft/model.go:61` are untouched. Rationale: the first three are one-line flag descriptions or pointers at the values, too short to carry meanings, and `add`'s long help is where the input shape is documented. The refusal in `mutate.go` states what is allowed, which is a different job from defining a taxonomy. The last two are rendering and storage, which FR-008 forbids changing.
- **The guard stays.** Decision: `plugin/skills/human-review/SKILL.md`'s "Report `confidence`, `severity` and `verified` only when you mean them" is left exactly as it is, and the new bullet sits immediately before it. Rationale: FR-005 and the Edge Case. Defining the words is the reason an agent might start supplying one reflexively; the sentence that says not to belongs next to them.
- **The claim this change cannot make.** Decision: whether reviewers actually pick more consistently goes on `specs/001-loupe-v1/validation.md`'s Unverified list. Rationale: FR-010 and Principle VII. It needs reviews landing on real pull requests over time, which no test in this repository can stand in for.

## Constitution Check

- **I. A tool for agents.** PASS, and this is the principle driving FR-007. No command, flag, input or refusal changes; `--json` envelopes are untouched. `loupe add --help` gains the meanings it was missing.
- **II. Nothing posts unread under a human's name.** PASS. Gates, readiness, decisions and the confirmation are untouched. `blocking` stays the human's to set in review.
- **III. Local files, no service.** PASS. Nothing written to disk changes.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. The machine contract is unchanged: the same four values are accepted, refused and rendered the same way. `docs/comment-format.md` is amended from this specification, not redesigned.
- **VI. Simplicity over ceremony.** PASS. No dependency, no abstraction, no new type, no new field. Four rows and a sentence.
- **VII. Verified means ran.** PASS. The verification is that the goldens do not move, run without `-update`; `mise run check` closes the work. The consistency claim is named unverified rather than made.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/012-severity-meaning/
├── spec.md
├── plan.md
└── tasks.md

docs/comment-format.md                     # the severity row, the meaning table, the blocking line
plugin/skills/human-review/SKILL.md        # severity lifted into its own bullet with the meanings
internal/cli/add.go                        # the severity clause in the long help
specs/001-loupe-v1/spec.md                 # FR-007 pointer
specs/001-loupe-v1/validation.md           # the Unverified row
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 010. The research is above; there is no data model change to record.

## Complexity Tracking

No Constitution Check violations, and nothing departs from a contract beyond the amendment this specification is. No Complexity Tracking entry is needed.
