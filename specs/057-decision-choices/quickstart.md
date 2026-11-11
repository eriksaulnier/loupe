# Quickstart: check decision choices end to end

Run these from the worktree after `mise run build`. Inside the repository, `loupe` is the dev build.

## Automated

1. `go test ./internal/draft/ ./internal/mode/ ./internal/cli/ ./internal/tui/ ./internal/integration/`. The refusal table in `contracts/cli-057.md`, the three picks (SC-002) and the pick-required check (SC-003) each have a test.
2. `go test ./internal/publish/ -run Schema`. This pins the schema 7 field set.
3. `mise run check` after the last edit.

## Older binary refuses a schema 7 draft (SC-005)

1. Seed a local run with a choice finding: `LOUPE_DEMO_HOME=<dir> mise run demo -- show 'acme/widgets:local#1' --json`.
2. Run the installed release on that home from outside the repository, where `loupe` is not the dev build: `cd ~ && LOUPE_HOME=<dir> loupe show 'acme/widgets:local#1'`.
3. Expect code `record`, a message that names schema 7 and the release's own schema, and the fix `upgrade loupe`.

## By eye, through tmux

1. Run `mise run demo -- review 'acme/widgets:local#1'`. Open "Who invalidates an entry on a force-push?". Three option sections show. The second carries `recommended`.
2. Press `a`. The picker opens under the finding, and option 2 is selected. Press `3`. The finding is accepted, and the header shows `accepted · 3`.
3. Press `t`, then `1`. The note composer opens. Type a note and press enter. The header shows `accepted · 1` and the note chip.
4. Press `a`, then esc. Nothing changes.
5. Repeat with `NO_COLOR=1`, and at the ASCII icon tier. The selected row and `recommended` read without color.
6. Return the run. `loupe feedback 'acme/widgets:local#1' --json` shows `choice` and the picked proposal only.

## Not covered here

Mouse picks (FR-013a) wait for spec 040.
