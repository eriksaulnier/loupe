# Feature Specification: Review visual design

**Feature Branch**: `eriksaulnier/038-review-visual-design`

**Created**: 2026-10-26

**Status**: Implemented in pull request #87, after the Bubble Tea v2 and lipgloss v2 migration. The demo feedback in the [layout contract](contracts/layout.md) amends this spec where the two differ.

**Input**: Owner brief for spec 038. Five mock rounds chose direction B, titled panes, with the lavender accent. The final round establishes the one-row header, lens badge, compact progress bar, expanded summary, findings table, nested diff pane, and grouped footer. This change specifies that design.

## Relationship to earlier specifications

This feature refines the interactive appearance from `specs/002-review-ux`, while preserving the 60x12 minimum and resize behavior from `specs/036-review-pane-guards`. It does not amend the machine output in [the CLI contract](../001-loupe-v1/contracts/cli.md), [the published format](../../docs/comment-format.md), run schemas, review order, publication gates, or decision semantics. The standalone interactive publish confirmation shares the new appearance. CLI text outside the interactive views stays unchanged.

The only new binding is `o` in list and detail. The only change outside the interactive appearance is future guidance in step 5 of `plugin/skills/human-review/SKILL.md`. Implementation MUST wait for the v2 migration. The [plan](plan.md) uses the v2 APIs.

## User Scenarios & Testing

### User Story 1 — Scan and track the review (Priority: P1)

The human sees the summary, findings, and decision progress together. The list remains the primary view.

**Why this priority**: Reviewing starts with orientation and returns to the same list after a decision.

**Independent Test**: Load a mixed round with pending, accepted, excluded, withdrawn, stale, and open-note findings. Compare the list against the linked frames and derived counts.

**Acceptance Scenarios**:

1. **Given** a seven-finding round with two accepted, one excluded, one withdrawn, and three pending findings, **when** the list opens, **then** it shows `3/6 decided`, seven rows, and `1 of 7` for the first selection.
2. **Given** a current accepted finding, **when** its revision changes, **then** it returns to pending and the decided count falls by one.
3. **Given** an open note, **when** its agent reply arrives, **then** the note remains open and its finding retains the sent-back progress color until the note closes.
4. **Given** an expanded summary, **when** the human presses `tab`, **then** it collapses without changing the finding selection or decisions.
5. **Given** 31 counted findings, **when** the list appears, **then** the progress bar uses fixed-width state runs and keeps the exact `N/M decided` count.

### User Story 2 — Read evidence and decide (Priority: P1)

The human opens a finding, reads its evidence, and uses the existing decision, edit, and send-back controls.

**Why this priority**: The layout must help the human assess the cited code without changing the decision process.

**Independent Test**: Open located and general findings, scroll their evidence, edit metadata, and send a note. Verify that resize and polling preserve text, focus, and position.

**Acceptance Scenarios**:

1. **Given** a located finding, **when** it opens, **then** its pane names the id and disposition and shows severity, blocking, label, confidence, verification, and a nested code location.
2. **Given** an added cited line, **when** it appears, **then** a yellow citation tint and left bar remain visible over muted syntax colors. The `+` sign remains visible.
3. **Given** a general finding or an unknown language, **when** detail opens, **then** it remains readable without a fabricated diff or invented syntax tokens.
4. **Given** an open note and reply, **when** the human scrolls to the thread, **then** its state, author, text, and existing resolve/dismiss keys remain available.
5. **Given** an accepted decision, **when** the interface returns to the list, **then** a brief confirmation appears in its bottom-left border without moving the rows.

### User Story 3 — Use help and browser access (Priority: P2)

The human consults help without losing the list and opens the pull request when the terminal cannot follow a hyperlink.

**Why this priority**: Discoverability must survive terminals with different link and color support.

**Independent Test**: Open and close help from each supported context. Inject successful, missing, and failing browser openers without launching a browser.

**Acceptance Scenarios**:

1. **Given** the list, **when** `?` opens help, **then** a titled overlay covers a dimmed list and captures its keys. Closing it restores the same selection and scroll position.
2. **Given** list or detail, **when** `o` succeeds, **then** it opens the pull request and leaves the review running on the same view.
3. **Given** no opener or an opener error, **when** `o` is pressed, **then** the interface shows the failure and the full safe PR URL for manual copying. It does not silently ignore the key or quit.

### User Story 4 — Preview and publish with the existing safeguards (Priority: P2)

