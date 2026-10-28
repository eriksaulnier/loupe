# Implementation Plan: Review visual design

**Branch**: `eriksaulnier/038-review-visual-design` | **Date**: 2026-10-26 | **Spec**: [spec.md](spec.md)

**Input**: The owner's chosen titled-pane design, [research](research.md), and [layout contract](contracts/layout.md).

## Summary

Replace the interactive hand-built frame with a style-owned shell and titled panes. Keep the findings list primary. Add the compact progress bar, muted evidence highlighting, layered help, grouped hints, and the PR opener binding. Preserve the existing decision and publication paths.

This change delivers specifications and reference frames only. Implementation MUST wait for the separate migration to Bubble Tea v2, Bubbles v2-compatible components, and lipgloss v2. The migration MUST pass `mise run check` before these tasks begin. There is no task here to perform that migration.

## Technical Context

**Language/Version**: Go, using the version established by the prerequisite migration. Current root baseline is Go 1.25.8. The throwaway mock has its own Go 1.26 module and MUST NOT set the product's version.

**Primary Dependencies**: `charm.land/bubbletea/v2` provides the migrated event loop and `tea.View`. `charm.land/lipgloss/v2` provides styles and layers. The migrated Bubbles components provide viewports and inputs. Existing glamour renders Markdown. Existing Chroma supplies syntax tokens and becomes a direct import because muted per-token styling must preserve line backgrounds. No new runtime module is required beyond that existing graph and the prerequisite migration.

**Storage**: Existing run files only. Progress, layout, notices, and browser requests are session-local display state. No schema bump.

**Testing**: Go tests, the migrated screen/teatest harness, fake GitHub, local Git repositories, injected dimensions and input, CLI goldens, and deliberate demo screenshots. No test launches a browser or uses a pseudo-terminal.

**Target Platform**: Existing terminal platforms, dark/light, true color, ANSI256, ANSI16, monochrome, and existing icon tiers.

**Project Type**: Interactive CLI.

**Performance Goals**: Preserve the existing `internal/tui/perf_test.go` limits. Tokenize the displayed hunk/file when its content or language changes, not on every key or notice tick. Do not introduce a global cache. Theme changes MAY restyle the same tokens.

**Constraints**: 60x12 minimum, complete preview, existing keys plus `o`, no publication or CLI contract changes, no edits to immutable SVGs.

**Scale/Scope**: List, detail, file diff, notes, send-back, metadata editor, help, three publish steps, published result, sending/error state, and too-small state. Progress must handle zero, 30, 31, and larger rounds.

## Constitution Check

| Principle | Before research | After design |
| :--- | :--- | :--- |
| I — Agent-neutral | Pass: no agent invocation | Pass: the skill guidance remains host-neutral |
| II — Human sign-off | Pass: preserve gates | Pass: preview, focused message input, confirmation, and one request remain unchanged |
| III — Local files | Pass: no new storage | Pass: rendering needs no network, browser launch reuses the existing explicit action |
| IV — Checkout safety | Pass: no checkout mutation | Pass: this design changes only loupe's own interface |
| V — Machine contract | Pass: interactive scope | Pass: CLI goldens, schemas, refusals, and published bodies stay unchanged |
| VI — Simplicity | Pass: existing dependencies | Pass: each primitive has at least two concrete consumers, with one existing dependency promoted |
| VII — Verified means ran | Pass: checks planned | Pass: injected tests and fake publication only, screenshot review remains explicit |

## Project Structure

```text
specs/038-review-visual-design/
  spec.md, plan.md, tasks.md
  research.md, data-model.md, quickstart.md
  contracts/layout.md
  checklists/requirements.md
  frames/README.md, *.txt, *.ansi

internal/style/
  style.go                 # keep existing noninteractive styles stable
  review.go                # new interactive palette, metrics, shell and panes
  review_test.go
  code.go, code_test.go     # muted syntax and line backgrounds
internal/tui/
  app.go, list.go, detail.go, filediff.go
  confirm.go, published.go, small.go
  *_test.go
plugin/skills/human-review/SKILL.md
internal/cli/plugin_test.go
```

The paths for new files are planned, not created by this specification. The local mock extension stays under `.type40/mocks/` and MUST NOT be copied into product packages.

## Style primitives and migration map

Names below describe the intended internal API. An implementation MAY adapt signatures to the completed migration, but MUST preserve ownership and the layout contract.

| Existing code | Planned primitive | Consumers and responsibility |
| :--- | :--- | :--- |
| `frame` / `frameWith` in `internal/tui/app.go` | `ReviewLayout` and `ReviewFrame` | Model and standalone ConfirmModel receive measured header, body, footer, and input rectangles. No fixed notice row. |
| `style.Box` | `Pane` with `PaneTitles` | Summary, findings, location, notes, input, preview, help, and published result. Four title slots, exact outer geometry, focus and optional thumb. |
| `Header` / `listHeader` | `ReviewHeader`, `Progress` | List, detail, publish, and published shell. Trusted link span, badge, priority truncation, exact counts and state runs. |
| `Footer` | `KeyGroups` | List, detail, editors, help, and confirmation. Semantic groups, measured wrapping, no-color disabled labels. |
| `Rule` | Pane border titles | Findings counts, detail identity, location path/stat, and bottom position replace loose horizontal rules. Existing noninteractive Rule consumers remain stable. |
| `style.On` | `SurfaceSpan` | Selected table rows and syntax spans apply explicit foreground plus inherited background. Prevent nested SGR resets from erasing selection or citation tints. |
| TUI severity, confidence and path formatting | `SeverityPill`, `ConfidenceScale`, `LocationTitle` | List and detail share tokens and width behavior. Missing values remain absent. |
| Hand-built help content frame | `HelpOverlay` | Contextual help in review and pickers uses v2 composition over a dimmed list. |
| Diff line styling | `CodeLine` and `CodeTheme` | Detail hunks and whole-file diff share token roles, gutters, addition/deletion tint and citation precedence. |

