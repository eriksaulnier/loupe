# Implementation Plan: Inline comments default to none

**Branch**: `026-inline-default-none` | **Date**: 2026-09-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/026-inline-default-none/spec.md`

## Summary

Two defaults move from `blocking` to `none`: the `--inline` flag of `loupe publish` (`internal/cli/publish.go`) and the cursor the inline picker in `loupe review` starts on (`internal/tui/list.go`). Unattended publication reads the same flag, so it follows without its own change. Nothing in `internal/publish` or `internal/render` changes, since both take the mode as an input. The two contracts and `research.md` name the new default. The help goldens move, and the one integration test that relied on the old default now asserts `inline=none` and no comments.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. Attempts and receipts already record the mode they were sent with.

**Testing**: Failing first: an integration test that publishes attended without `--inline` and reads an empty `comments` list and `inline=none` from the fake GitHub, the same for `--unattended`, and a review-model test that presses Enter on an action and finds the picker's cursor on `none`. Then the two defaults change. `go test ./internal/cli/ -update` regenerates the two help goldens, and the diff is read. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution Boundaries: `docs/comment-format.md` is a contract, amended through this spec. `contracts/cli.md` is adopted, not redesigned: one sentence changes.

**Scale/Scope**: Two one-word code changes, two contract sentences, one `research.md` line, one demo comment and value, two help goldens, three new or amended tests.

## Research

- **Where the flag default lives.** Decision: the cobra default in `newPublishCmd` becomes `"none"`. Rationale: `runPublish` reads the flag once for both attended and unattended paths, and `publish.Options.Inline` carries it to `publish.Build`. No other code names a default. `internal/publish` and `internal/render` panic or refuse on an unknown mode and never fill one in.
- **Where the picker default lives.** Decision: `updateAction` sets `m.pick` to `slices.Index(publish.InlineModes, "none")`, which is 0. Rationale: the clarification of 2026-09-24. Naming the mode rather than writing `0` keeps the line readable and survives a reorder of `InlineModes`. The esc path from the picker back to step 1 is unchanged.
- **What `loupe-workflows` passes.** Decision: nothing to change there. Its `.github/workflows/review.yml` publish step runs `loupe publish --unattended --json` with no `--inline` (read from the local clone at `~/projects/loupe-workflows` on 2026-09-24). Unattended reviews move to `none` once the pipeline installs a loupe release with this change. Rationale: the spec's User Story 2 and FR-007. That repository is out of scope for edits.
- **Which goldens move.** Decision: `testdata/golden/cli/publish-help.80.txt` and `publish-help.100.txt`, where cobra prints `(default "blocking")`. The body goldens under `testdata/golden/*.md` and `testdata/golden/publish/` pass `Inline` explicitly through `render.Input` or `publish.BuildInput`, so they MUST NOT move. If one does, that diff is a finding, not a regeneration. Rationale: SC-003.
- **Tests that pass a mode explicitly stay as they are.** Decision: tests that pass `Inline: "blocking"` or `"all"`, and the confirm tests that pass `"blocking"` to `ConfirmTitle`, keep their values. Rationale: they test a mode, not the default, and FR-002 says the explicit modes behave as before. The one test known to rely on the default is `TestPublishSendsCapturedSource` in `internal/integration/publish_test.go` (publish without `--inline`, then asserting `inline=blocking`). It is amended to assert `inline=none`. Any other test that turns out to rely on the default fails once the default moves, and is amended the same way only when it tests the default rather than a mode.
- **The demo body.** Decision: `demoBody` in `cmd/loupe-demo/main.go` composes with `Inline: "none"`, and its comment says it matches publish's default. Rationale: the comment says it composes the review "as publish would send it", and `mise run review-screenshot` posts that body. The visible body is the same for every mode, so only the hidden marker changes, and the README picture stays current. The demo test in `cmd/loupe-demo/main_test.go` passes `Inline: "blocking"` to exercise gates, not the default, and stays.
- **Contract wording.** Decision: `contracts/cli.md` says "`--inline` defaults to `none` (specs/026-inline-default-none)". `docs/comment-format.md` "Inline modes" says "default `none`". `specs/001-loupe-v1/research.md` step 4 says "`none` (default), `blocking` or `all`", with the spec reference. Rationale: FR-005. The two marker examples in `docs/comment-format.md` keep `inline=blocking`. They show a review sent with `--inline blocking`, which is still a valid marker, and `TestExampleGoldenMatchesDoc` pins them to `example.md`, which MUST NOT move.
- **Other documents.** Decision: `README.md`, `plugin/skills/human-review/SKILL.md` and `.github/review-instructions.md` do not name the default, so they are unchanged. Earlier specs' plans and tasks are dated history and are left as written. Rationale: SC-004 was checked with `git grep -n -- 'blocking.*default\|default.*blocking'`.
- **Screenshots.** Decision: `mise run screenshots` is left to the owner. `docs/tapes/walkthrough.tape` takes both publish steps at their defaults, so its gif's confirmation header changes from `inline blocking (1)` to `inline none (0)`. A gif re-renders differently on every run, and the brief lists it as pending. `mise run review-screenshot` is not needed: the visible body does not change.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The contracts are amended in place, and no stored entity changes.

## Constitution Check

- **I. A tool for agents.** PASS. `loupe publish --help` states the new default, so the workflow stays completable from help alone.
- **II. Nothing posts unread under a human's name.** PASS. The confirmation still shows every inline comment that will be sent, and the review is still one request. Unattended reviews still post as the App, as COMMENT, marked unattended.
- **III. Local files, no service.** PASS. Nothing new is stored or called.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. `--json` envelopes, refusal codes and exit codes are unchanged. `contracts/cli.md` changes one sentence through this spec.
- **VI. Simplicity over ceremony.** PASS. No new abstraction, package or dependency.
- **VII. Verified means ran.** PASS. Tests fail first against the fake GitHub and injected keys, goldens are regenerated deliberately and read, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/026-inline-default-none/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/cli/publish.go                     # --inline default "none"
internal/tui/list.go                        # picker cursor starts on "none"
internal/tui/list_test.go                   # Enter on an action lands the cursor on none
internal/integration/publish_test.go        # attended and unattended publish without --inline: no comments, inline=none
testdata/golden/cli/publish-help.{80,100}.txt  # regenerated with -update
cmd/loupe-demo/main.go                      # demoBody composes with the default mode
specs/001-loupe-v1/contracts/cli.md         # the default sentence
specs/001-loupe-v1/research.md              # step 4's mode list
docs/comment-format.md                      # Inline modes: default none
```

## Complexity Tracking

None.
