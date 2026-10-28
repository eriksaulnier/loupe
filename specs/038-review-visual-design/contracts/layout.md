# Interactive layout contract

This contract supplements [spec.md](../spec.md). It changes no CLI or publication contract. Dimensions are display cells, not bytes or Unicode code points. The reference frames show synthetic content. Dynamic text MUST follow these rules rather than copy the fixture values.

## Shell and row budget

The usable width is the terminal width. The shell has one header row, a body, and the measured footer. There is no permanent notice row. Each pane subtracts two border rows and four columns from its outer rectangle. A nested pane subtracts those costs again. A scrollbar replaces cells in the right border and consumes no content column.

Let `H` be terminal height, `F` the footer's measured row count, and `S` the summary's outer height. The list's finding-row budget is `H - 1 - F - S - G - 2 - C`, where `G` is the gap between summary and findings and `C` is one column-heading row. `G=1` when space permits and `0` on short screens. At 100x30 with four one-line bullets and a one-row footer, `S=6` and 18 findings fit. At 80x24 with the same summary and a two-row footer, 11 findings fit. Wrapping bullets consume their actual rows.

A collapsed summary costs three rows: two borders and one clipped preview. Before removing finding rows, the layout MUST remove the inter-pane gap and then automatically collapse the summary if fewer than three findings would fit. This collapse MUST NOT overwrite the human's expanded/collapsed preference. When height grows, the preference MUST return. Pressing `tab` requests expansion only within the available budget.

At 60x12, a one-row header, three-row footer, three-row summary, and five-row findings pane leave two finding rows after the column heading. If a wider id or hint forces another row, the layout MUST remove the column headings before reducing findings below one. It MUST NOT hide a required action, disposition, id, or selection. Optional footer wording MUST shorten before wrapping. The short-screen footer MUST remain at most three rows at 60 columns.

Expanded summary content MAY use at most `max(1, H - 1 - F - G - 2 - C - 3 - 2)` rows, leaving three finding rows. Four short bullets are the intended input. A legacy summary that exceeds this budget MUST end with an explicit clipped-content indicator. Its bottom border MUST offer the existing command `loupe show --run <ref>` when clipped, instead of pretending every line is visible. `tab` still toggles the pane. Storage MUST remain unchanged. The owner settled this behavior on 2026-10-26 to avoid adding summary navigation keys. The full summary remains available through `loupe show`.

Detail uses one main pane of `H - 1 - F` rows. Its scrollable document includes the title, metadata, nested location, body, impact, suggested fix, references, and notes. The viewport MUST scroll this whole document. A tall nested code block MUST NOT permanently hide the body. Send-back and metadata input panes reserve their measured heights above the footer and reduce the detail viewport. Their cursor MUST stay visible. The send-back input keeps its current one-third-height content cap.

Publish uses one tab row under the header. Its body uses the remaining `H - 2 - F` rows. Action and Inline comments use a choice pane and a Message explanation pane. At short heights the explanation MUST become a wrapped paragraph inside the choice pane, removing one pane's cost. Message uses a scrollable complete preview with the existing input embedded at its true position. It MUST preserve sticky history and warning visibility through scrolling. Payload view uses the same viewport. While sending, the pane MUST say that keys are ignored until the outcome is recorded.

Below 60 columns or 12 rows, the existing too-small guard MUST replace all these layouts. It MUST show actual and required dimensions and the existing quit/cancel instruction within the available rectangle. It MUST preserve the sending exception and all typed text. The 100x30 and 80x24 too-small frames illustrate the design only. Tests MUST trigger it at 59x24, 80x11, and 40x10, then restore size.

## Width tiers

