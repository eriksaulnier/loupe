# Feature Specification: Review interface UX refresh

**Feature Branch**: `002-review-ux`

**Created**: 2026-09-14

**Status**: Ready for implementation

**Input**: The review interface should be less visually heavy, use conventional keyboard navigation, feel at home inside a Herdr pane, preserve optional Nerd Font icons, and remain ready for a future Herdr integration that opens `loupe review` in a split.

## Relationship to loupe v1

This specification amends the presentation and keyboard choices under “Terminal review interface” in `specs/001-loupe-v1/research.md` and User Story 2 in `specs/001-loupe-v1/spec.md` where they conflict.

The constitution, CLI contract, draft model, publication state machine, published comment format, per-finding human sign-off, stale-version protection, immediate persistence, plain-mode fallback, locale fallback, and `NO_COLOR` behavior remain authoritative and unchanged.

The implementation MUST update the amended passages in `specs/001-loupe-v1/research.md` so the repository does not retain two conflicting interface descriptions.

The one CLI-contract line this specification amends is the `LOUPE_ICONS` row in `specs/001-loupe-v1/contracts/cli.md`: its default under a UTF-8 locale becomes `unicode`.

## 1. Problem and goals

The current interface combines a Powerline header, a colored brand segment, a readiness pill, many Nerd Font icons, full-width selection bands, pane-like separators, and dense shortcut footers.

Those elements make Loupe resemble tmux or a shell status line, especially when Loupe already runs inside Herdr and inherits Herdr’s workspace, tab, pane, border, and agent chrome.

The refresh MUST make the review process easier to learn, reduce visual competition, work well in the narrower area of a Herdr split, and preserve fast keyboard operation for experienced users.

### Goals

- Arrow keys MUST be the advertised navigation controls.
- Existing Vim-style navigation SHOULD remain as compatibility aliases where it does not conflict with text entry.
- Each screen MUST show only the actions that are useful in its current state.
- The default visual treatment MUST use terminal-native spacing and a flat hierarchy rather than Powerline segments or nested dashboard chrome.
- Catppuccin-compatible colors and optional Nerd Font icons MUST remain available.
- The finding detail MUST have one scroll model.
- The interface MUST remain usable from 60 columns upward and MUST remain fully usable outside Herdr.
- The design MUST allow a future Herdr integration to launch `loupe review <run>` in a split without requiring a second Loupe interface.

## 2. Non-goals

- This change MUST NOT add a Herdr dependency, call the Herdr API, inspect `HERDR_ENV`, or add a Herdr-only execution path.
- This change MUST NOT open, resize, focus, or close Herdr panes.
- This change MUST NOT alter the draft schema, finding dispositions, note lifecycle, publication gates, confirmation semantics, GitHub request, or published Markdown.
- This change MUST NOT add mouse interaction, syntax highlighting, filtering, sorting, batch decisions, or a browser interface.
- This change MUST NOT add a theme configuration system.
- This change MUST NOT remove plain mode, ASCII output, `NO_COLOR`, or explicit Nerd Font support.

## 3. Visual system

### 3.1 Overall direction

Loupe MUST use Herdr’s general visual grammar without copying Herdr’s outer application chrome: terminal background, compact rows, restrained borders, one navigation accent, muted secondary text, and semantic status colors.

Loupe MUST NOT render Powerline divider glyphs or visually separate the brand, repository, title, and status into colored header segments.

The terminal background SHOULD remain visible through the main content area.

Borders MUST be reserved for content that needs a true boundary; ordinary screens and publish-choice steps MUST NOT be placed inside decorative boxes.

### 3.2 Color roles

The existing adaptive Catppuccin Mocha and Latte values MAY remain, but the visible roles MUST be reduced to:

- normal terminal text for primary content;
- a muted neutral for metadata and secondary hints;
- one accent for navigation, selected objects, links, and keys;
- green, yellow, and red only for success, pending or warning, and blocking or error states.

The brand and cursor MUST NOT introduce additional competing accent colors.

Color MUST NOT be the only carrier of disposition, blocking state, selection, readiness, or errors.

### 3.3 Glyph tiers

`LOUPE_ICONS=ascii`, `LOUPE_ICONS=unicode`, and `LOUPE_ICONS=nerd` MUST remain supported.

With a UTF-8 locale and no explicit `LOUPE_ICONS`, Loupe MUST select the Unicode tier.

An explicit `LOUPE_ICONS=nerd` MUST retain Nerd Font icons for meaningful objects and states such as pull requests, files, notes, blocking findings, and publication.

