# Implementation Plan: Sticky rounds anchored in data

**Branch**: `033-round-anchors` | **Date**: 2026-09-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/033-round-anchors/spec.md`

## Summary

`render.Body` writes one anchor line before each round of a sticky body, the top one at line 1. A new `internal/render/anchor.go` writes, parses and checks anchors, and reads an anchored body back. The current reader moves unchanged into `internal/render/sticky_legacy.go` and runs only on a body with no anchor. Its result is anchored before it is returned, so the next body is anchored throughout. `render.ReadSticky` returns rounds as values that carry their number, commit, anchor, block and whether they were edited, so the length limit drops a round with its anchor, and the confirmation can name edited rounds. The confirmation shows each anchor as `<!-- loupe-round N -->`.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. `crypto/sha256` is the standard library.

**Storage**: Unchanged.

**Testing**: Test-first. Render tests pin the anchor's grammar and checksum, each refusal, carrying an edited round, the note count, each fixture's migration and a round trip. The legacy reader's tests run on bodies converted to the v0.13.0 layout by a test helper, so they keep testing the frozen code. Publish tests pin the drop with anchors and the naming of edited rounds. TUI tests pin the anchor's display. The render goldens `sticky.md` and `sticky-three.md` regenerate. `GOFLAGS=-buildvcs=false mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.0.0. `docs/comment-format.md` is a contract, amended in its Sticky reviews section only. Non-sticky goldens MUST NOT change.

**Scale/Scope**: `internal/render/{anchor,sticky,sticky_legacy,body}.go`, `internal/publish/{envelope,sticky,publish}.go`, `internal/tui/{confirm,plain}.go`, their tests, one fixture, two render goldens, `docs/comment-format.md` and `docs/github-facts.md`.

## Research

- **Anchor grammar.** Decision: `<!-- loupe-round v=1 n=N commit=HEX [key=value …] sha256=HEX64 -->`, each key `[a-z0-9]+`, each value `[0-9a-z]+`, `sha256` last. Rationale: `loupe-meta`'s shape, and no value can hold `-` or `>`. A line that starts `<!-- loupe-round ` and is not the legacy delimiter is an anchor or malformed. Nothing else decides which reader runs.
- **Checksum input.** Decision: the anchor's text between `<!-- loupe-round ` and ` sha256=`, a newline, then the round's text with LF endings and blank lines trimmed at both ends. The top round's text ends with a newline and the reconciliation marker. Rationale: taking the fields as written covers keys a later loupe adds, so an older reader still checks them. The marker is carried into the collapsed round, so it is covered with it.
- **Counts on the top anchor.** Decision: the chips row's counts: `blocking`, then the non-blocking count of each label group. Rationale: `loupe-meta`'s census counts blocking findings in their label group too, so it cannot say how many chips each group shows. One function builds both the chips row and the summary pills from these counts.
- **Prose and note counts.** Decision: `prose` counts the opening prose's lines after blank lines are trimmed at both ends, and `note` the note's the same way. A sticky body writes both trimmed. Rationale: the reader trims the round's text at both ends, so a leading or trailing blank line would move every count. Trimming changes nothing GitHub renders, and only sticky bodies do it.
- **Demotion of a verified top round.** Decision: skip the chips row, keep `prose` lines, then remove each divider loupe wrote in the sections: a `---` outside a fence and outside every `<details>`, with the blank line after it. The footer is the line before the note, and the divider before it goes with it. Rationale: the prose count bounds the author's text, so no section heading is matched by name, and the result is byte for byte what the v0.13.0 reader produces.
- **An edited round.** Decision: a collapsed round keeps its block, and its anchor is written again from its fields as found with a checksum over the block. The top round is collapsed whole: its text is quoted, its summary line comes from the anchor, and its marker follows the quote. Rationale: owner call 1. Writing the checksum again names the edit once. The anchor MUST still parse, since the summary line and the sequence check need it.
- **Legacy migration.** Decision: move `ReadSticky` and every helper only it uses into `sticky_legacy.go`, renamed `readLegacy` and otherwise unchanged. A new `ReadSticky` sends a body with no anchor to it and anchors the result: `n` from each block's place, `commit` as `PreviousRound` reads it today, and a checksum over the block. Rationale: FR-009. The legacy file gains no branch, and `PreviousRound` stays there as the one reader of an unanchored block's commit.
- **Confirmation.** Decision: `render.AnchorsAsNotes` replaces each anchor on a structural line with `<!-- loupe-round N -->`, and `displayBody` applies it. The confirmation's inline message slot cuts the body after this display step. `publish.Preview` gains `Edited`, the numbers of the rounds carried as edited, shown in the edit notice. Unattended, `publish.Run` writes one stderr line naming them. Rationale: the top anchor holds the prose count and a checksum over the message, so the raw bytes before the slot change as the human types.
- **Render probe.** Decision: the top anchor at line 1. Rationale: the probe in the spec's Assumptions saw nothing above the chips in `body_html`, `body_text` and `gh pr view`. The fallback, the anchor after the footer, is not needed.

## Constitution Check

- **I. A tool for agents.** PASS. No new command or flag.
- **II. Nothing posts unread under a human's name.** PASS. The attended confirmation still shows the whole body, and now also names each round carried as edited.
- **III. Local files, no service.** PASS. Not in scope.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. Every refusal is `sticky` with the existing fix. The `--json` result is unchanged. The unattended notice goes to stderr.
- **VI. Simplicity over ceremony.** DEPARTS. A checksum where the constitution prefers a version number. Justified below.
- **VII. Verified means ran.** PASS. Tests use fixtures, bodies composed in memory and the fake GitHub.
- **Boundaries.** The format contract changes through this spec.

Re-checked after design: unchanged.

## Project Structure

```text
internal/render/anchor.go          # anchor writing, parsing, checksum; the anchored reader
internal/render/sticky.go          # StickyInput, Round, ReadSticky's dispatch, shared helpers
internal/render/sticky_legacy.go   # the v0.13.0 reader, moved unchanged
internal/render/body.go            # anchors in Body; chips from counts
internal/publish/envelope.go       # Earlier as rounds; the compare link from the newest round
internal/publish/publish.go        # Preview.Edited; the unattended stderr line
internal/tui/confirm.go, plain.go  # anchors shown as round numbers; edited rounds named
testdata/sticky-v0.13.0-layout.md  # the v0.13.0 layout with 032's note
testdata/golden/sticky.md, sticky-three.md
docs/comment-format.md             # Sticky reviews section only
docs/github-facts.md               # the render probe
```

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| :--- | :--- | :--- |
| Principle VI prefers a version number to a hash; each anchor carries a SHA-256 checksum over its round | Only a checksum tells a body edited on GitHub from loupe's own bytes. The fields that locate the prose, footer and note are only true of the bytes loupe wrote, so a round without a match is carried whole instead of cut by fields that no longer fit it | A version number says which layout wrote the round, not whether a person changed it since. Without the check, an edit that moves a line would make the fields cut the round in the wrong place. The findings record set the precedent: its checksum exists for the same reason |
| The legacy reader stays beside the new one | A series started before this release MUST continue after it | Refusing an unanchored body would end every series in flight. The frozen file is read at most once per series |
