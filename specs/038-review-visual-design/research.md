# Research: Review visual design

## Existing behavior and mock gaps

**Decision**: Use round five as the visual target and preserve the existing interaction model. Earlier rounds explain the title borders, overlay, and transient notice, but do not override the final header and bar.

**Rationale**: `internal/tui/list.go:updateList` supports selection, summary, reload, opening detail, and publication. It does not support the mock's `a/x/s` list shortcuts. `internal/draft/derive.go` already defines current decisions, stale revisions, withdrawal, ordering, and readiness. The appearance MUST use those results. The default seven-finding mock is coherent: three decided out of six counted, with seven visible rows.

**Alternatives considered**: Adding the mock's list shortcuts would violate the brief's only-new-binding rule. Counting sent-back findings as decided would contradict the current model. Hiding withdrawal would lose inspection and reinstate access. The private comparison page could not be retrieved, so the supplied frames and brief are the evidence.

**Settled clarifications, 2026-10-26**: The owner accepted the seven decisions recorded in the report. The spec records them as settled clarifications, including exact aggregation, absent-opener behavior, and the meaning of the note box. Long summaries cannot remain fully visible alongside findings at 60x12 without new navigation. The plan keeps explicit clipping and the existing `loupe show` route to the complete summary.

## Bubble Tea and lipgloss v2

**Decision**: Consume the separate migration's `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` APIs. Model views return `tea.View` through `tea.NewView(content)`. The migration owns `View.AltScreen`, input-event conversion, Bubbles upgrades, and test-harness compatibility.

**Rationale**: The [official upgrade guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md) and [Bubble Tea API](https://pkg.go.dev/charm.land/bubbletea/v2) describe the new view and terminal messages. Handle `tea.ColorProfileMsg.Profile` and `tea.BackgroundColorMsg.IsDark()` through the existing migrated model. Tests MAY use `tea.WithColorProfile(...)`.

**Alternatives considered**: A v1 design would force another layout migration. Combining the migrations would exceed this change's scope. The local mock already compiles against lipgloss v2.0.4.

## Layer composition and profile fallback

**Decision**: Use `lipgloss.NewLayer(content).X(x).Y(y).Z(z)` and `lipgloss.NewCompositor(base, overlay).Render()` inside a style helper. Render the list with muted style roles, then place an opaque help pane over it. Use `lipgloss.LightDark(isDark)` and `lipgloss.Complete(profile)(ansi, ansi256, truecolor)` for explicit roles.

**Rationale**: These APIs exist in the [lipgloss v2 API](https://pkg.go.dev/charm.land/lipgloss/v2) and local v2.0.4 source. Layers have no opacity parameter. Border titles and scroll thumbs need small local primitives used by multiple panes. Preserve the explicit nonempty `NO_COLOR` check: local [colorprofile](https://pkg.go.dev/github.com/charmbracelet/colorprofile) v0.4.3 parses the environment value as a boolean, which differs from loupe's behavior for values such as `anything`.

**Alternatives considered**: Hand-cutting the overlay into a string would duplicate v2 composition. Independent terminal probes through a compatibility package would duplicate the migrated runtime's knowledge. Automatic color downsampling alone cannot preserve monochrome state.

## Hyperlinks and browser opening

**Decision**: Reuse `style.Link`, `style.LinkableURL`, `Config.OpenURL`, and the asynchronous opener in `internal/tui/published.go`. Generalize its request context so list/detail completion does not quit.

**Rationale**: The existing opener handles platform fallback and validates URLs. `frameWith` currently calls `render.ForDisplayANSI`, which removes OSC 8. Trusted generated links MUST be inserted after sanitization or kept separate from untrusted spans, as the published view already does. Lipgloss v2 also provides `Style.Hyperlink`, but that is not a reason to replace the established validation boundary.

**Alternatives considered**: A second opener duplicates platform behavior. A blanket escape-preserving frame sanitizer could pass hostile captured text. Reusing the existing completion unchanged would quit the review on a successful browser launch.

## Muted syntax and backgrounds

**Decision**: Promote existing `github.com/alecthomas/chroma/v2` to a direct dependency when imported. Use `lexers.Match(path)`, `chroma.Coalesce(lexer)`, `lexer.Tokenise(...)`, and `chroma.SplitTokensIntoLines(...)`. Map tokens to the five foreground roles in the layout contract.

**Rationale**: The [Chroma API](https://pkg.go.dev/github.com/alecthomas/chroma/v2) and [library guide](https://github.com/alecthomas/chroma) support token iteration. Chroma is already indirect at v2.27.0 through glamour. Its terminal formatter emits resets between tokens. Rendering spans through style allows every token and trailing cell to preserve the winning selection/citation background. Unknown language is a normal plain-text fallback. A tokenization failure MUST retain the original code and show a brief highlighting-unavailable notice rather than hide code or swallow the failure.

**Alternatives considered**: The mock's regular expression cannot parse languages correctly. Importing another syntax package would duplicate Chroma. Stock Catppuccin highlighting was rejected as too colorful. Neutral code with a few muted token roles meets the owner's direction.
