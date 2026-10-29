# Implementation Plan: Anchor a finding to a Markdown heading

**Branch**: `044-heading-anchors` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/044-heading-anchors/spec.md`, with the decisions in [research.md](research.md).

## Summary

An agent files `{"path": "plan.md", "heading": "Rollout"}` instead of a line range. `capture` stores `outline.json`, a heading outline of each changed Markdown file at the head, parsed with goldmark. `add` and `edit` resolve the anchor against `pr.diff` and that outline to the lines of the one hunk that changes the section. They store the range with the heading's text in a new `draft.FindingLocation`, and refuse with `location` and a fix that names what to use in every other case. The publication, the receipt and the findings record keep the plain `draft.Location`, so the published format does not change. The review interface shows the heading after the range on the detail's location line. One owner-approved live probe confirms that GitHub accepts a range that starts on a context line.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`), unchanged.

**Primary Dependencies**: goldmark v1.8.6, already direct (`internal/markdown`), parses headings. go-gitdiff, cobra and Bubble Tea as today. No new module.

**Storage**: one new strict run file, `outline.json` at `schema` 1. The draft goes to `schema` 3 for `findings.location.heading`. No other run file and no findings record changes.

**Testing**: `go test` with `internal/testutil/gitrepo` and `fakegh` for capture, add, edit and publish end to end. Unit tests for the parser and the resolver. The schema test pins both field sets. CLI goldens: only `--help` output may change.

**Target Platform**: as today, Linux, macOS and Windows.

**Project Type**: CLI with an interactive review interface.

**Performance Goals**: capture reads one blob per changed Markdown file. `add` reads `outline.json` only when an input carries a heading. Resolution is linear in the file's hunk lines and headings.

**Constraints**: Principle IV for the blob read (`git cat-file blob`, no checkout). `add` and `edit` never read the clone (FR-005). No change to `docs/comment-format.md`, the findings record's `v=`, the attempt or the receipt.

**Scale/Scope**: a spec pull request touches a handful of Markdown files with tens of headings each.

## Constitution Check

| Principle | Before research | After design |
| :--- | :--- | :--- |
| I. A tool for agents | Pass: an input form any shell agent can send | Pass: `--help` documents the form and its refusals, and the skill describes it next to the line form |
| II. Nothing posts unread | Pass: the human still decides each finding | Pass: the review interface shows the heading the published comment omits. The live probe's publish is the owner's confirmation |
| III. Local files | Pass: one more run file | Pass: `add` resolves offline from the run |
| IV. Never touch the checkout | Pass: capture reads blobs only | Pass: `git cat-file blob <headRef>:<path>` writes nothing |
| V. Machine contract first | Pass: refusals name the fix | Pass: every refusal of R6 names a fix the agent can apply in one retry. Both schema changes follow `docs/versioning.md`, and the schema test pins them |
| VI. Simplicity | Pass: existing dependency | Pass: two new types, each with a second use: `FindingLocation` separates the draft from three published records, and `LocationInput` is read by both `add` and `edit` |
| VII. Verified means ran | Pass | Pass: fake GitHub and local repositories in tests. The one live run is the probe FR-022 requires and the owner approved |

## Project Structure

### Documentation (this feature)

```text
specs/044-heading-anchors/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/cli-delta.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/markdown/headings.go       # Headings(src), Normalize, IsMarkdownPath (new)
internal/diff/heading.go            # Outline, (*Diff).ResolveHeading and its refusals (new)
internal/gitx/gitx.go               # Blob(clone, ref, path)
internal/run/outline.go             # OutlineFile, OutlineSchema, Outline, EncodeOutline, LoadOutline (new)
internal/cli/capture.go             # build and store outline.json
internal/cli/add.go, edit.go        # --heading, LocationInput, load the outline when needed, --help
internal/draft/model.go             # FindingLocation, schema 3
internal/draft/mutate.go            # LocationInput, resolve in validateInput and Edit
internal/publish/envelope.go        # Published() for the envelope location
internal/tui/detail.go, app.go      # heading after the range on the detail's location line
internal/publish/schema_test.go     # draft 3 and outline 1 field sets
internal/integration/heading_test.go  # end to end (new)
cmd/loupe-demo/seed.go              # FindingLocation type only
docs/versioning.md, docs/github-facts.md
specs/001-loupe-v1/contracts/cli.md
plugin/skills/human-review/SKILL.md
```

**Structure Decision**: the parser joins the existing goldmark user, `internal/markdown`. Resolution joins location validation in `internal/diff`, so it reuses `fileOn`, `notAFile` and `formatRanges`, and its result passes through `Validate`. `internal/run` owns the file, as it does `pr.diff` and `target.json`. `internal/run` already imports `internal/diff`, and `internal/diff` now imports `internal/markdown`. Neither imports `run`, so there is no cycle.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
| :--- | :--- | :--- |
| `draft.FindingLocation` beside `draft.Location` | FR-015 stores the heading in the finding's location, and FR-016 keeps it out of the receipt and every published record | One `Location` with a heading field is shared by the attempt, receipt, `previous.json` and assessments. It would leak the heading into the receipt and bump three more schemas |
| Draft `schema` 3 | `docs/versioning.md` requires a bump for the new field | None: the rule is not optional. Specs 041 and 046 also take draft schema 3 on another branch, so whichever merges second renumbers to 4 and keeps both field sets |
