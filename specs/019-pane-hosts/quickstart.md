# Quickstart: Pane hosts

How to check this feature. The behavior is in [spec.md](spec.md); the contract change lands in `specs/001-loupe-v1/contracts/cli.md`.

## Automated

1. `go test ./internal/pane/ ./internal/cli/ -run 'Handoff|Open|Detect|Direction|Skill'`. Expected: with the fake `orca` on the injected `PATH`, the calls are `show`, `split`, `switch` in order and the result carries `host: orca`; a stale handle refuses `pane-failed` with `details.step: probe` and makes no further call; on Orca tty widths 120, 119 and unreadable give `right`, `down`, `right`, and on Herdr the layout width decides over the tty; with both environments set, only `herdr` runs.
2. `go test ./internal/cli/ -update`, then `git diff testdata/golden/cli`. Expected: only `handoff-help.80.txt` and `handoff-help.100.txt` move.
3. `mise run check`. Expected: green, including the widened `herdr|orca` guard in `scripts/check-tests.sh`.

## Live, in Orca (owner-approved)

1. From an agent terminal at least 120 columns wide: `LOUPE_DEMO_HOME=.demo mise run demo -- handoff 'acme/widgets#42' --json`. Expected: `host: orca`, `direction: right`, and review opens in the split with focus.
2. Press `q` in review. Expected: the pane closes, as the probe split's `exit` did (research, "Live probe").
3. Hand off a published run so `loupe-demo review` refuses in the pane. Expected: the pane stays open showing `error:` and `fix:`.

Record each result in `specs/001-loupe-v1/validation.md`; any step not run goes on its Unverified list (spec FR-022, FR-023).
