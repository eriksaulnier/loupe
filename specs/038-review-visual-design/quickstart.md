# Validation guide

This guide applies after implementation. Current product code still uses the prior appearance. A passing check on this specification change does not verify the proposed interface.

## Prerequisites

The separate Bubble Tea v2/lipgloss v2 migration MUST be complete and green. Use its Go version and compatible test harness. The human-review plugin guidance MUST match the implementation. Browser tests MUST inject an opener. Publication tests MUST use the fake GitHub.

## Automated checks

```sh
mise exec -- go test ./internal/style ./internal/tui ./internal/cli
mise run check
```

The focused run MUST pass with rendered-text, geometry, progress, and opener cases. The complete check MUST pass vet, lint, actionlint, test hygiene, and all Go tests. Existing `testdata/golden/cli` files MUST remain unchanged. Regenerate them only to investigate an intentional contract change, which this feature does not authorize.

The test matrix MUST include the reference frames, minimum dimensions, boundary resizes, dark/light, 16-color, both nonempty `NO_COLOR` examples, and icon tiers. It MUST include empty/all-withdrawn rounds, stale decisions, an open note with a reply, multiple notes on one finding, 30/31 counted findings, a current exclusion on an unincluded finding, and accepted blocking findings.

The opener matrix MUST cover success, failure, nil injection, invalid target, repeated `o`, and completion after navigation. The confirmation matrix MUST cover focused `y/p`, blur and confirm, payload toggle, cancellation, sticky history, head movement, message validation error, and resize during typing. Errors MUST remain visible without losing the human's text.

## By-eye review

Use `mise run demo` through a background tmux terminal for manual checks. Use the existing seeded demo instead of constructing a live run. No browser or real publication is needed. The reviewer MUST compare all required views with [the frame index](frames/README.md) at 100x30 and 80x24. The reviewer MUST also inspect light and 16-color output, then verify 60x12 and a resize below the minimum.

```sh
mise run screenshots
```

Inspect the resulting images in `docs/assets/`. Check the badge and lavender accent, titled panes, faded decided rows, muted syntax, continuous citation backgrounds, grouped footer, and help overlay. Check that required hints remain readable. Record the actual command output and any uncovered cases. Do not claim by-eye coverage from text assertions alone.

The published-format screenshot command MUST NOT run. The published review format is unchanged. The maintainer-only items in `specs/001-loupe-v1/validation.md` remain unverified.

## Implementation record, 2026-10-28

The implementation ran in agent sessions. An early session could not run the full check, because a type40 gate refused the `git push` that the `internal/testutil/gitrepo` fixture makes. It ran every step of the `check` task except the four packages that use the fixture. A later session ran the full check after the gate changed.

**T001 baseline**: `go.mod` held Go 1.26.7, `charm.land/bubbletea/v2` v2.1.0, `charm.land/bubbles/v2` v2.2.1 and `charm.land/lipgloss/v2` v2.0.6, and the migrated `x/vt` screen harness was in `internal/tui/app_test.go`. The targeted check passed before any change: vet printed nothing, lint printed `0 issues.`, and every package printed `ok`.

**T023**: `mise run check` at `982e4f8` exited 0. `golangci-lint` printed `0 issues.` Every package printed `ok`. The first full run failed in `internal/integration`, the package the early session left out. `TestPublishEndToEnd` read the confirmation without its new outer pane, and `TestPublishCarriesGateCounts` waited for `edit f-004` where the pane title is `Edit f-004`. Both tests were corrected, and the product code was unchanged. `go test -count=1 -v -run TestOpenUnder100ms ./internal/tui/` printed `open and first detail render took 12.664988ms`.

**T024**: `env -u NO_COLOR mise run screenshots` exited 0 after 149 s and rewrote `docs/assets/walkthrough.gif` and `docs/assets/sendback.gif`. Frames inspected: the list with the badge, lavender accent, titled panes, faded decided rows and grouped footer; the detail with the nested location pane, muted syntax and a continuous citation band; the success notice in the bottom-left border; the Message step with its tabs and nested input; and the published screen. A background tmux server showed the 100x30 list and detail and the help overlay in `NO_COLOR`, the 80x24 detail and file diff in true color, the 60x12 list with its summary collapsed to fit, and the too-small screen at 40x10. Light, 16-color and 256-color output were checked by tests only, not by eye. The GIFs do not show the help overlay or light mode. The images were recorded again from the committed redesign after the T025 fixes, and sampled frames were inspected.

**T025**: a same-vendor read-only review (Claude Opus 5.5) found one medium and five low problems. All six are fixed, each with a test that failed first: the 16-color surface painted black, the too-small pane cut its text at widths 24 to 38, a refusal at 60x12 lost its last row, a slow `o` from the list disabled `o` on the published screen, the clipped-summary command was cut at 60 columns, and dead code and stray glyph choices remained in the interface. A cross-vendor review (openai/gpt-6-astra, tree `067d4ac`) then found one issue: at 60x12 a refusal taller than the findings pane, and the own-PR refusal on the Action step, still lost rows. Both now keep the refusal whole, pinned by `TestRefusalTallerThanTheFindingsPaneStaysWhole` and `TestActionRefusalStaysWholeAtTheMinimum`, which failed first.

**T026–T035**: the maintainer's first demo session asked for the list position only when the list scrolls, a progress marker for the open finding in detail, a header gap under Orca, status at a glance in the detail title, status-colored borders, a blank row under the header, and no Message pane on the early publish steps. A second cross-vendor review (openai/gpt-6-astra, tree `98fabbe`) found two issues: the full-summary command was cut for long repository names, and a refusal taller than the screen lost its fix. Both are fixed, and each fix has a test that failed first. Adding the header gap also showed that the published screen found its URL rows by assuming a one-row header. The full check had hung in `internal/integration` once, through a deadlock in the test screen harness, which was fixed in #83.

**T036–T052**: a second demo session asked for more changes. Input panes became amber, and note threads hang from a left bar. Section headings and the fix bar took color, and statuses became chips. The footer wraps by group and sits on a band, and the focused pane shows its name as a tab. A decision flashes its notice, and the last decision returns to the list. The first publish steps became a popup with a plain step row. The Message step opens readable, with `v` stepping to the Markdown and the payload, and its message box opens two rows tall. `shift+↑↓` scrolls while typing. A band behind the detail's title was mocked and dropped. A third cross-vendor review (openai/gpt-6-astra, tree `ee47126`) found two issues: unwrapped summaries in the readable review, and a popup that could hide the selected choice at 60x12. Both are fixed, each with a test that failed first. The full check once hung for 10 minutes on a deadlock in the test screen harness, which was fixed in #83 (`21ab652`).

**Final check**: `mise run check` at `9f3d570` exited 0. `golangci-lint` printed `0 issues.` Every package printed `ok`. `TestOpenUnder100ms` printed `open and first detail render took 19.097648ms`. Both README images were recorded at `9f3d570` with `ORCA_TERMINAL_HANDLE` unset, and sampled frames were inspected.

**Not verified**: light and 16-color output by eye, a real browser launch from `o`, and the maintainer-only items in `specs/001-loupe-v1/validation.md`. `mise run review-screenshot` did not run, because it posts a GitHub review. The reference frames predate the demo feedback: they show the Message pane, the old step tabs, note panes and the raw confirmation. The contract's Demo feedback section overrides them. Mouse-wheel scrolling waits for its own spec. The header gap under Orca was checked by tests only.
