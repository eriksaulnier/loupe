# Implementation Plan: Richer findings and run provenance

**Branch**: `008-finding-fields` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/008-finding-fields/spec.md`

## Summary

A finding gains `impact`, `verified` and `references`, and `severity` narrows to four words. Severity and verified join confidence on the meta line as bold labels; impact and references become sections inside the disclosure. A run gains an optional `model` from `loupe capture --model`, carried in `loupe-meta` only. Every new field follows the plumbing `confidence` and `suggestedFix` already have, and `model` follows `source`. Existing runs load, render and publish unchanged, and a body without the new fields is byte for byte what it was.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. URL parsing is `net/url`.

**Storage**: `draft.json` findings gain three optional keys; `target.json` gains one. No schema bump: every key is additive and absent on existing records.

**Testing**: `internal/draft` for validation, clearing, notes and the digest; `internal/run` for the model pattern; `internal/render` for the meta line, sections, marker and the doc-pinned example golden; `internal/cli` for flags, help goldens and `show`; `internal/tui` for the detail and plain views; `internal/integration` for the end-to-end add, edit and publish against `fakegh`, and for the flag enumeration. No pseudo-terminal, no network.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.0 Boundaries (comment format is a contract, so this spec); spec 001 FR-007 and the Finding entity as amended; `docs/comment-format.md` and `contracts/cli.md` updated from the spec, not redesigned.

**Scale/Scope**: About 300 lines across `internal/draft`, `internal/run`, `internal/render`, `internal/publish`, `internal/cli`, `internal/tui` and `cmd/loupe-demo`, their tests, the goldens, three documents, the skill and the workflow.

## Research

- **Severity enum and the legacy value.** Decision: `validateInput` in `internal/draft/mutate.go` gains a severity switch in the shape of the confidence switch, and `Edit` skips that one check when `next.Severity == stored.Severity`. Rationale: `Edit` re-validates the merged finding on any publishable change (`mutate.go:333`), so an unconditional switch would refuse a title edit on a run captured before this feature. `draft.Load` (`store.go:16`) validates schema only, so old values load and render. Alternative rejected: a load-time migration that rewrites old severities, which would change a draft's digest behind the human's back.
- **Verified.** Decision: same switch shape, values `reproduced` and `plausible`, absent allowed. Rationale: FR-002; it is reviewer-reported, never derived.
- **Impact.** Decision: a `string` with `omitempty`, checked with `markdown.Check(in.Impact, markdown.Body, …)` beside the body check in `validateInput` and again in `internal/publish/envelope.go:56`. Rationale: FR-003; it is Markdown the reader sees, so it takes the same allowlist and the same recheck at composition.
- **References.** Decision: `[]string` with `omitempty`; a `ValidateReferences` in `internal/draft` enforcing at most six, each at most 200 bytes, `url.Parse` with scheme `http` or `https` and a host, and no whitespace, control byte, `<`, `>` or backtick. An empty slice is stored as nil. Rendered as `- <url>` autolinks. Rationale: FR-004 and FR-010; the autolink form is why `<`, `>` and whitespace are refused, since any of them would end or break the autolink. Alternatives: a Markdown link per entry (needs a title and escaping); bare URLs (GitHub autolinks them, but that is GFM's extension, not CommonMark).
- **Edit and clearing.** Decision: `impact` and `verified` join the `decodeOptional` table in `applyEdit`; `references` gets its own branch because it decodes to a slice (`null` or `[]` clears, else `[]string`). `Edit` gains three `note()` lines, references compared with `slices.Equal`. The "can be cleared" message at `mutate.go:447` lists the new fields. Rationale: FR-005 mirrors `severity` exactly.
- **Digest.** Decision: `digestFinding` in `internal/draft/digest.go` appends `impact`, `verified` and `references` after `suggestedFix`, and its comment becomes an append-only rule. Rationale: FR-006. `Reconcile` reads the marker from the stored attempt and never recomputes (`publish.go:177`, `reconcile.go:17`), so a changed encoding cannot orphan an attempt. `digest_test.go:30` pins a literal hash with its document spelled out; that is a hand edit.
- **Meta line.** Decision: `metaBlock` (`internal/render/body.go:235`) builds the second line from `**Confidence:**`, `**Severity:**` and `**Verified:**`, each `EscapeHTML(OneLine(...))`, joined by ` · `; the code-span severity goes. `disclosure` (`body.go:219`) appends `**Impact**` between body and fix and `**References**` after it. Rationale: FR-008 to FR-011. The inline comment reuses `disclosure`, so FR-011 holds without a second path.
- **Model.** Decision: `run.Target` gains `Model string` with `omitempty`; `ModelPattern` is `^[a-z0-9][a-z0-9._/:-]*$`, at most 64 characters, no `--`, in a `ValidateModel` shaped like `ValidateSource` (`internal/run/target.go:60`) and called from `LoadTarget`. `render.Input` gains `Model`; the marker appends ` model=` after `src=` (`body.go:131` onward). Capture validates before `verifyClone`. Rationale: FR-013 to FR-015; the pattern excludes `>` and `--`, so the value is safe unescaped in the HTML comment, and admits `/` and `:` for `anthropic/claude-sonnet-5`. Optional per FR-016.
- **Show and the review interface.** Decision: `internal/cli/show.go` prints verified on the meta row and adds `impact` and `references` blocks beside `fix`; `internal/tui/app.go` `chips()` shows severity and verified; `detail.go` and `plain.go` render impact and references beside the suggested fix. Rationale: FR-007; the human decides with the same information the reader gets.
- **Docs and goldens.** Decision: `docs/comment-format.md`'s example block gains the fields, and `testdata/golden/example.md` is hand-edited to match, since `TestExampleGoldenMatchesDoc` derives one from the other. `go test ./internal/render/ -update` and `go test ./internal/cli/ -update` regenerate the rest; the diff is read for footer stability. Rationale: FR-012 is proven by the goldens that do not move.
- **Workflow and skill.** Decision: `review.yml`'s capture step tests the variable first and exits 1 with a message, then passes `--source loupe-review-action@<workflow ref>` and `--model`. The skill names the fields and the model flag. Rationale: FR-017 and FR-018.

## Constitution Check

- **I. A tool for agents.** PASS. Every field enters through `loupe add` JSON and flags; the skill and workflow only describe them.
- **II. Nothing posts unread under a human's name.** PASS. Readiness, decisions and confirmation are untouched. A changed field clears a decision as today.
- **III. Local files, no service.** PASS. Additive keys in existing files. References are never fetched.
- **IV. Never touch the user's checkout.** PASS. Capture writes one more key to its own target file.
- **V. Machine contract first.** PASS. Every new refusal is `input` with a fix; `--help` shows the new keys and flags; `contracts/cli.md` is updated from the spec.
- **VI. Simplicity over ceremony.** PASS. No dependency, no abstraction. `ValidateReferences` and `ValidateModel` each have exactly the two callers their siblings have.
- **VII. Verified means ran.** PASS. Failing test first per task; `mise run check` closes each phase; the live marker check stays unverified.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/008-finding-fields/
├── spec.md
├── plan.md
└── tasks.md

internal/run/target.go                 # Model, ValidateModel
internal/cli/capture.go                # --model
internal/draft/model.go                # Impact, Verified, References
internal/draft/mutate.go               # validation, clearing, notes
internal/draft/digest.go               # appended keys
internal/render/body.go                # meta line, sections, model=
internal/publish/envelope.go           # copy fields, impact recheck, model
internal/cli/add.go, edit.go           # flags, help
internal/cli/show.go                   # verified, impact, references
internal/tui/app.go, detail.go, plain.go
cmd/loupe-demo/seed.go                 # seeded fields and model
testdata/golden/example.md             # hand-edited with the doc
testdata/golden/cli/*.txt              # regenerated
docs/comment-format.md                 # vocabulary, meta line, sections, model=
specs/001-loupe-v1/contracts/cli.md    # capture, add, edit
specs/001-loupe-v1/data-model.md       # finding table
plugin/skills/human-review/SKILL.md    # fields, --model
.github/workflows/review.yml           # variable guard, --source, --model
```

**Structure Decision**: No new package, no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 007. The research is above and the data model change is the finding table in spec 001.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Spec 001 data model: `severity` narrows from arbitrary text to four words | A reader compares severity across findings and the meta line renders it as a word beside confidence | Keeping free text keeps the code span and the asymmetric line that prompted this |
| `Edit` validates severity only when it changes | Runs captured before this feature hold free-text severity and must stay editable | Unconditional validation refuses a title edit on an old run |
| Digest encoding gains keys, changing the hash of unchanged content | The new fields are content and a change to one must clear a decision | Leaving them out lets an edited impact publish under a stale digest |