The Nerd tier MUST NOT enable Powerline header separators.

A non-UTF-8 locale MUST continue to force ASCII regardless of `LOUPE_ICONS`.

`NO_COLOR` MUST continue to remove escape sequences without changing the selected glyph tier except where the locale requires ASCII.

Decorative icons SHOULD be removed where the adjacent word already communicates the same meaning and the icon does not improve scanning.

### 3.4 Header

Every full-screen view MUST use one flat header line with, in priority order, the current object or step, the run reference when useful, and concise readiness or position information.

The header MAY use one subtle background across its complete width, but MUST NOT use separately colored segments, angled joints, or a high-contrast brand block.

A representative list header is:

```text
loupe · acme/widgets#42 · round 1                            2 pending
```

A representative detail header is:

```text
f-002 · 2 of 7                                              4 pending
```

When space is constrained, the pull-request title MUST truncate first, followed by redundant repository context; the current finding identifier, current position, and actionable readiness state MUST remain visible.

The detailed accepted, pending, excluded, withdrawn, and open-note counts MUST appear once on the list rather than being duplicated by a large `READY` or `NOT READY` pill.

While review is incomplete, the concise header state SHOULD name the actionable remainder, such as `2 pending` or `1 open note`.

When publication becomes available, the concise header state MUST read `ready to publish`.

### 3.5 Selection, headings, and metadata

A selected row MUST remain identifiable without color through its cursor glyph.

With color, selection SHOULD use the accent and either bold text or a restrained background; it MUST NOT become a visually dominant full-width banner.

Section headings MUST use sentence case rather than all-uppercase dashboard labels.

A finding’s disposition, blocking state, label, confidence, and severity MUST remain readable as text even when their icons are absent.

The interface SHOULD use one disposition glyph and at most one explicit blocking marker in a list row; file, label, and general-finding icons SHOULD be omitted when their text is already clear.

A list row MUST also mark a finding with an open note using the note glyph, in the note color once the agent has replied to it and dim while it has not, so the counts line's open notes can be found without opening each finding.

## 4. Interaction model

### 4.1 Global keys

The advertised global controls MUST be:

| Key | Action |
| :--- | :--- |
| `Esc` | Return to the preceding screen or cancel the active input |
| `?` | Open or close contextual help |
| `q` | Quit from the list, finding detail, or file diff; recorded decisions remain saved |
| `Ctrl+C` | Quit where the publication safety rules permit it |

Text input and final publication confirmation MUST retain their existing safety-specific key handling.

### 4.2 List keys

| Primary key | Compatibility alias | Action |
| :--- | :--- | :--- |
| `↑` | `k` | Select the previous finding |
| `↓` | `j` | Select the next finding |
| `Enter` | none | Open the selected finding |
| `Tab` | none | Expand or collapse the summary |
| `p` | none | Begin publication when the draft is ready |

On initial open or reload, the selected row SHOULD be the first pending finding, then the first finding with an open note, then the first finding when neither exists.

### 4.3 Finding-detail keys

| Primary key | Compatibility alias | Action |
| :--- | :--- | :--- |
| `←` | `N` | Open the previous finding |
| `→` | `n` | Open the next finding |
| `↑` | `k` | Scroll the finding document up |
| `↓` | `j` | Scroll the finding document down |
| `Page Up` | none | Scroll one page up |
| `Page Down` or `Space` | none | Scroll one page down |
| `a` | none | Accept the finding as shown |
| `x` | none | Exclude the finding from the review |
| `s` | none | Send the finding back with a note |
| `u` | none | Restore an excluded finding to pending; reinstate a withdrawn one, which includes and accepts it (specs/011-reinstate-withdrawn) |
| `r` | none | Resolve the finding’s open note |
| `d` | none | Dismiss the finding’s open note |
| `f` | none | Open the whole-file diff for a located finding |

`J` and `K` MUST no longer control a separate hunk scroll region.

A successful accept, exclude, or send-back MUST continue to advance to the next finding when one exists.

When advancement will occur, the visible action wording SHOULD communicate it, for example `accept + next`.

The existing delay that prevents typeahead from deciding an unseen finding MUST remain in effect for every decision key.

### 4.4 File-diff keys

| Primary key | Compatibility alias | Action |
| :--- | :--- | :--- |
| `↑` | `k` | Move to the previous diff line |
| `↓` | `j` | Move to the next diff line |
| `←` | `[` | Jump to the previous finding marker |
| `→` | `]` | Jump to the next finding marker |
| `Enter` | none | Open the finding marked on the current line |
| `Esc` | none | Return to the originating finding |

