# Research: Anchor a finding to a Markdown heading

Each decision below resolves a "how" that the spec leaves to the plan. The spec's Clarifications settle the "what", and nothing here reopens them.

## R1: Parse headings with goldmark, not a line scanner

- **Decision**: `internal/markdown` gains `Headings(src []byte) []Heading`. It parses the file with the goldmark parser loupe already uses, keeps only `*ast.Heading` nodes that are direct children of the document, and keeps a node only when its source line matches `^ {0,3}#{1,6}([ \t]|$)`. The line comes from `node.Pos()`, which goldmark v1.8.6 sets for every block, an empty heading included. The text is the heading's `Lines()` segments, which goldmark already strips of the opening and closing `#` runs, with whitespace trimmed and each inner run collapsed to one space (FR-007).
- **Front matter**: goldmark core has no front matter. When line 1 is `---`, the lines up to the next `---` or `...` line are blanked before the parse, keeping their newlines, so line numbers stay true. Without a closing line nothing is blanked, as in a file that only opens with a thematic break.
- **Rationale**: a probe on 2026-10-28 against v1.8.6 showed the document's direct children cover FR-006 by construction. A fenced block, an indented block, a block quote and a list each come back as one container node, so a `#` line inside one is never a top-level heading. A setext heading comes back as `*ast.Heading` too, and the ATX line check drops it. A line scanner would have to rebuild CommonMark's container rules for lists, lazy continuation and HTML blocks to get the same answer.
- **Alternatives**: a line scanner tracking fences and front matter. It is smaller, but it misreads a `#` line inside a list item or an HTML block, and FR-019 prefers the existing dependency.

## R2: The outline file

- **Decision**: capture writes `outline.json` on every capture, as an optional run file beside `previous.json` and `comments.json`:

  ```json
  {"schema": 1, "files": [{"path": "specs/007-x/plan.md", "headings": [{"heading": "Rollout", "level": 2, "line": 14}]}]}
  ```

  `files` lists every diff entry whose new name ends in `.md`, `.markdown` or `.mdx` in any case, that is not deleted and not binary, in diff order, once per path. A file with no headings is listed with an empty `headings`, so a missing entry is damage and never a file without headings.
- **Reading the head**: `gitx.Blob(clone, headRef, path)` runs `git cat-file blob <headRef>:<path>`. It writes nothing, so Principle IV holds. `cat-file blob` never applies textconv or filters, which matches the `--no-textconv` diff. A read error fails the capture through `leftRefs`, as any other git failure after the fetch does (FR-004).
- **Strictness**: `run.LoadOutline` reads it through `ReadJSON` at schema 1, so an unknown field is damage and a higher schema asks for an upgrade. A missing file returns a nil outline, which means a run from before this feature (FR-014).
- **Alternatives**: storing the outline in `target.json` would bump the target schema and load it on every command. Reading the clone at `add` time breaks FR-005.

## R3: Resolution lives in `internal/diff`

- **Decision**: `diff.Outline` is `map[string][]markdown.Heading`, and nil means no outline file. `Diff` gains an `Outline` field that only `add` and `edit` set, so the draft's signatures do not change. `(*Diff).ResolveHeading(path, text string) (startLine, line int, err error)` holds the section rules and every refusal of FR-003 and FR-010 to FR-014. It sits beside `Validate` in `locate.go`'s package, since it reuses `fileOn`, `notAFile` and `formatRanges`, and its result goes through `Validate` as FR-009 asks.
- **Section bounds**: heading *i* spans from its line to the line before the next heading with a level at or above its own, or to the end of the file.
- **Changes per hunk (FR-008)**: an added line counts at its new number. A deleted line counts at its preceding new-side line: the last context or added line before it in the hunk, else `NewStart - 1`, or `NewStart` when the hunk has no new-side lines. A value of 0 belongs to no section.
- **Range (FR-009)**: the first and last new-side line of the hunk inside the section. A hunk that changes the section always holds such a line under `-U3`, since a deletion's preceding line is either in the hunk as context or is line 0. A hunk with none is skipped rather than trusted.
- **Load only when needed**: `add` and `edit` read `outline.json` only when an input carries a heading, so a damaged outline never refuses a line-anchored finding.

## R4: The finding's location gets its own type

