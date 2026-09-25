# Implementation Plan: Quoted earlier rounds

**Branch**: `030-quoted-rounds` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/030-quoted-rounds/spec.md`

## Summary

The demote step in `render.ReadSticky` writes the round it takes off the top as one blockquote: its prose and sections, loupe's section headings at `###` as on top, then its footer, every line prefixed with `> ` or `>` when blank, with no divider inside, and its reconciliation marker after the quote. Read-back learns the quoted layout beside every earlier one, and carries a quoted round whose lines lost a marker as it is (amended after the CI review of #54). `markdown.OpenDetails` and `markdown.MapSummaryLines` learn to see through quote markers, so the terminal confirmation keeps showing a collapsed round's pills as words and its disclosures open. Nothing else in the body changes.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. `markdown.OneDisclosure` already parses with goldmark, which reads blockquotes.

**Storage**: Unchanged.

**Testing**: Test-first. Render tests pin the quoted demotion, each dropped divider, the marker's place, the `> > ` location line and each read-back refusal. A new fixture, `testdata/sticky-v0.12.0-layout.md`, is a three-round body written by v0.12.0 (the tree at `a489c82`) and reads back and publishes over. `markdown` tests pin `OneDisclosure` and the two display mappers on quoted lines. The sticky body golden is regenerated with `-update`, and a three-round golden is added. Every read-back guard is confirmed by mutation. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.0.0. `docs/comment-format.md` is a contract, amended here in its Sticky reviews section only. A body without `--sticky`, and the newest round of a sticky body, MUST NOT change by one byte (SC-003).

**Scale/Scope**: `internal/render/sticky.go`, `internal/markdown/allowlist.go`, their tests, one fixture, one new golden, `docs/comment-format.md` and `docs/github-facts.md`.

## Research

- **Quoting.** Decision: `quoteLines` prefixes each line with `> `, and a line that is empty or only whitespace becomes `>`. Rationale: FR-001. A whitespace-only line without a marker would end the quote. Inside a fence such a line turns empty, which renders the same.
- **Heading level.** Decision: loupe's section headings keep `###` inside the quote, and the `####` demotion goes. Rationale: the owner's call after the sandbox check. Quoted lines never match `sectionHeadings`, which reads only the round on top, so read-back is unaffected. Rounds collapsed with `####` are carried as they are.
- **Which dividers go.** Decision: the demoted round drops the divider before each of loupe's section headings (the one after the prose, or after the chips row when there is no prose, and the one between `Must fix` and `Worth a look`), the one before the footer and the one after it. A divider the author wrote inside the prose stays. Rationale: FR-002 names loupe's dividers, which split a round into what reads as two. The author's prose is theirs and is carried as written, as its headings already are. A section divider missing because the body was edited on GitHub is left missing rather than refused, since nothing is lost.
- **The marker.** Decision: the reconciliation marker follows the quote after one blank line, then `</details>`. Rationale: FR-003. The blank line ends the quote, so the marker stays an HTML comment outside it and reconciliation's whole-line match reads it as today.
- **Recognizing a quoted round.** Decision: a collapsed round is in the quoted layout when its content, the lines between its `<summary>` and its reconciliation marker, ends on `> ` and a footer line, or on a bare footer line directly under a line that starts with `>`. Rationale: every earlier layout ends that content on `---`, on a bare footer under a divider, or, for v0.11.0, on the round's own last line, so the footer is what tells the layouts apart. Keying on a leading `>` instead would take an older round whose prose opens on a blockquote for a quoted round and refuse it. Superseded: the lost-marker refusal that needed this is gone (see Refusals), and `keepsFooter` alone reads the quoted footer.
- **Refusals.** Decision: the newest collapsed round MUST keep its footer, and `keepsFooter` accepts a quoted footer. A round in which a line lost its `>` is carried as it is. Rationale: FR-006 as amended. The first build refused a lost marker, but it recognized a quoted round by a footer-shaped last line, which a v0.11.0 round's authored prose can also end in, so the CI review of #54 showed it refusing a valid legacy series. A lost marker only moves a line outside the quote. A quoted footer removed on GitHub leaves no footer, so the rule that already refuses a newest round without one covers it. A line that starts with `>` and no space still renders inside the quote, so only the `>` is required.
- **Earlier layouts.** Decision: nothing in the reading of v0.11.0 and earlier bodies changes. A v0.12.0 round is recognized as today. Rationale: FR-005 and User Story 2.
- **`OneDisclosure`.** Decision: unchanged, and pinned by tests. Rationale: goldmark parses the quote and reads a `<details>` inside it, and a tag in a fence inside it, as GitHub does, so a quoted round balances and a stray tag in one still refuses.
- **The display mappers.** Decision: `mapTagLines` also reads each run of structural lines that start with `>` with one marker taken off, recursively, and puts the marker back. Rationale: the terminal confirmation calls `OpenDetails` and `PillsAsWords` on the whole body. Without this, a collapsed round's finding rows would show a line of pill HTML where they show a word today. `Check` and `StructuralLines` do not change: authored text is checked before it is quoted, and the delimiters, markers and record read-back look for lines that a quoted line can never equal.
- **A tab-indented code block.** Decision: quoted as written, and left to the owner. Rationale: `POST /markdown` renders a code block indented by one tab on top, but renders `> ` and that tab as a paragraph, since the tab then reaches only column 4 of the quote's two. A fenced block, a list continued by a tab and a code block indented by four spaces render as on top. Expanding the tab would rewrite authored bytes, which FR-001 does not ask for.
- **Length.** Decision: unchanged. Rationale: the drop-oldest loop in `publish.Build` measures the composed body, so the two added characters a line are counted with it.
- **The rendering fact.** Decision: recorded in `docs/github-facts.md` as observed on 2026-09-25 with `POST /markdown` (render only). Rationale: SC-004. Tests MUST NOT reach GitHub.

### GitHub facts

| Fact | Status | Source |
| :--- | :--- | :--- |
| `<details>` and a nested `> >` quote render inside a blockquote | Observed 2026-09-25 | The owner's session, with GitHub's `POST /markdown` render endpoint |
| A body's rendered review matches `POST /markdown` in `gfm` mode | Assumed | Both are GitHub's renderer; the review page is not captured by this feature |

## Constitution Check

- **I. A tool for agents.** PASS. No command changes.
- **II. Nothing posts unread under a human's name.** PASS. The quote adds no words. The confirmation still shows the whole replacement body, quote markers included, and keeps its pill words and open disclosures inside the quote.
- **III. Local files, no service.** PASS. Not in scope.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. No result, refusal code or exit code changes. A body that is now refused is refused with the existing `sticky` code and fix.
- **VI. Simplicity over ceremony.** PASS. No dependency and no new abstraction. Rounds already collapsed are carried as they are rather than rewritten by a parser per layout.
- **VII. Verified means ran.** PASS. Every behavior is tested, each new guard is confirmed by mutation, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

### Documentation (this feature)

```text
specs/030-quoted-rounds/
├── spec.md
├── plan.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/render/sticky.go          # quoted demotion, quoted read-back and its refusals
internal/markdown/allowlist.go     # mapTagLines reads through quote markers
testdata/sticky-v0.12.0-layout.md  # new: a three-round body written by v0.12.0
testdata/golden/sticky.md          # regenerated: round 1 quoted
testdata/golden/sticky-three.md    # new: rounds 1 and 2 quoted
docs/comment-format.md
docs/github-facts.md
```

**Structure Decision**: The existing single Go module. No new package or file beyond the fixture and golden.

## Complexity Tracking

None.
