# Implementation Plan: A note on the newest sticky round

**Branch**: `032-sticky-round-note` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/032-sticky-round-note/spec.md`

## Summary

`loupe publish --unattended --sticky --note <markdown>` puts the note under the newest round's footer between two hidden delimiters. `render.ReadSticky` removes the delimited lines before it reads the round, so the collapsed round is what it would be without the note. The note travels from the CLI through `publish.Options` and `publish.BuildInput` to `render.StickyInput`, and never reaches the draft, so the digest cannot cover it.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. The note is in the envelope's body, and so in `attempt.json` and the receipt, as any body text is.

**Testing**: Test-first. Render tests pin the note's place, its drop on collapse, a three-round series and each read-back refusal. Publish tests pin the digest and the usage refusal. An integration test runs two rounds through the CLI against the fake GitHub. The drop is confirmed by mutation. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.0.0. `docs/comment-format.md` and `contracts/cli.md` are contracts, amended here.

**Scale/Scope**: `internal/cli/publish.go`, `internal/publish/{publish,envelope}.go`, `internal/render/{body,sticky}.go`, `internal/markdown/allowlist.go`, their tests, the `publish --help` goldens and the two contracts.

## Research

- **Scope.** Decision: `--unattended --sticky` only. Rationale: Principle II forbids a command that sets prose under a human's name, and a review that is never edited never collapses.
- **Place.** Decision: under the footer, in the footer's block, with no divider of its own. Rationale: the footer closes the round, and the note reads as a postscript to it. A divider would add a fourth rule to every round.
- **Delimiters.** Decision: `<!-- loupe-note -->` and `<!-- loupe-note-end -->`, read on structural lines only. Rationale: the same guarantee as `<!-- loupe-round -->`: the allowlist refuses an HTML comment in authored text outside a fence.
- **Read-back.** Decision: remove the pair and the blank line before it first, then read the body as before. Rationale: every other read-back rule then applies unchanged. A pair that is not loupe's shape is refused, as the rest of read-back refuses what it did not write.
- **Allowlist kind.** Decision: a `markdown.Note` kind with the summary's depth and the name `note`. Rationale: a refusal that said `summary` would point a pipeline at the wrong text.

## Constitution Check

- **I. A tool for agents.** PASS. A flag any caller can pass.
- **II. Nothing posts unread under a human's name.** PASS. The note is refused without `--unattended`, whose review posts as an App and is marked unattended.
- **III. Local files, no service.** PASS. Not in scope.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. Both refusals use existing codes and name a fix. The `--json` result is unchanged.
- **VI. Simplicity over ceremony.** PASS. One flag, one field, no dependency.
- **VII. Verified means ran.** PASS. Tests as above, and `mise run check` after the last edit.

## Project Structure

```text
internal/cli/publish.go          # --note, its usage refusal and help
internal/publish/publish.go      # Options.Note and its usage refusal
internal/publish/envelope.go     # BuildInput.Note, its allowlist check
internal/render/body.go          # the note under the footer
internal/render/sticky.go        # StickyInput.Note, dropNote
internal/markdown/allowlist.go   # the Note kind
testdata/golden/cli/publish-help.{80,100}.txt
docs/comment-format.md
specs/001-loupe-v1/contracts/cli.md
```

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| :--- | :--- | :--- |
| Principle II names the draft's summary as the only source of an unattended review's prose; `--note` adds a second source, a flag | The hint belongs to the pipeline that runs every round, not to the round: it names the label that starts the next round, and a collapsed round MUST NOT keep it | Putting it in the summary repeats it in every collapsed round, which is the defect this spec fixes. The flag is refused unless the publication is both `--unattended` and `--sticky`, so it never sets words under a human's name |
