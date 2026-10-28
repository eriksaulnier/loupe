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