The human moves through Action, Inline comments, and Message, then sees the published review link.

**Why this priority**: Appearance must preserve the exact preview and confirmation rules.

**Independent Test**: Complete and cancel the flow against the fake GitHub, including a typed message, a stale head, sticky replacement, and payload inspection.

**Acceptance Scenarios**:

1. **Given** a ready draft, **when** `p` starts the flow, **then** the active tab is Action. Advancing selects Inline comments and then Message.
2. **Given** the Message step, **when** the human types `y` or `p`, **then** the characters enter the message. They MUST NOT publish while the input has focus.
3. **Given** a blurred message input, **when** the human confirms using an existing confirmation key, **then** the exact displayed preview is sent through the existing publication path.
4. **Given** publication succeeded, **when** the published screen appears, **then** the review URL and existing opener/quit behavior remain available.

### User Story 5 — Review within terminal limits (Priority: P1)

The human can review in dark, light, 16-color, monochrome, and narrow terminals without hidden state.

**Why this priority**: Color and extra space cannot be prerequisites for a correct decision.

**Independent Test**: Exercise the width and height boundaries in the layout contract. Compare both reference sizes in dark and `NO_COLOR` modes.

**Acceptance Scenarios**:

1. **Given** `NO_COLOR` and an unready draft, **when** the footer appears, **then** it says `p publish (not ready)`.
2. **Given** a 60x12 terminal, **when** the list opens, **then** its compact summary and findings remain usable with at least one finding row and all footer hints reachable.
3. **Given** a terminal smaller than 60x12, **when** a resize arrives, **then** the too-small screen replaces the view. Restoring the size restores its state and unsent text.
4. **Given** a 16-color terminal, **when** two states share a color, **then** their glyphs, labels, and counts still distinguish them.

### Edge Cases

Empty rounds show `0/0 decided`, no bar, `No findings`, and `0 of 0`. All-withdrawn rounds also show `0/0 decided`, while retaining withdrawn rows for inspection. Readiness still comes from the existing domain rules, including open notes. Full progress is not a promise that publication is available.

Long PR references, paths, titles, summaries, Unicode graphemes, large ids, and large counts MUST fit by display-cell width. Displayed external text MUST remain sanitized. Polling MUST NOT reset an editor, cursor, viewport, or help position. An older timer MUST NOT erase a newer notice.

## Requirements

### Functional Requirements

