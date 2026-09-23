# Implementation Plan: Two sections and readable rows

**Branch**: `020-review-format` | **Date**: 2026-09-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/020-review-format/spec.md`

## Summary

`section.Rank` becomes blocking against the rest, so `draft.Ordered` and every terminal surface follow the new body order with no change of their own. `render.Body` emits `Must fix` and `Worth a look` in place of five label sections. `summaryLine` becomes one row rule for body and inline: dot, label in `<b>`, a severity pill image, a colon, the title. The row carries no location. The chips row, meta block, footer and markers keep their bytes. Inside the disclosure, impact leads, one-line fields sit on their label's line, the suggested fix becomes allowlist-checked Markdown with a fenced fallback, and references become short links. `docs/comment-format.md` is amended, and `docs/github-facts.md` records the image spike.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. Stored order stays arrival order, and the digest does not move.

**Testing**: New cases in `internal/render/body_test.go`, `inline_test.go` and `internal/section/section_test.go` are written failing first. Then `go test ./internal/render/ -update` and `go test ./internal/cli/ -update`, each followed by reading the diff. The publish goldens under `testdata/golden/publish/` are rewritten from the renderer's output and read. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution Boundaries: the comment format is a contract, and this specification is its amendment. `loupe-meta`, the reconciliation marker, the footer, the chips row's bytes and the meta block are out of scope.

**Scale/Scope**: One function in `internal/section`, the section loop and `summaryLine` in `internal/render/body.go`, two documents amended, one clarification line added, goldens regenerated.

## Research

- **The chips row keeps its bytes.** Decision: `chipsRow` is unchanged in code. Rationale: FR-005. Today's label sections hold only nonblocking findings, so per-label counts over nonblocking findings are what it already emits. Only the contract's wording of the rule changes, from "one chip per section" to "one chip per row dot".
- **`Rank` keeps its name and signature.** Decision: `section.Rank(label, blocking)` returns 0 for blocking and 1 otherwise, and ignores the label. The label parameter stays because it keeps both callers unchanged, and `Group` stays the tie-break. Alternative rejected: deleting `Rank` and sorting on `Blocking` inline in two packages. FR-003 wants one shared definition, and the body and `draft.Ordered` would then each state it.
- **One sort for both sections.** Decision: `Body` partitions on `Blocking` and sorts each part with the same comparator: severity, label group, id. This is today's `Blocking` comparator applied to both. Rationale: FR-002. `draft.Ordered` already sorts rank, severity, group, id, so it is byte for byte the same sequence.
- **The TUI draws no section headers.** Decision: no change in `internal/tui` or `internal/cli/show.go`. Rationale: a search found no heading drawn from the label sections on any terminal surface. They read `draft.Ordered`, which reads `section.Rank`. The change reaches them through that function, and their tests that assert an order across label sections are updated where they break.
- **Escaping inside the row.** Decision: the label is collapsed to one line and passed through the same escaping the title uses, which is HTML escaping in a `<summary>` plus backslash-escaping of ASCII punctuation inline. Rationale: FR-016. GitHub parses no Markdown inside `<summary>`, and an inline first line is a Markdown paragraph where backslash escapes decode (github-facts, 2026-09-21).
- **No location on the row.** Decision: dropped after the owner compared a same-line and a second-line location on the scratch PR. Rationale: FR-013. The meta block's first line is the full linked location, and it is what a reader sees first on opening the row.
- **Severity as a pill.** Decision: `severityPill(word)` returns a fixed `<picture>` string per enum word. Rationale: FR-010 and the second spike round. The string is fixed per word, so nothing reviewer-supplied reaches an attribute. `PillsAsWords` replaces those exact strings with the word for the terminal, which cannot mis-hit authored content, since the allowlist refuses raw `<picture>` in any authored field.
- **Hosting.** Decision: `assets/review/v1/` on `main`, pinned by a hash test. Alternatives rejected: a commit SHA from this branch, which the repository's squash merges leave unreachable; a separate assets repository with a tag, which the owner declined as another repository to keep.
- **The colon.** Decision: `: ` after the last of label and pill, and none when both are absent. Rationale: the owner's call, so that the eye finds where the title starts.
- **The dot inline and in the body is one function.** Decision: the existing inline dot logic (`⛔` when blocking, else `dots[group]`) moves ahead of the context check and applies to every row. Rationale: FR-009 and SC-003.
- **The summary context type shrinks.** Decision: `inBlocking`, `inLabelSection` and `inOther` collapse into one body context, and `inInline` stays. The only remaining difference between contexts is the location part and inline escaping. So a `bool` inline flag MAY replace the enum. Pick whichever keeps the diff smaller.
- **`sectionTitles` goes away. `dots` stays.** Decision: the headings are the two literals `Must fix` and `Worth a look`. The dot array still feeds chips and rows.
- **Impact first.** Decision: meta, impact, body, fix, references. Rationale: FR-022. The consequence decides whether the reader reads on.
- **Inline labels for one line.** Decision: `labeled` puts a value with no newline after `**Label:** `, and anything longer below `**Label**`. Rationale: FR-023. A single line prefixed by bold text stays a paragraph whatever it starts with, so the rule is safe. A list, quote or fence can only open at the start of a line, so a longer value keeps its own block.
- **The fix becomes Markdown, and old fixes fall back.** Decision: `validateInput` checks `suggestedFix` like `impact`. `render` checks it again and fences it when it fails, instead of refusing at publish. Rationale: FR-024. A run stored before this change can hold anything, and refusing it at publish would strand a round over a formatting change. The fence keeps such a fix inert as it always was. `render` importing `internal/markdown` adds no cycle.
- **Reference links.** Decision: `[text](<url>)`. The text escapes `&` and ``\ ` * _ [ ] ~``. The destination doubles `\` and escapes `&`, since CommonMark decodes both inside `<…>`. Rationale: FR-025. Input validation already refuses whitespace, `<`, `>` and backticks, which are what could end the destination.
- **The meta block stays.** Owner, 2026-09-22: a one-line meta (`path · high confidence · reproduced`) would make it harder to read. It keeps one part per line.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The only contract is `docs/comment-format.md`, and this change amends it in place. No entity changes.

## Constitution Check

- **I. A tool for agents.** PASS. No command or flag changes. `add` and `edit` now refuse a `suggestedFix` that fails the allowlist. They use the existing `markdown` refusal code and correction, so no new code appears, and the help text, skill and `contracts/cli.md` say so.
- **II. Nothing posts unread under a human's name.** PASS. Gates and confirmation are untouched. The human still meets every finding, now in the new order.
- **III. Local files, no service.** PASS. Nothing loupe writes changes, and loupe makes no new network call. Readers' clients load the pills from GitHub. That is a dependency of the published review, not of loupe, and the alt word covers its failure.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. The `--json` envelopes are unchanged. The comment-format contract is amended through this spec, per Boundaries.
- **VI. Simplicity over ceremony.** PASS. There is no new package and no dependency. Code is removed: `sectionTitles`, three summary contexts and the per-section sort.
- **VII. Verified means ran.** PASS. Tests fail first, goldens are regenerated deliberately and read, and `mise run check` runs after the last edit. Real GitHub rendering of the final composed body is checked by one post to the scratch PR, which is not a pull request of loupe's.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/020-review-format/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/section/section.go        # Rank: blocking against the rest
internal/section/section_test.go   # the new rank
internal/render/body.go            # two sections, one comparator, one row rule, disclosure order and labels, reference links
internal/draft/mutate.go           # suggestedFix allowlist check
internal/cli/add.go, edit.go       # --suggested-fix help text
plugin/skills/human-review/SKILL.md # suggestedFix is Markdown
specs/001-loupe-v1/contracts/cli.md, data-model.md # suggestedFix is Markdown
internal/render/body_test.go       # mixed, only-blocking, only-nonblocking, unrated, unlabeled, unknown, LEFT, range, general
internal/render/inline_test.go     # the same row without location
internal/markdown/allowlist.go     # lone CR as a line break; MapSummaryLines for the pill display
internal/tui/confirm.go, plain.go  # pills shown as words; the message box sits under the chips row
cmd/loupe-demo/                    # loupe-demo body, the input to the README's review picture
assets/review/v1/                  # the severity pill SVGs, pinned by hash
scripts/review-screenshot.sh       # the README's picture of a published review
testdata/golden/*.md               # regenerated, example.md from the doc
testdata/golden/publish/*.md       # regenerated from renderer output
testdata/golden/cli/*              # regenerated if they move
docs/comment-format.md             # Vocabulary, Body composition, Chips, The summary line, example, image ban rationale
docs/github-facts.md               # the image spike
specs/001-loupe-v1/spec.md         # one clarification line
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| Five label sections become two | `specs/001-loupe-v1` clarification log (section headings with dots), `docs/comment-format.md` Body composition | Real reviews hold zero to ten findings, usually one. The label is the weakest sorting axis, and five headings cost more structure than they index |
| Body rows carry a dot | `specs/001-loupe-v1` ("rows in the label sections still carry no dot"), `specs/014-severity-badges` table | With no label headings the row carries its own mark, and the chips row becomes a key for those marks |
| ` · ` bold prefix and trailing colon go | `specs/014-severity-badges` FR-006, FR-011 | Separate styled tokens read as fields, and one bold-then-plain run does not |
| Unrated nonblocking rows no longer render byte for byte as before | `specs/014-severity-badges` FR-008, SC-003 | Every row changes format. Comparability with older reviews is given up deliberately |
| Chips index row dots, not headings | `specs/014-severity-badges` FR-012 rationale, `docs/comment-format.md` Chips | The headings no longer name labels. The chips' bytes do not change |
| The severity pill is an external image | `docs/comment-format.md` ("No external badge images"), `specs/014-severity-badges` FR-007 and FR-016 | The second spike round met the bar the ban protected: legible, centered, no taller than text, and the click is kept. The alt word covers a pill that does not load |
| Terminal order changes across labels | `specs/014-severity-badges` FR-001 section list | FR-001's rule (terminal order equals body order) is kept, and only the body order it points at changes |
| Impact before the body; fix and references reformatted | `specs/008-finding-fields` (impact after the body, fix always fenced, references as autolinks), `docs/comment-format.md` | Consequence first reads better, and a fenced sentence reads as code. Stored fixes that fail the allowlist keep the fence |
| An enum severity is back on the meta line as text | `specs/014-severity-badges` FR-014 (one severity, in one place) | The row now shows severity only as an image, which text-only readers drop; the meta line is the text channel (FR-029) |
| `suggestedFix` becomes allowlist-checked Markdown | `specs/001-loupe-v1/contracts/cli.md` ("prose or code") | Prose renders as prose. Writers fence code. A new fix with raw HTML is now refused |
