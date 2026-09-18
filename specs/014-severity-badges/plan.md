# Implementation Plan: Severity at the top level

**Branch**: `014-severity-badges` | **Date**: 2026-09-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/014-severity-badges/spec.md`

## Summary

A new leaf package holds the four words as one ordered list and the comparison every surface sorts by. `internal/draft` validates against it instead of an inline switch, `internal/render` sorts by it and leads each rated summary line with the word, the review interface gains a severity column and orders its list, detail navigation and line-by-line mode by it, and `loupe show` follows. `docs/comment-format.md` is amended where it forbade severity in the summary line and where it stated the `Blocking` sort. An unrated finding renders byte for byte what it renders today, everywhere.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. No file loupe writes gains or loses a key, and stored order stays arrival order.

**Testing**: New unit tests first, per the repository's failing-test-then-implementation rule: the rank and comparison over a mixed set including unrated and legacy free text; a summary line per word in each context; an unrated finding rendering byte for byte as before; the list row at each of the three widths. Then `go test ./internal/render/ -update` and `go test ./internal/cli/ -update`, each followed by reading the diff. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.1 Boundaries — the comment format is a contract, so this specification is the amendment that permits the summary-line and sort changes. The reconciliation marker, the `loupe-meta` keys, the chips row, section placement and the meta block are out of scope and untouched.

**Scale/Scope**: One new leaf package of four declarations, one palette entry and the role that names it, one validation switch replaced, one sort key added in three places, one column added to one row layout, one ordered view threaded through the review interface, and three documents amended.

## Research

- **A leaf package, not a home in `draft`.** Decision: add `internal/severity` holding `Order`, `Rank` and `Compare`, on the model of `internal/findingid` and `internal/refusal`. Rationale: FR-005. The words live today as a literal switch in `internal/draft/mutate.go`, and `internal/render` deliberately does not import `internal/draft` — it carries its own `Finding` so composition depends on no storage type. Four packages now need one ordering, so the enum cannot live in any of them. Constitution VI wants a second concrete use before a new abstraction; there are four: validation in `draft`, sort and summary line in `render`, sort and color in `tui`, sort and meta cell in `cli`.
- **Unrated sorts last, not as `trivial`.** Decision: `Rank` returns a sentinel past the end of `Order` for an absent, empty or non-enum value, and unrated findings fall back to finding-id order among themselves. Rationale: FR-002 and FR-003. `docs/comment-format.md` already forbids computing, defaulting or inferring severity; mapping an absent value onto `trivial` for ordering would be inferring one, and would also reorder every review published before this change relative to itself. A legacy free-text value takes the same sentinel, because nothing in the codebase can say where `P2` ranks.
- **A word on the summary line, not a dot.** Decision: the severity word leads the bold prefix, separated by ` · `, and no dot or glyph is introduced. Rationale: FR-007 and FR-016. The contract reserves dots for labels and already records that a label dot under the `⛔` heading reads as a severity; a second dot vocabulary on the same line would make that worse, not better. External badge images are banned by the contract as a network dependency through GitHub's camo proxy. A word costs nothing and survives a plain-text reader.
- **The enum word is already known inert.** Decision: only an enum word may lead a summary line; a legacy free-text severity is omitted there and keeps its meta-line code span. Rationale: the Edge Case and FR-014. A summary line interpolates its prefix outside a code span, and `severity` is the one field the renderer interpolates as is rather than escaping — safe for the four known words, and not safe for arbitrary stored text. Alternative rejected: escaping the free text into the summary line, which would put a value nothing else in the review treats as a rank into the one line a reader is guaranteed to see.
- **Severity outranks the label group inside `Blocking`.** Decision: `Blocking` sorts severity, then label group, then id; a label section and `Other` sort severity, then id. Rationale: FR-009 and FR-010, and the spec's "What this deliberately costs". The section that exists to be read first was ordered by a taxonomy that is not urgency, so a `critical` question sat below a `minor` issue. The label group survives as the tie-break, so labels still cluster among equal severities, and the contract's stated property is amended rather than quietly broken.
- **One place per finding.** Decision: the meta block drops an enum severity and keeps only a pre-enum free text. Rationale: FR-014. `section()` and the inline path both emit a summary line immediately above the meta block, so there is no context where dropping it loses the word. The contract's location rule is the same decision already made once, which is why this reads as applying a rule rather than adding one. Alternative rejected: keeping the labeled `**Severity:**` line so the bare word on the summary line has a definition somewhere — it only reaches a reader who has the fold open, and that reader has just read the word two lines up.
- **The chips row is left alone.** Decision: no severity chip and no severity count. Rationale: FR-012. The row's stated property is that every chip maps to a heading below it that a reader can scroll to. Severity decides no heading, so a severity chip would be the one chip that indexes nothing, and the row would carry two taxonomies at once.
- **One new role, and no new glyph.** Decision: `critical` takes `style.Bad`, `major` a new `style.Caution`, `minor` `style.Warn`, `trivial` `style.Dim`, and no tier gains a severity glyph. Rationale: FR-018 and FR-021. The obvious ramp reuses `Bad`, `Warn`, `Note` and `Dim`, but `Warn` and `Note` are both `colorYellow`, so `major` and `minor` come out the same color — confirmed by eye in the demo before this was changed. `Caution` adds Catppuccin Peach between yellow and red, which is the one step the palette was missing, and it is named for what it means so the next ramp can reuse it. No call site names a color. `style.GlyphSet` documents that a field empty in a tier has no icon there and "the word beside them carries the state alone"; inventing a severity glyph would mean inventing one for three tiers, and the word plus its color already degrades correctly to a plain word in a column under `NO_COLOR`.
- **Severity gives way after the label.** Decision: as the window narrows the location shortens to a filename, then the label column drops, then the severity column. Rationale: FR-020. The two are the droppable columns and severity is now the higher-value one, so it goes last. The title floor is unchanged and still wins over both.
- **One ordered view, not a sorted draft.** Decision: the review interface derives an ordered slice of the loaded draft and indexes that; `internal/draft` keeps arrival order and is never sorted in place. Rationale: FR-004. The draft is written back on every decision, and the digest is computed over an id-sorted copy, so sorting the stored slice would either move the digest or be undone on the next write. The list cursor, the detail view's previous and next, its "N of M" and line-by-line mode all read the same ordered slice, which is what keeps them agreeing (FR-022, FR-024).
- **Line-by-line mode is a surface.** Decision: `--plain` follows the same order, though the specification's file list was written around the full-screen list. Rationale: FR-001 and FR-024, confirmed with the owner before implementation. It is the same review interface for a terminal that cannot take a full-screen program; two review surfaces that disagree about which finding is next give the human two answers to the same question.
- **The severity cell in `loupe show` needs its own style.** Decision: `writeFinding` styles each meta cell and stops dimming the joined line. Rationale: FR-025. The meta line is composed, wrapped and then dimmed whole, so a colored cell inside it would emit its own reset and strip the dim from everything after it. Styling per cell, with dim as every other cell's kind, keeps the line looking as it does today and lets one cell carry a color.
- **What this change cannot check.** Decision: how GitHub renders `<b>major · issue (blocking):</b>` inside a `<summary>`, and whether the words in a column make reviewers pick alike, go on `specs/001-loupe-v1/validation.md`'s Unverified list. Rationale: FR-027 and Principle VII. The first needs a review on a real pull request; the second needs reviews landing over time, beside the entry `specs/012-severity-meaning` already put there.

## Constitution Check

- **I. A tool for agents.** PASS. No command, flag, input shape or refusal code changes. `--json` envelopes are untouched: this moves what human-facing surfaces show and what order they show it in.
- **II. Nothing posts unread under a human's name.** PASS. Gates, readiness, per-finding decisions and the confirmation are untouched. Reordering the list changes which finding the human meets first, never whether each one is met.
- **III. Local files, no service.** PASS. Nothing written to disk changes, and no new file is read or written.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. The same four values are accepted and refused, by a list instead of a switch. `docs/comment-format.md` is amended from this specification rather than redesigned, and every changed rule is named in FR-015.
- **VI. Simplicity over ceremony.** PASS. No dependency. One new package of three declarations, justified by four concrete uses rather than a possible one, and it removes a duplicated enum rather than adding one.
- **VII. Verified means ran.** PASS. New tests are written failing first; the goldens are regenerated deliberately and the diffs read; `mise run check` runs before every commit. The two claims this repository cannot check are named on the Unverified list rather than made.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/014-severity-badges/
├── spec.md
├── plan.md
└── tasks.md

internal/severity/severity.go     # new leaf: Order, Rank, Rated, Compare
internal/draft/mutate.go          # validateSeverity checks severity.Order
internal/render/body.go           # summaryLine prefix, blocking sort, section sort
internal/tui/list.go              # severity column, ordered view, column heads
internal/tui/detail.go            # navigation and N of M follow the ordered view
internal/tui/plain.go             # line-by-line mode follows the ordered view
internal/tui/app.go               # the ordered view; chips() colors severity
internal/draft/derive.go          # Ordered, the one presentation order for a draft
internal/style/style.go           # the Caution role and the severity ramp
internal/cli/show.go              # ordered findings; the severity cell carries its kind
docs/comment-format.md            # summary-line table and rule, Blocking sort, the example
specs/001-loupe-v1/validation.md  # two Unverified rows
```

**Structure Decision**: No `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 013. The research is above, and the only data-model statement — that stored order does not change — is a requirement rather than a change.

## Complexity Tracking

No Constitution Check violations. The one departure from a contract is the amendment this specification is, and the property it costs — label grouping within `Blocking` — is stated in the specification and rewritten in `docs/comment-format.md` rather than left to drift.