The file diff MUST continue to mark every finding on the file and keep the active line visible while navigating.

### 4.5 Contextual footers

The persistent footer MUST advertise primary arrow navigation, currently available decisions, and contextual help.

The footer MUST NOT advertise `restore` unless the finding is excluded. (Amended 2026-09-17 by `specs/011-reinstate-withdrawn` FR-006: it MUST NOT advertise `reinstate` unless the finding is withdrawn, which is the only other meaning `u` has.)

The footer MUST NOT advertise note resolution or dismissal unless the finding has an open note.

The footer MUST NOT advertise `file` for a general finding.

The list footer MUST show `p publish` at all times; while the draft is not ready it MUST read `publish (not ready)`, dimmed, and leave naming the blockers to the header.

Compatibility aliases MUST appear in help but SHOULD NOT consume space in the persistent footer.

When all hints do not fit on one line, the footer MUST drop the `+ next` suffixes and tighten its gaps, then wrap onto a second line between hints, before it gives up any hint. Only when two lines cannot hold every hint does it give way, and then it MUST preserve currently available decision actions and `? help` before optional navigation prose. Every body MUST be sized against its own view's footer, so a wrapped footer takes a row from the body rather than hiding the body's last row; help and the note and edit rows, which open and close without a layout, MUST NOT change that size. The confirmation keeps a one-line footer and moves its cancel sentence to the notice line instead, since that line is reserved anyway. Plain mode's answer legend wraps the same way. (Amended 2026-09-15: a review split beside an agent pane is about 71 columns wide, where a one-line footer dropped `q quit` and arrow navigation.)

Representative pending-finding footer:

```text
←/→ finding   ↑/↓ scroll   a accept   x exclude   s send back   f file   ? help
```

Representative excluded-finding footer:

```text
←/→ finding   ↑/↓ scroll   u restore   ? help
```

## 5. Screen specifications

### 5.1 Finding list

The list MUST retain the draft summary, readiness counts, finding disposition, identifier, blocking state, title, and location.

The selected row MUST remain visible while navigating.

At narrow widths, columns MUST disappear in this order: label, repository directories within the location, then location.

The title MUST retain at least 20 cells whenever the full-screen mode remains active.

The summary MUST remain collapsed by default and MUST continue to identify `Tab` as its expand or collapse control even when the footer omits that hint.

The list SHOULD avoid blank rows that do not separate meaningful regions.

### 5.2 Finding detail

The finding detail MUST be one scrollable document rather than a body viewport plus a separately scrollable hunk region.

For a located finding, the document order MUST be:

1. title and concise state metadata;
2. location and anchored diff hunk;
3. finding body;
4. suggested fix when present;
5. notes and replies when present.

For a general finding, the document MUST identify it as general and omit the hunk region without leaving an empty separator.

The anchored diff lines MUST remain visibly marked without relying on color.

Long anchored ranges MUST remain fully reachable through the document’s normal scrolling rather than a second set of controls.

The document MUST preserve the existing escaping of terminal controls and bidirectional characters and the existing Markdown rendering behavior.

### 5.3 File diff

The file diff MUST retain added and removed styling, line numbers, finding markers, the current-line cursor, and concise file statistics.

The header MUST prioritize the file name, active finding context, and marker count over the full repository path.

The full path MAY truncate from the left so the file name remains visible.

### 5.4 Publish choices

The action and inline-comment choices MUST render as ordinary stepped screens rather than centered bordered boxes.

Each step MUST name its place in the flow, such as `Publish · Step 1 of 3` and `Publish · Step 2 of 3`.

Each option MUST retain its explanatory sentence.

A disabled action MUST remain visible and MUST state why it is unavailable.

The choice screens MUST use `↑` and `↓` as their advertised controls and retain `j` and `k` as aliases.

### 5.5 Final confirmation

The final confirmation MUST remain visually distinct because it is the only screen that can publish.

Only `y` and `p` MUST publish, and neither MAY publish while the message input holds the keyboard. `p` MUST publish only when the confirmation has a message input, because that screen opens with the input focused, so a `p` pressed by reflex is typed, not sent. Without an input the screen opens on its own keys, where a reflexive `p` would publish before anything was read, so there `p` MUST do nothing, neither publishing nor canceling. The footer MUST name the key as `y/p` where `p` publishes and as `y` where it does not. Scrolling and payload-toggle keys MUST retain their current behavior, and every other answer MUST continue to cancel without sending.