- **FR-001 — Scope and identity**: The interface MUST use titled panes and the lavender accent (`#b4befe` dark, `#7287fd` light). It MUST retain the primary findings list. It MUST NOT add a card-at-a-time view or imitate sift. The loupe badge, framed list, and chosen accent establish the identity. There is no requirement to inspect or reproduce sift.
- **FR-002 — Header**: A single `surface0` row MUST show ` ⦿ loupe ` with base-colored bold text on accent, the underlined `owner/repo#N` PR link, dim round, and PR title. The title MUST truncate first. Progress and bold `N/M decided` MUST sit at the right. The version MUST NOT appear. `⦿` is the Unicode default tested in DejaVu Sans Mono. The Nerd Font tier MAY substitute its magnifier. The ASCII tier MUST use `loupe` without the lens glyph.
- **FR-003 — Count semantics**: A finding is decided only when its current derived disposition is accepted or excluded. The denominator is accepted + excluded + pending. Derived withdrawn findings MUST appear in neither count nor bar. A stale decision MUST follow the existing disposition rule, which makes an included finding pending. A current exclusion on an unincluded finding remains excluded and counts. The implementation MUST use the domain derivation, not infer state from `included` alone. List positions count every row, including withdrawn rows.
- **FR-004 — Progress**: Up to 30 counted findings MUST produce one contiguous `▮` each in list order, omitting withdrawn rows. Accepted is green, excluded `overlay1`, a pending finding with any open note yellow, pending blocking dim red, and other pending `surface1`. Accepted and excluded take precedence over the note color. A note reply alone MUST NOT change yellow. Above 30 findings, a 20-cell bar MUST group runs in that same state order. Nonempty states MUST get at least one cell, with remaining cells apportioned by counts and stable largest-remainder ties. `●` MUST NOT appear in a progress bar. Narrow headers MAY omit the bar under the layout rules, but MUST retain the exact count.
- **FR-005 — Summary**: The Summary pane MUST expand by default and preserve bullet structure. `tab` MUST toggle it. Its bottom border MUST say `tab collapses` or `tab expands`, or `collapsed to fit` when the window, not the human, collapsed it. Step 5 of the human-review skill MUST request at most four short bullets covering what changed, what blocks, and what was not checked. This guidance MUST NOT reject, truncate in storage, or rewrite existing summaries. In attended review the summary MUST NOT become the published opening prose. Existing unattended summary behavior is unchanged.
- **FR-006 — Findings**: Columns MUST appear as state glyph, id, severity pill, confidence scale, title, and location, with a separate selection marker. Label moves to detail. The top border MUST contain blocking, open-note, and pending counts. Blocking counts included, nonexcluded blocking findings, including accepted ones. Open notes count all open note records, including multiple notes on one finding. Pending is the derived pending count. The bottom-right border MUST show `N of M`. A right-border thumb MUST appear only on overflow and track the visible finding rows. The selected row MUST stay visible.
- **FR-007 — Confidence and pills**: High, medium, and low confidence MUST render as `▰▰▰`, `▰▰▱`, and `▰▱▱`. `NO_COLOR` MUST use `■■■`, `■■□`, and `■□□`. Missing confidence or severity MUST remain blank in the list and MUST NOT imply a rating. Severity pills MUST have uppercase labels with tinted fill and the red/orange/yellow/gray hues from `assets/review/v1/*.svg`. Accepted, excluded, and withdrawn rows MUST recede visually without hiding selection, state, or blocking markers.
- **FR-008 — Detail**: The main pane MUST be titled with the finding id and disposition. It MUST show severity, `● blocking` when applicable, label, confidence, and verification status. The nested location pane MUST put the path in its border, brighten the filename above the directory, and show the displayed hunk's added/removed line counts at the right. A general finding MUST say `general` and omit the location pane. The outer pane MUST scroll through all evidence, impact, fix, references, notes, and replies with a border thumb. The whole-file diff MUST use the same code and pane treatment.
- **FR-009 — Code**: Chroma, already present through glamour, MUST provide syntax tokens. The palette MUST be less saturated than Chroma's Catppuccin colors, as specified in the [layout contract](contracts/layout.md). Most text MUST stay neutral. Added lines MUST have a faint green tint. Cited lines MUST override that tint with yellow and retain a left citation bar. Line numbers and `+`/`-` signs MUST remain visible. Unknown languages MUST use plain code. `NO_COLOR` MUST retain citation and diff markers without ANSI styling.
- **FR-010 — Notices and editors**: A successful decision MUST appear briefly in the bottom-left border, such as `✓ f-001 accepted`. The notice MUST expire after two seconds. It MUST NOT consume a separate notice row. A longer refusal or error MUST remain readable in the active pane, wrap without losing its corrective text, and persist until the next relevant action. Send-back input, note threads, and metadata editing MUST use titled panes without changing their existing keys or saved-state behavior.
- **FR-011 — Help**: Help MUST be an opaque overlay pane composited with lipgloss v2 layers over the dimmed list. Opening help from another supported view MUST preserve that view and use its existing help content. Closing help MUST return to that view. The overlay MUST scroll at small heights. Under `NO_COLOR`, a solid border and `Help` title MUST distinguish it without dimming. Input MUST NOT affect the covered view.
- **FR-012 — Footer and keys**: Hints MUST be grouped in decide, move, and app order, with a dim `│` between groups and the key in accent. Empty groups MUST be omitted. Hints MUST describe only bindings available in the current context. Unavailable actions MUST be dim and have an explicit unavailable reason under `NO_COLOR`, including `p publish (not ready)`. Existing bindings and aliases MUST remain unchanged. The list MUST NOT gain decision shortcuts merely because the mocks show them. The new `o` binding is limited to list and detail, outside help and text editors.
- **FR-013 — Browser**: The header MUST link to the captured pull request using a safe OSC 8 link where styling is enabled. `o` MUST reuse the existing injected opener, URL validation, asynchronous invocation, and platform fallback. In list/detail it MUST preserve the session on success and failure. An absent opener, invalid URL, or failed invocation MUST show a persistent explanation. A valid URL MUST remain fully readable and copyable, including on narrow terminals. The unavailable footer MUST say `o open PR (unavailable)` under `NO_COLOR`. The published screen MUST retain its existing review-URL target and quit semantics.
- **FR-014 — Publish**: Action, Inline comments, and Message MUST appear as steps in the Action and Inline comments popup, not as new keyboard targets; the Message screen shows none (amended after the second demo). Action and Inline comments MUST be a popup over the dimmed list, with the tabs as its first row, whose border says nothing has been sent; the footer says what the next step does. The Message step stays a full screen (amended after the first demo, see `contracts/layout.md`). The final Message step MUST show the human input within the complete existing preview, including warnings, inline choices, sticky replacement body, and payload view. The preview opens readable, and `v` steps to the exact Markdown and then the payload (amended after the first demo). The exact publication gates, focus rules, confirmation keys, cancellation, and sending behavior MUST remain unchanged. The published screen MUST use the common shell and a titled Published pane.
- **FR-015 — Space and fallback**: Each pane MUST cost two rows and four columns, including its borders and one padding column on each side. The minimum MUST remain 60x12. Width thresholds, row budgets, summary collapse, overflow, monochrome, ASCII, and 16-color rules MUST follow [contracts/layout.md](contracts/layout.md). The too-small view MUST retain the existing key guard and resize restoration.
- **FR-016 — Ownership**: Every appearance decision MUST live in `internal/style`. The plan MUST map the current frame helpers, Box, Header, Footer, Rule, and On to the new primitives. The TUI MUST supply content and state, not colors, border geometry, glyph choice, or ANSI escape assembly.
- **FR-017 — Reference frames**: [The frame index](frames/README.md) MUST provide list, detail, notes, send-back, all three publish steps, published, help, and too-small views at 100x30 and 80x24 in dark and `NO_COLOR`. Supplemental edit and file-diff frames MUST protect the existing surfaces. Too-small frames at reference sizes are presentation studies, not activation examples. Runtime activation is tested below 60x12.
- **FR-018 — Verification**: Implementation MUST check rendered text and geometry in `internal/tui`, style units, existing CLI goldens, and the complete `mise run check`. It MUST run and inspect `mise run screenshots` after the implementation. Product screenshots and goldens MUST NOT be regenerated during this specification-only change. The published-format screenshot command MUST NOT run for this work.