| Terminal width | Findings columns | Other changes |
| :--- | :--- | :--- |
| 100 and above | Selection, state, id, severity, confidence, title, location | Location gets 24 cells. Title gets the remainder. Summary and nested location panes remain. |
| 80–99 | Same columns | Location gets 18 cells and prefers filename plus line range. Title gets the remainder. Footer wraps between hints. |
| 60–79 | Selection, state, id, severity, title | Confidence and location disappear from the list only. Detail retains both. Metadata wraps between pills. The nested location remains, with a middle-truncated path. |
| Below 60 | Too-small screen | No review action may operate behind it. |

Selection and state consume one cell each plus their separators. The id column is at least five cells and grows for larger ids. A severity pill reserves ten cells, including padding or monochrome brackets. Confidence reserves three cells. Columns have one-cell gaps. At 100 columns with a five-cell id, the title has 46 cells before any blocking/note marker. At 80 it has 32 cells. Each marker consumes two cells within that title allocation. Long ids MUST take space from title first, then location, then confidence. The full id MUST remain visible in detail, where it MAY wrap. A narrow title MUST still leave at least eight cells before optional columns disappear.

Header priority is exact decided count, badge, PR reference, round, then title. Truncate the title to zero first. At 80–99 use `rN` for the round. Below 80 omit the title and bar, and retain `rN` if it fits. At widths of at least 80, omit the bar if it prevents the PR reference and count from fitting after the title is gone. Then omit the round, then middle-truncate the reference. Keep the complete URL behind the hyperlink and in the opener failure message. The header MUST remain one row. A bar that is shown MUST be complete, never a misleading prefix.

Top-border titles MUST retain their left title and essential counts. At 60–79, use `B blocking · N notes · P pending`. Counts MUST NOT disappear through clipping. Bottom borders reserve the position at the right, then fit the notice or hint at the left. Long error text belongs in wrapped pane content, not a clipped border. Scroll thumbs MUST have at least one cell, move monotonically with offset, and reach the top/bottom at the corresponding document bounds.

## Palette and non-color signals

| Role | Dark | Light | 16-color fallback |
| :--- | :--- | :--- | :--- |
| Base / text | `#1e1e2e` / `#cdd6f4` | `#eff1f5` / `#4c4f69` | Terminal background / foreground |
| Surface0 / Surface1 | `#313244` / `#45475a` | `#ccd0da` / `#bcc0cc` | Background / bright black, with borders |
| Accent | `#b4befe` | `#7287fd` | Bright blue |
| Secondary / overlay1 | `#a6adc8` / `#7f849c` | `#6c6f85` / `#8c8fa1` | Foreground / bright black |
| Accepted | `#a6e3a1` | `#40a02b` | Green |
| Open note / citation | `#f9e2af` | `#df8e1d` | Yellow |
| Pending blocking | Red `#f38ba8` blended 45% over base | Red `#d20f39` blended 45% over base | Red |
| Critical pill | `#ff7b72` | `#cf222e` | Red |
| Major pill | `#ffa657` | `#bc4c00` | Yellow |
| Minor pill | `#e3b341` | `#9a6700` | Yellow |
| Trivial pill | `#8b949e` | `#6e7781` | Bright black |

Pill fill MUST blend its foreground at 12% over the underlying surface in true color. Accepted rows use secondary text. Excluded and withdrawn rows use overlay text. Selected rows use a `surface0` background and accent selection bar. Foreground-only token styles MUST inherit that background. In 16-color mode, pills MAY omit the tint. Uppercase text and brackets MUST retain their boundaries. Color collisions MUST NOT remove labels or markers. ANSI256 MAY use nearest palette colors while retaining the same structural signals.

`NO_COLOR` with any nonempty value MUST disable all SGR styling and OSC 8 output. An ASCII-capability output MUST also remain free of generated escape sequences. Unicode borders remain available when the icon tier is Unicode. In the ASCII icon tier, borders use `+`, `-`, and `|`, confidence uses `###`, `##-`, or `#--`, and the citation/selection marker is `>`.