The confirmation MUST continue to show the review as it will read, inline comments, moved-head information when applicable, and the exact request payload through the existing toggle.

The flat header and reduced color roles MUST apply, but visual simplification MUST NOT weaken the confirmation wording or publication safety.

### 5.6 Help

Help MUST lead with the current screen’s primary controls.

Arrow keys MUST be listed before compatibility aliases.

Compatibility aliases MUST be identified as aliases rather than presented as parallel primary controls.

Help MUST use one column when two columns would make descriptions wrap or truncate and MAY use two columns when both fit comfortably.

The explanation of stale-version protection MUST remain present.

### 5.7 Settled during implementation

These follow the agreed mockup and stay within the intent of the sections above.

- A dim horizontal rule the width of the window closes the header block of every full-screen view except the final confirmation, whose review opens with titled rules of its own. It takes the place of the blank line under the header where a view had one; the list and the file diff give up one row for it.
- Finding identifiers stay visible but quiet: list ids are dim, and a selected row bolds its title in the accent rather than its id. The detail header shows the id dim beside its position.
- The file diff marks a line carrying a finding with a one-cell glyph (`◆`, ASCII `*`) instead of the identifier, and its header names the finding under the cursor, with `+N` when the line carries more.
- The list's counts line keeps three-space gaps when it fits; narrower, zero counts drop and the gaps shrink to two; if it still does not fit, it splits over two lines and the header grows with it.
- Help taller than the window scrolls with `↑`, `↓`, `Page Up` and `Page Down`; its header then shows `lines a–b of n` and its footer adds `↑/↓ scroll`.
- When `y/p publish this review` (or `y publish this review` without a message input) and `any other key cancels, nothing is sent` do not fit the confirmation footer together, the cancel sentence moves, word for word, to the line above it.
- A disabled publish action wraps its reason under the description column rather than clipping it.
- The final confirmation reads `Publish · Step 3 of 3` inside the review program's publish flow. Opened by `loupe publish`, whose flags replace the two choice steps, it reads `Publish` with no step count.

## 6. Responsive and Herdr behavior

The design MUST assume that Herdr’s sidebar, pane borders, gaps, and scrollbars can reduce the usable pane width.

The full-screen interface MUST remain complete at the existing minimum of 60 columns by 12 rows.

Layouts SHOULD treat 60–79 columns as compact, 80–99 columns as standard, and 100 columns or more as wide.

No rendered line MAY exceed the current terminal width in any glyph tier or color mode.

At compact widths, Loupe MUST remove optional context and hints before truncating a finding identifier, decision action, or actionable status.

Loupe MUST NOT draw an outer border around its full screen because Herdr or the host terminal may already frame the pane.

A future Herdr integration MUST be able to launch the unchanged command `loupe review <run>` in an interactive split and receive the same behavior as a user launching it in any other terminal.

## 7. Requirements and acceptance criteria

### Functional requirements

- **UX-001**: UTF-8 terminals with no icon override MUST use Unicode glyphs rather than Nerd Font glyphs.
- **UX-002**: Explicit Nerd Font mode MUST remain supported without enabling Powerline separators.
- **UX-003**: Every full-screen view MUST use a flat, non-segmented header.
- **UX-004**: Arrow keys MUST be the advertised navigation controls in lists, finding detail, file diff, and publish choices.
- **UX-005**: Existing `j` and `k` movement MUST remain available as aliases outside text input.
- **UX-006**: Existing `n` and `N` finding movement and `]` and `[` marker movement MUST remain available as aliases.
- **UX-007**: Finding detail MUST use one viewport and MUST remove independent `J` and `K` hunk scrolling.
- **UX-008**: The finding footer MUST show only actions valid for the current finding and note state.
- **UX-009**: Successful settling decisions MUST continue to advance while preventing typeahead from deciding an unseen finding.
- **UX-010**: Publish choices MUST use unboxed stepped screens with visible descriptions and refusal reasons.
- **UX-011**: Final confirmation behavior MUST remain unchanged except for presentation and advertised arrow-first scrolling.
- **UX-012**: Full-screen, plain, Unicode, Nerd, ASCII, color, and `NO_COLOR` modes MUST preserve all security escaping and width guarantees.
- **UX-013**: No runtime path MAY depend on Herdr or Herdr environment variables. Superseded on 2026-09-15 by `specs/006-agent-plugins` FR-010: `loupe handoff` runs Herdr from `internal/pane`; `loupe review` still has no Herdr path.