Existing `Style.New`, color detection, and glyph access MUST integrate with the completed migration rather than add a second renderer. Interactive lavender tokens MUST NOT replace global CLI accent values. Existing helpers MAY remain for noninteractive callers. Remove old TUI geometry only after all its consumers use the new primitives.

## Layout and event integration

`Model.View()` and `ConfirmModel.View()` MUST return the migration's `tea.View`, constructed with `tea.NewView(content)`. The migrated event loop owns terminal profile and background messages. These become explicit inputs to `internal/style`, including the stronger nonempty `NO_COLOR` rule. The TUI passes domain content and layout state. It MUST NOT assemble borders, hardcode palette values, or import a syntax theme.

`ReviewLayout` measures the footer first, then applies the layout contract's row budget. The list derives progress from `draft.Ordered`, `draft.Dispositions`, and `draft.ReadinessOf`. It MUST NOT modify domain ordering or readiness. Position remains an index into all displayed rows, including withdrawn rows. Pane scroll metrics derive from actual content/viewport dimensions. Automatic summary collapse is separate from the human's preference.

Detail retains one scrollable document. Nested location and note panes are document blocks, not competing scroll regions. Editors reserve space and keep focus visible. Polling, resize, live replies, and typed text retain the existing behavior. A notice command carries a generation token so an older timer cannot clear newer information.

## Browser and trusted output

Generalize the existing `canOpen` and `browserMsg` path in `internal/tui/published.go` to accept a safe target and request context. Continue calling `Config.OpenURL` asynchronously. The CLI's existing platform handler remains the only opener. List/detail success stays in review. Published success still quits. Missing capability, invalid targets, and invocation failure produce the specified persistent explanation and manual URL when safe. A late callback MUST NOT quit a view or overwrite an editor unrelated to its request.

Sanitize PR metadata, summary, findings, paths, notes, and error text before styling. Keep `style.Link` as the URL validation boundary. Add only the generated OSC 8 hyperlink after sanitization. Clipping MUST preserve grapheme boundaries and close link/style sequences. `NO_COLOR` frames MUST contain no generated escape sequences. This rule applies even when `NO_COLOR=anything` rather than `1`.

## Code highlighting

`internal/style/code.go` owns Chroma lexer selection and muted token-to-style mapping. Match the filename, coalesce tokens, split them by line, and render spans with the final line background. Unknown language uses plain code. If tokenization fails, retain the source and return a diagnostic for a visible highlighting-unavailable notice. Do not silently discard an error or replace the code with a blank block.

The TUI supplies already-parsed hunk lines, old/new line numbers, side, and citation range. Highlighting MUST preserve those associations. It MUST NOT fetch source from GitHub or read a different checkout. For a whole-file view, use the captured diff's available content. Incomplete context can reduce highlighting accuracy, but it MUST NOT change evidence text. The diff stat counts additions/deletions in the displayed hunk, or the full displayed file diff on that screen.

## Publication and skill guidance

Tabs are display elements derived from existing views. `confirm.go` MUST keep its composition closure, sticky preview, payload toggle, warning text, focus routing, and cancellation rules. The Message pane is the existing input at the same place in the composed body. Action/Inline explanations MUST say that no review has been sent. Sending and published views use the same shell without altering their event handling.

After implementation, step 5 of `plugin/skills/human-review/SKILL.md` MUST request at most four short bullets. Those bullets MUST cover what changed, what blocks, and what was not checked. No draft validation, CLI output, or published format changes accompany this wording. The summary remains unpublished for attended review. Existing unattended publication behavior remains unchanged.

## Verification strategy

Run the [quickstart](quickstart.md) after implementation. Test exact counts and retained semantics before altering rendering. Add geometry assertions at 59/60, 79/80, 99/100 columns and 11/12, 23/24, 29/30 rows. Verify true color, light, 16-color, `NO_COLOR=1`, `NO_COLOR=anything`, ASCII, and Unicode. Check long ids, paths, titles, graphemes, and count labels.

Rendered-text tests MUST assert visible state and controls, not only ANSI snapshots. Style tests MUST inspect token color/background continuity, no escapes under `NO_COLOR`, and stable border/scroll dimensions. Existing CLI goldens MUST remain unchanged. If a golden changes unexpectedly, correct the shared-style leak rather than accept it. Run the complete repository check before committing implementation.

Run `mise run screenshots` after product implementation and inspect the generated terminal images. A gif can change solely from timing. The maintainer-only items in `specs/001-loupe-v1/validation.md` remain unverified. This spec change MUST NOT run `mise run review-screenshot`, which would post a GitHub review.

## Complexity Tracking

No constitution or external-contract departure. The brief explicitly authorizes specification work before the prerequisite migration. The implementation gate preserves that sequence. The reference frames retain current list keys where the throwaway mock conflicts with the brief. The owner accepted that choice and the long-summary behavior on 2026-10-26. The spec records all seven accepted decisions as settled clarifications.