- **Decision**: `draft.Finding.Location` becomes `*draft.FindingLocation`, with the fields of `draft.Location` plus `Heading string \`json:"heading,omitempty"\``. Its `Published()` method returns the plain `draft.Location`. `draft.Location` stays the type of the envelope, `EarlierFinding` and `previous.json`.
- **Rationale**: `draft.Location` is shared by the attempt, the receipt, the previous round and the assessments. Adding the heading there would put it in the receipt, against FR-016, and bump three more schemas. One new type keeps the change to the draft.
- **Schema**: the draft goes to `schema` 3, which adds `findings.location.heading,omitempty`. The change only adds a field, so `ReadJSON` needs no migration (`docs/versioning.md`).
- **Readers**: the envelope builds its `Location` from `Published()`. The digest, the render input and the findings record already copy the four fields one by one, so none of them can carry the heading.

## R5: Input uses its own type

- **Decision**: `draft.FindingInput.Location` becomes `*draft.LocationInput`: the line-form fields plus `Heading *string \`json:"heading,omitempty"\``. A pointer tells `"heading": ""` from an absent key, so an empty heading refuses with `input` instead of reading as line 0. `edit` decodes its `location` into the same type.
- **Shape checks (FR-002)**: with a heading, a non-zero `line` or `startLine`, or a `side` other than empty or `RIGHT`, refuses with `input`. An empty or all-whitespace heading refuses with `input`.
- **Order**: the location stays the last check in `validateInput`, so an entry with a missing title still reports the title first. `validateInput` returns the resolved `*FindingLocation`, and `Add` stores what it returned.
- **Edit**: `applyEdit` decodes the location and `Edit` resolves it before it compares the old and new locations. An edit that names the same heading and resolves to the same range changes nothing. An edit to a line form at the same range drops the heading, and that counts as a change (FR-015).

## R6: Refusal texts

All refusals use code `location` except the shape checks of R5. `formatRanges` gives the existing `path:12-18, 30-34` style.

| Case | Message | Fix | Details |
| :--- | :--- | :--- | :--- |
| Not Markdown (FR-003) | `plan.txt is not a Markdown file; a heading anchor needs .md, .markdown or .mdx` | `use a line location: {"path", "line", "side", "startLine"}` | none |
| Deleted file | `plan.md is deleted by the pull request and has no headings at the head` | `use a LEFT line location or a general finding` | none |
| Binary file | existing binary refusal | existing | none |
| No outline (FR-014) | `this run was captured before heading anchors, so it holds no heading outline` | as FR-003 | none |
| No match (FR-011) | `plan.md has no heading "X"` | `use one of: A, B` | `headings` |
| No change (FR-012) | `the section "X" of plan.md has no changes in the diff` | as FR-011 | `headings` |
| Two hunks (FR-010) | `the section "X" of plan.md is changed in 2 hunks` | `file one line-anchored finding per hunk: plan.md:12-18, 30-34` | `ranges` |
| Duplicate (FR-013) | `plan.md has 2 headings "X", on lines 4, 20` | `use a line location: plan.md:6-9, 22-25` | `matches` |

The FR-011 fix lists only headings whose section changes in exactly one hunk, since a section changed in more hunks refuses under FR-010. When no section of the file changes in exactly one hunk, the FR-011 fix is `no section of plan.md changes in exactly one hunk; use a line location or a general finding with "general": true`. When no duplicate holds a change, the FR-013 fix and `details.headings` are those of FR-011.

## R7: The review interface

- **Decision**: the detail's location line becomes `<file glyph> plan.md:12-18 · Rollout`, joined with `Glyphs.Sep`, which every icon tier defines. A finding with no heading renders as it does today. The list, the hunk view and the whole-file diff do not change, since the whole-file diff already marks `Location.Line`.

## R8: The live probe

- **Decision**: one capture of `eriksaulnier/loupe-probe#1`, one heading-anchored `add` whose resolved range starts on a context line, and one `loupe publish --inline all`. Publishing needs the human's acceptance and confirmation (Principle II), so the agent stages the run and stops at the review step. The owner either runs `loupe review` and the publish, or tells the agent how to finish this one probe. The result goes into `docs/github-facts.md` under Inline review comments: the date, the review id, and what GitHub returned.
- **Alternatives**: a raw `gh api` review creation would test GitHub, not loupe's publish path, and FR-022 names `--inline all`.