### Acceptance scenarios

1. **Given** an unset `LOUPE_ICONS` under a UTF-8 locale, **When** the list renders, **Then** it uses Unicode status glyphs and contains no private-use or Powerline glyphs.
2. **Given** `LOUPE_ICONS=nerd`, **When** every full-screen view renders, **Then** meaningful Nerd Font icons appear and no Powerline divider appears.
3. **Given** a pending located finding, **When** its detail renders, **Then** the footer shows arrow navigation, accept, exclude, send back, file, and help, but not restore or note-closing actions.
4. **Given** an excluded general finding with no open note, **When** its detail renders, **Then** the footer shows restore and help but does not show accept, file, resolve, or dismiss.
5. **Given** an open note, **When** its finding renders, **Then** resolve and dismiss are advertised.
6. **Given** a long finding body and a long anchored range, **When** the human uses `↑`, `↓`, `Page Up`, and `Page Down`, **Then** every part of the body, hunk, suggested fix, and note thread is reachable through one viewport.
7. **Given** a finding detail, **When** the human presses `←` or `→`, **Then** the preceding or following finding opens with the same boundary behavior as `N` or `n`.
8. **Given** a file diff, **When** the human presses `←` or `→`, **Then** the cursor jumps to the preceding or following finding marker with the same boundary behavior as `[` or `]`.
9. **Given** a successful decision followed immediately by repeated decision input, **When** the next finding has not had time to be displayed, **Then** the repeated decision is dropped and no unseen finding is decided.
10. **Given** a ready review, **When** the human enters publication, **Then** action and inline choices appear as steps without decorative boxes and final confirmation still publishes only on `y`.
11. **Given** widths of 60, 79, 80, 99, 100, and 140 columns, **When** every view renders in each glyph and color tier, **Then** no line exceeds the window and required actions remain visible.
12. **Given** plain mode, a non-UTF-8 locale, or `NO_COLOR`, **When** review runs, **Then** the existing fallback behavior and decision semantics remain unchanged.

### Success criteria

- **SC-UX-001**: A first-time user can select, open, navigate, decide, inspect a file, and return using arrows, Enter, Esc, and the visible decision keys without opening help.
- **SC-UX-002**: No primary visible shortcut changes meaning only because Shift was held.
- **SC-UX-003**: The default interface contains no private-use or Powerline glyphs.
- **SC-UX-004**: No detail footer advertises an action that cannot apply to the displayed finding.
- **SC-UX-005**: Each view has one visually dominant current object and one concise actionable status.
- **SC-UX-006**: All automated repository checks pass after the final implementation edit.

## 8. Implementation guidance

Claude SHOULD implement this as a focused TUI and style change rather than introducing a new UI framework or configuration abstraction.

Expected implementation files include:

- `internal/style/style.go` for glyph defaults, flat headers, reduced visual roles, headings, selection, and removal of Powerline behavior;
- `internal/tui/app.go` for help, shared header and footer behavior, and responsive rendering;
- `internal/tui/list.go` for list density, initial actionable selection, arrow-first hints, and unboxed publish steps;
- `internal/tui/detail.go` for one viewport, reordered finding content, arrow navigation, contextual actions, and removal of hunk scrolling;
- `internal/tui/filediff.go` for left and right marker navigation and the flat file header;
- `internal/tui/confirm.go` for presentation-only alignment while preserving confirmation semantics;
- `internal/tui/plain.go` only where the Unicode default or shared styles change its output;
- `internal/style/style_test.go`, `internal/tui/keys_test.go`, `internal/tui/list_test.go`, `internal/tui/detail_test.go`, `internal/tui/confirm_test.go`, and `internal/tui/tiers_test.go` for changed behavior;
- `specs/001-loupe-v1/research.md` and any pinned task wording that would otherwise contradict this specification.

Implementation MUST follow failing test, implementation, passing check.

Tests MUST verify semantics and rendered structure without introducing a real pseudo-terminal, live network host, live pull request, or publication call outside `internal/publish`.

Golden output MUST be regenerated only when deliberately required, and its diff MUST be read before acceptance.

After the final edit, the implementer MUST run:

```sh
mise run check
```

The implementer MUST report the applicable owner-only checks from the Unverified section of `specs/001-loupe-v1/validation.md` as unverified without restating them.

No commit, push, pull request, release, live review, or Herdr integration is authorized by this specification.