| Meaning | Unicode | Monochrome meaning |
| :--- | :--- | :--- |
| Selected | `▌` | `>` |
| Accepted | `✓` | `✓` (ASCII `+`) |
| Excluded | `×` | `×` (ASCII `x`) |
| Withdrawn | `↩` | `↩` (ASCII `-`) |
| Pending | `·` | `·` (ASCII `.`) |
| Blocking | `●` before title, `● blocking` in detail | Same (ASCII `!` / `! blocking`) |
| Open note | `✎` before title | Same (ASCII `?`) |
| High / medium / low confidence | `▰▰▰` / `▰▰▱` / `▰▱▱` | `■■■` / `■■□` / `■□□` |
| Severity | Tinted uppercase pill | `[CRITICAL]`, `[MAJOR]`, `[MINOR]`, `[TRIVIAL]` |
| Cited line | Yellow tint and left bar | `>` plus unchanged line number and diff sign |

In monochrome, per-finding progress uses `=`, `x`, `~`, `!`, and `-` for accepted, excluded, pending with open note, pending blocking, and other pending. Above 30 it uses those characters in the same 20-cell grouped runs. Help MUST explain this legend and the confidence scale. It MUST distinguish an open note from a reply that still needs human action. A note marker MUST NOT replace the disposition marker.

Footer hints MUST retain `p publish (not ready)` and other unavailable labels under `NO_COLOR`. Colored output MAY use the shorter dim hint, because counts explain readiness. Missing browser capability uses `o open PR (unavailable)` in monochrome. Pressing an unavailable key MUST still produce the existing refusal or the specified opener explanation. Decide, move, and app group labels are semantic groupings, not extra printed headings. On a wrapped footer, separators MUST NOT appear alone at a row boundary.

## Muted code palette

Chroma supplies token boundaries, not the complete colored terminal string. The style package MUST apply these foreground-only roles. Plain identifiers, operators, punctuation, functions, and types share the neutral text role. Bold MAY mark keywords. Syntax MUST NOT paint its own background.

| Token role | Dark | Light |
| :--- | :--- | :--- |
| Neutral | `#c0c3ce` | `#4c4f60` |
| Keyword | `#aca7ba` | `#655e77` |
| String | `#a8b4a5` | `#586850` |
| Number / literal | `#b7ada1` | `#716252` |
| Comment | `#888c99` | `#737684` |

These muted roles replace Catppuccin's saturated mauve, green, and peach tokens. Added-line background blends green at 8% over base. Deleted-line background MAY blend red at 8%. Citation background blends yellow at 14% and wins over either tint. The citation bar uses the full yellow. All token spans and trailing padding MUST carry the winning line background, so token resets cannot cut holes in it. In 16-color mode, syntax MUST use neutral text with keyword emphasis, and citation/diff markers MUST carry meaning even if tints disappear. Monochrome MUST use plain token text.

## Interaction details

Success notices use a two-second timer and a notice generation token. A timer MUST clear only its own notice. Failure notices MUST NOT share that expiry. If an error needs more room than the border offers, the pane MUST reserve wrapped body rows for the complete message and corrective text. At small heights it MAY use the existing scrollable document, retaining a border indicator until the message is dismissed. It MUST NOT create an invisible refusal below a clipped viewport.

`o` in list/detail targets the captured PR URL, never a finding URL or the published review URL. A pending opener MUST reject repeat launches without blocking navigation. Completion MUST be associated with its request context, so a delayed result cannot quit another view or overwrite an editor. Success keeps the current view. Failure shows `Could not open the browser: <reason>` and `Open this URL manually: <safe URL>`. No configured opener shows `No browser opener is available.` with the same URL. Invalid URLs show a safe explanation without emitting a hyperlink. Published behavior remains as implemented today.

Tabs are visual step labels. They MUST NOT add Tab navigation to the action or inline picker. The Message pane on earlier steps explains preparation, not a claim that a write already happened. Final confirmation MUST display its existing cancellation instruction and input-focus hints. The status for a published review MUST distinguish it from a browser launch that failed.
