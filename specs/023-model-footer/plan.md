# Implementation Plan: The model in the review footer

**Branch**: `023-model-footer` | **Date**: 2026-09-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/023-model-footer/spec.md`

## Summary

`render.Body` appends `` · `MODEL` `` to the footer after the source segment and before ` · unattended`, whenever `Input.Model` is set. `loupe-meta` is untouched. The Footer and Markers sections of `docs/comment-format.md`, the `--model` help, `contracts/cli.md` and the `human-review` skill drop "the footer never shows it". Three tests that assert the model's absence from the footer flip to assert its presence, one new body golden pins the attended form with a source, and literal footer assertions pin the rest.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. `target.json` already stores `model` (spec 008). The digest covers the publishable draft, not the body, so it does not move.

**Testing**: The footer cases in `internal/render/body_test.go` are extended first and fail against the current renderer. Then the renderer changes and `go test ./internal/render/ -update` writes the one new golden, which is read before it is accepted. The tests in `internal/publish` and `internal/integration` that assert the model is absent from the footer are rewritten to assert the exact footer. `go test ./internal/cli/ -update` runs to regenerate the `capture` help golden if the flag's help text moves one. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution Boundaries: the comment format is a contract, amended here through this spec. `loupe-meta`, the reconciliation marker, the digest and every body without a model are out of scope and MUST stay byte-identical.

**Scale/Scope**: About four lines in `internal/render/body.go` and its `Input.Model` comment, one help string in `internal/cli/capture.go`, three test files, one new golden, and wording in four documents.

## Research

- **Where the segment is built.** Decision: in `render.Body`, directly after the ` · via ` segment and before the ` · unattended` one, reusing `CodeSpan(OneLine(in.Model))`. Rationale: FR-001 and FR-002. The footer is built in that one place, and the preview and the request both send `env.Body` from `publish.Build` (`internal/publish/envelope.go:140`, `publish.go:155` and `:349`). Alternative rejected: a separate footer function. It has one caller, so Constitution VI does not justify it.
- **The `model=` append stays where it is.** Decision: leave `meta += " model=" + in.Model` after the unattended block. Rationale: FR-006. Moving it next to the new footer line would read cleaner but would invite a reorder of the marker keys. The existing comment says why the footer and marker cannot build in step.
- **Escaping.** Decision: `CodeSpan(OneLine(...))`, like the SHA and the source. Rationale: `run.ModelPattern` (`^[a-z0-9][a-z0-9._/:-]*$`) admits no backtick or whitespace, so both are no-ops on valid input. They cost nothing and keep the comment-format rule that every generated span goes through them true without a caveat.
- **Which goldens move.** Decision: none of the existing ones. No existing golden input carries a model (`grep model= testdata/golden` finds none), so SC-002 is checked by `go test` passing on the unchanged files. One new body golden, `modeled.md`, carries a source and a model on the attended path, beside `sourced.md`. The unattended and no-source forms are pinned as literal footer strings in `TestBodyFooterSegmentOrder`, which already enumerates the forms. If an existing golden moves, that diff is a finding, not a regeneration.
- **The `capture` help golden.** Decision: rewrite the `--model` flag help from "recorded in the published review's loupe-meta" to say it shows in the footer and `loupe-meta`. Rationale: FR-009. If a `testdata/golden/cli` help golden holds that text, it is regenerated with `-update` and its diff read. A search for the phrase under `testdata/` found nothing, so none is expected to move.
- **The doc example.** Decision: leave `example.md` and the doc's example without a model. Rationale: the example pins the attended body with no source, the most common hand-run form. The Footer section's list of forms carries the new ones instead. `TestExampleGoldenMatchesDoc` keeps passing without an edit.
- **The demo.** Decision: no change. `cmd/loupe-demo/seed.go` already seeds `Model: "demo/reviewer-1"`, so `mise run demo` and the README images show the segment once rebuilt. The README's `docs/assets/review.png` is redrawn only by `mise run review-screenshot`, which posts a GitHub review. That rerun is listed as pending for the owner.
- **The gifs.** Decision: not rerun here. `docs/tapes/walkthrough.tape` reaches the publish confirmation, whose preview may show the footer. Nothing checks the gifs are current, and a rerun rewrites every gif whether or not a frame changed. The rerun is listed as pending for the owner, beside the screenshot.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The only contracts are `docs/comment-format.md` and `contracts/cli.md`, amended in place, and no stored entity changes.

## Constitution Check

- **I. A tool for agents.** PASS. No command or flag is added. The `--model` help changes wording only.
- **II. Nothing posts unread under a human's name.** PASS. The footer is composed by loupe, is shown in the attended confirmation preview, and rides in the one review request. The segment carries no word claiming the model wrote a human's review.
- **III. Local files, no service.** PASS. Nothing new is written or fetched.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. `--json` envelopes, refusal codes and exit codes are unchanged. The comment-format contract is amended through this spec, per Boundaries.
- **VI. Simplicity over ceremony.** PASS. No new function, type or dependency.
- **VII. Verified means ran.** PASS. Tests are extended to fail first, use the fake GitHub and local repositories, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/023-model-footer/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/render/body.go               # the model segment; Input.Model comment
internal/render/body_test.go          # segment order table gains model rows; TestBodyMetaNamesModel asserts the footer; modeled.md case
internal/publish/envelope_test.go     # TestBuildCarriesModel asserts the footer segment
internal/integration/fields_test.go   # asserts the full footer instead of the model's absence
internal/cli/capture.go               # --model help
testdata/golden/modeled.md            # new: source and model, attended
docs/comment-format.md                # Footer forms and rules; model= rule drops "never appears in the footer"
specs/001-loupe-v1/contracts/cli.md   # capture: the footer carries --model
plugin/skills/human-review/SKILL.md   # --model shows in the published footer
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| The model shows in the footer | `specs/008-finding-fields/spec.md` ("Why these fields": model in `loupe-meta`, not the footer) and `docs/comment-format.md` Footer "Nothing else" | Owner, 2026-09-24: for an AI review the model is part of how a reader weighs the findings |