### Key Entities

- **Derived progress**: Ordered finding dispositions and open notes determine exact counts and display segments. This is transient display data.
- **Pane**: An outer rectangle, an inner content rectangle, border titles, optional notice, and optional scroll thumb.
- **Review session**: Existing selection, summary preference, viewport positions, editor focus, typed text, and publication state survive appearance changes and resize.

## Success Criteria

- **SC-001**: Every required view has a readable reference frame at both reference sizes, in dark and monochrome, with no lost border or hidden required control.
- **SC-002**: Mixed, empty, stale, withdrawn, 30-finding, and 31-finding rounds show exact progress and independent readiness without ambiguous state.
- **SC-003**: At the minimum size, the human can select and open a finding, read its evidence, author a note, inspect a publication, and cancel safely.
- **SC-004**: All existing review and confirmation keys keep their effects. `o` works or explains its failure without ending list/detail review.
- **SC-005**: Monochrome users can identify selection, disposition, severity, confidence, blocking, citation, and unavailable actions without color.
- **SC-006**: The CLI envelopes, refusal codes, exit codes, published review bodies, and persisted run schemas remain unchanged.

## Clarifications

### Settled by the owner, 2026-10-26

The owner accepted all seven decisions recorded in `.type40/report.md`.

1. **List shortcuts**: The list MUST NOT gain the mock's `a/x/s` shortcuts. Detail retains those controls. The only new binding is `o` in list and detail.
2. **Progress aggregation**: The aggregate threshold MUST be exactly 31 counted findings, with a 20-cell bar. Nonempty states MUST receive one cell before proportional allocation. The exact decided count MUST remain visible.
3. **Legacy summaries**: The interface MUST NOT add summary navigation keys. Long summaries MUST retain their full stored content and show an explicit clipping indicator. The complete summary remains accessible through `loupe show --run <ref>`.
4. **Note boxes**: “Note box” means the existing note/reply thread. “Send back” means the existing composer. The interface MUST NOT add a second note command or composer.
5. **Opener failure**: Missing capability or opener failure MUST leave list/detail running and show a persistent explanation with the full safe URL. The published screen MUST retain its existing quit behavior.
6. **Success notices**: A successful decision notice MUST expire after two seconds. Failures MUST NOT expire. A generation token MUST protect newer notices from an older timer.
7. **Location stats**: The location stat MUST count the displayed hunk. The whole-file view MUST count its displayed file diff.
