# Implementation Plan: Finding design language and local header

**Branch**: `052-finding-design-language` | **Date**: 2026-11-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/052-finding-design-language/spec.md`, rules from `docs/tui-design.md`

## Summary

Write the rules down in `docs/tui-design.md`, then make the code follow them. `internal/style` gains one section helper and the local header's location. `internal/tui` draws every finding block through that helper, in one order, with the proposal and the suggested fix sharing a renderer, and caps the location pane. The published review does not move.

## Technical Context

**Language/Version**: Go, as in `go.mod`

**Primary Dependencies**: Bubble Tea v2 and Lip Gloss v2, already present. No new dependency.

**Testing**: `go test ./...` through `mise run check`. TUI tests type keys through `internal/testutil/screen`. Goldens under `testdata/golden/cli` and the `internal/render` goldens.

**Constraints**: PR-run headers and the published review MUST NOT change. `NO_COLOR` and ASCII glyphs MUST keep the structure.

## Constitution Check

- **I. A tool for agents**: no change to commands or the plugin. Pass.
- **II. Nothing posts unread**: the published review is untouched (FR-008). Pass.
- **III. Local files**: no new state. Pass.
- **IV. Never touch the checkout**: the header reads `target.json` only. Pass.
- **V. Machine contract**: no `--json` or refusal change. `loupe list` and `show` keep `local#N`. Pass.
- **VI. Simplicity**: one helper replaces three (`sectionHead`, `threadHead`, `hangFromBar`). No dependency. Pass.
- **VII. Verified means ran**: tests use the screen helper and fakes, no pty. Pass.

## Design

### `internal/style`

- **`Section(tone Tone, heading string, chip string, body []string) []string`**: the bold heading in the tone, an optional chip after it (note status, withdraw code), then each body line behind the tone's `Quote` glyph. A blank body line keeps the bar. Without color the chip is set off by the separator, as `threadHead` does today. This is the only place a finding heading or bar is styled.
- **Pane title helpers**: a `PaneTitle(text string)` and a `PaneTitleKey(text string)` (the top-right key name in the note tone), so `rewriteView` and `confirm.go` stop rendering titles inline.
- **`ReviewHeading.Local`**: a `*LocalPlace` with `Repo`, `Branch` and `Capture`. `ReviewHeader` draws it for a local run in place of the ref, plain, never underlined, never linked. It picks the richest of three forms (full, medium, short) that fits with up to 16 cells left for the title, and the short form drops the bar. A PR heading leaves `Local` nil and draws as today.

### `internal/tui`

- **`refreshDetail`** builds the document in the contract's order: meta, `Withdrawn`, location pane, `Why`, proposed change, `Impact`, `References`, notes. Each block returns `[]string` from `style.Section`.
- **`proposedChange(f)`** replaces `proposal` and `suggestedFix`. The mode supplies the heading (`Suggested fix` or `<applier> will`) and the body lines, and both go through `style.Section` in `ToneGood`. The body keeps today's treatment in each mode: a proposal is rendered Markdown, and a suggested fix is wrapped plain text, so a fix still cannot close a fence. The renderer is shared, and the text handling stays as it is.
- **Question**: when `ui.question(f)`, the slot is `style.Section(ToneNote, "A question for <applier>", …)`. No glyph.
- **Withdrawn**: the reason is a muted section with the code as its chip. While the finding is withdrawn, every other section takes `ToneMuted`.
- **`codePane`** gains a cap of one third of the detail viewport, at least eight rows. A taller hunk shows a window of its code opened two lines above the finding's first line, under the whole anchored heading, with the pane's scrollbar marking the window and `N of M lines · f shows all` in its bottom border. The scroll keys keep moving the document, and `f` is the way to the rest (owner, 2026-11-04).
- **`applyUI.heading`** fills `Local` from the target: `Prefix`'s repo name or `filepath.Base(Root)`; `Branch`; the capture summary from `Capture`, `Files` and `BaseRef`.

### Research notes

- R1: The running program already draws a blank row between the header and the body (the demo capture `pr-finding-a.png` from the design session). The first mocks dropped it. The contract now states it, and the code keeps it.
- R2: The ASCII glyph tier has `Quote: "|"`, so `Section` needs no fallback of its own.
- R3: Capture forms are `changed`, `files` and `no-files` (`internal/local/capture.go`). `changed` without `BaseRef` is a diff against `HEAD`, so its summary is `uncommitted changes`.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/tui-design.md` supersedes part of `specs/038-review-visual-design/contracts/layout.md` | The owner chose a living contract over another spec contract, so the rules have one home that later specs amend |
