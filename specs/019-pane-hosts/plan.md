# Implementation Plan: Pane hosts

**Branch**: `019-pane-hosts` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/019-pane-hosts/spec.md`

## Summary

`internal/pane` becomes a small host registry: a `Host` interface, a fixed detection order (Herdr, then Orca), one direction rule (the host's report of the agent pane's width, else the agent's tty, else `right`; amended 2026-09-21), one command line carrying `LOUPE_HOME`, and one `pane-failed` builder with `details.host` and `details.step`. Herdr moves into `herdr.go` with its steps renamed `probe`, `split` and `run`; Orca joins in `orca.go` with `probe`, `split` and `switch`. `loupe handoff` detects a host, hands it `Deps.TTYWidth` in the request, and reports `host.Name()` and the direction the host chose. The skill's sandbox rerun rule keys on `error.details.step` being `probe`. Help, README, AGENTS, CONTRIBUTING, the host guard and spec 001's contract follow.

## Technical Context

**Language/Version**: Go 1.25, Markdown, Bash (guard).

**Primary Dependencies**: None added. `golang.org/x/term` is already direct and reads the tty. Herdr 0.9.0 and Orca are hosts, not dependencies.

**Storage**: N/A. `loupe handoff` still reads a run and writes nothing.

**Testing**: `internal/pane` unit tests with an injected runner, one file per host plus the shared cases; `internal/cli/handoff_test.go` with fake `herdr` and `orca` shell scripts on the injected `PATH` and a fixed `TTYWidth`; `internal/cli/plugin_test.go` for the skill; the `handoff-help` goldens; `scripts/check-tests.sh`.

**Target Platform**: Herdr panes and Orca terminals on Linux and macOS, with a shell that has `&&` and `NAME=value command` (spec 003).

**Project Type**: CLI.

**Constraints**: Constitution 2.0.2. No pseudo-terminal, real tty, real Herdr or real Orca in tests (VII). The `--json` envelope and refusal codes are pinned; no code is added or removed, and `pane-failed` gains `details`.

**Scale/Scope**: One package split into three files and three tests, one command's detection, direction and help, one `Deps` field, one skill rule, one guard, docs and spec 001's contract.

## Research

The Orca facts, the live split probe's results, and the decisions behind the width source, the detection order, the error text, the command line and the message format are in [research.md](research.md). The `orca.go` parser and `orca_test.go`'s canned split reply MUST follow the recorded result.

## Design

### `internal/pane`

- **`pane.go`**, the shared pieces:
  - `type Host interface { Name() string; Open(ctx context.Context, req Request) (Opened, error) }`, with `Request{Command string; TTYWidth func() (int, bool); Fix string}` and `Opened{PaneID, Direction string}`. `DataRoot` is dropped: neither host reads it once `LOUPE_HOME` is in the command. The host decides the direction inside `Open` and reports it in `Opened.Direction` (amended 2026-09-21).
  - `Detect(getenv func(string) string) (Host, bool)` tries `detectHerdr` then `detectOrca` and returns the first match. A host whose conditions are met only in part is skipped (FR-003).
  - `direction(hostWidth int, hostOK bool, tty func() (int, bool)) string`: takes the host's width when `hostOK`, else calls `tty` when it is non-nil; a known width below `wideEnough` is `down`, anything else `right`. `wideEnough` stays 120 (amended 2026-09-21, spec FR-005).
  - `TTYWidth() (int, bool)`: opens `/dev/tty`, calls `term.GetSize` on its fd, closes it; any error is `(0, false)`.
  - `Runner func(ctx context.Context, path string, args ...string) (stdout, stderr []byte, err error)`, with the shared `execRunner`. Each host has a `message(stdout, stderr []byte, err error) string` that picks the error text (research, "Error text").
  - `failed(host, step, message, fix string) error` returns `refusal.PaneFailed` with message `<host> <step>: <message>` and `Details{"host": host, "step": step}`.
  - `lookPath(getenv, name string) (string, bool)`: today's `PATH` walk from `Detect`, shared.
- **`herdr.go`**: `detectHerdr` keeps FR-001's conditions. Steps: `probe` runs `pane layout --pane <id>`, requires it to exit zero, parse and list the agent's pane, and reads that pane's `rect.width` (absent is unknown) into `direction`. `split` runs `pane split --pane <id> --direction <direction> --focus` with no `--env`, and requires `result.pane.pane_id`. `run` runs `pane run <new id> <req.Command>`. `herdrMessage` moves here, reading stderr and the exit error only.
- **`orca.go`**: `detectOrca` needs `ORCA_TERMINAL_HANDLE` non-empty and an executable `orca` on `PATH`, and reads no other variable. Steps: `probe` runs `terminal show --terminal <handle> --json` and requires `ok: true`; it catches a stale or unknown handle, not an exited terminal, which is enough because the handle is the agent's own. `split` runs `terminal split --terminal <handle> --direction horizontal|vertical --command <req.Command> --json`, with the direction from `direction(0, false, req.TTYWidth)`, mapping `right` to `vertical` and `down` to `horizontal` (Orca names the divider; research.md records the observation), and requires `ok: true` and the new handle at `result.split.handle`; a missing handle refuses at `split` and runs no `switch`. `switch` runs `terminal switch --terminal <new handle> --json`. `Opened.PaneID` is the new handle; `Opened.Direction` is `right` or `down`. `orcaMessage` reads `error.message` from stdout JSON, else the last stderr line not starting `[relay-connect]`, else the exit error.
- **Tests**: `pane_test.go` keeps the shared cases (direction table: tty 120, 119, unreadable and absent, and a host width overriding the tty; detection order with both environments; a partial host skipped; `failed`'s message and details). `herdr_test.go` takes today's fake and calls table without `--env`, with the step keys renamed, plus layout widths 119 and 120 overriding a tty width and a pane without a width falling back to the tty. `orca_test.go` mirrors it with the tty deciding the direction (unreadable is `right`), the recorded split result (`result.split.handle`), the stale-handle error on stdout with the handshake on stderr, a split without a handle, and a `switch` failure after a split.

### `internal/cli`

- **`root.go`**: `Deps` gains `TTYWidth func() (int, bool)`; nil means unreadable, so a test that does not set it gets the host's width, else `right`.
- **`handoff.go`**: after `review-open`, `pane.Detect(deps.Getenv)`; on no host, `no-pane-host` with `no terminal pane can be opened here: HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH, or ORCA_TERMINAL_HANDLE and orca on PATH`. The command is `LOUPE_HOME=<shellQuote(abs root)> <shellQuote(exe)> review <shellQuote(ref)> && exit`, built once by `reviewCommand`. `deps.TTYWidth` goes into `pane.Request`; the direction comes back in `Opened.Direction`. The result's `host` is `host.Name()`; `pane.Name` goes. `handoffHelp` is rewritten host-neutral: one line per host's detection, the detection order, the direction from the host's report of the agent pane's width, else the agent's terminal, else `right`, both refusals with `pane-failed`'s `details.step` and `probe` as the step that opens nothing, and the result with `host` `herdr` or `orca`.
- **`handoff_test.go`**: `fakeHerdrBin` keeps a layout width (150) and loses the `--env` argument; a new `fakeOrcaBin` records argv the same way, prints the handshake on stderr on every call, and at `failAt` prints Orca's error shape on stdout and exits 1. New cases: Orca success (`show`, `split`, `switch`; `host: orca`; the new handle as `paneId`); stale handle refuses at `probe` with one call; both environments set runs only `herdr`; directions from Herdr's layout width over the tty, and from the tty on Orca with unreadable as `right`; `shellQuote` against literals; `no-pane-host` names both hosts and runs neither binary; `pane-failed` carries `details.host` and `details.step` on both hosts.
- **`plugin_test.go`**: `TestPluginSkillNamesNoHerdrCommand` also rejects `orca` and no longer strips `` `herdr pane layout` ``; the pinned sandbox line becomes FR-017's text.

### Wiring

- **`cmd/loupe/main.go`** and **`cmd/loupe-demo/main.go`**: `TTYWidth: pane.TTYWidth`. `cmd/loupe-demo/main_test.go` leaves it nil.

### Documents

- **`specs/001-loupe-v1/contracts/cli.md`**: the `handoff` section and the `pane-failed` row are amended there during implementation, as FR-016 lists. This plan does not copy the contract into `specs/019-pane-hosts/contracts/`.
- **`plugin/skills/human-review/SKILL.md`**: FR-017's sandbox rule; no `herdr` or `orca` command.
- **`README.md`**, **`AGENTS.md`**, **`CONTRIBUTING.md`**: FR-019 and FR-020.
- **`scripts/check-tests.sh`**: the guard greps `herdr|orca` with today's exclusions and says "Herdr or Orca" in its violation text (FR-015).
- **`specs/006-agent-plugins/spec.md`**: FR-021's superseded and amendment notes.
- **`specs/001-loupe-v1/validation.md`**: the new tests, the live checks as they run, and FR-023's Unverified rows. The probe observed an Orca pane closing on shell exit, so that row is recorded as observed on 2026-09-21 rather than left unverified.
- **Goldens**: `go test ./internal/cli/ -update`; only `handoff-help.80.txt` and `handoff-help.100.txt` may move.

### Tests that pin the old Herdr text

The tasks stage MUST rename these:

- `internal/pane/pane_test.go:89` asserts the message contains `herdr pane layout`; the step keys `"layout"` throughout the file become `"probe"`.
- `internal/cli/plugin_test.go:147` strips `` `herdr pane layout` `` from the skill before checking for `herdr`, and `:182` pins the sandbox line that matches `error.message` starting `herdr pane layout`.
- `testdata/golden/cli/handoff-help.80.txt` and `.100.txt` carry "A failed herdr call refuses pane-failed with Herdr's message"; they move with the help rewrite.

`internal/cli/handoff_test.go` asserts only Herdr's own text (`pane w1:p1 not found`), not the prefix, and needs no rename for the message.

## Constitution Check

- **I. A tool for agents.** PASS. `loupe handoff` stays reachable from `loupe --help`, and `--help` names both hosts' conditions. Orca, like Herdr, is the human's terminal, not an agent host, so no path is reachable from only one agent host.
- **II. Nothing posts unread under a human's name.** PASS. Handoff opens review and publishes nothing; the pane rules from 006 hold.
- **III. Local files, no service.** PASS. `orca` is a local executable, as `herdr` is. Its transport to the Orca application is Orca's, not network access by loupe. No run state is written.
- **IV. Never touch the user's checkout.** PASS. No git call.
- **V. Machine contract first.** PASS. No code added or removed; `pane-failed` gains `details.host` and `details.step`, amended in `contracts/cli.md`; both refusals keep their fix.
- **VI. Simplicity over ceremony.** PASS. No dependency. The `Host` interface has two concrete uses, which is what 006 lacked when it rejected one; see Complexity Tracking.
- **VII. Verified means ran.** PASS. Fake `herdr` and `orca` on the injected `PATH` and an injected width; no test opens `/dev/tty`. The live Orca checks and the Herdr walk go on the Unverified list until they run.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/019-pane-hosts/           spec.md, plan.md, research.md, quickstart.md, tasks.md, checklists/
internal/pane/                  pane.go, herdr.go, orca.go, pane_test.go, herdr_test.go, orca_test.go
internal/cli/                   root.go (Deps.TTYWidth), handoff.go, handoff_test.go, plugin_test.go
cmd/loupe/main.go               TTYWidth wiring
cmd/loupe-demo/main.go          TTYWidth wiring
testdata/golden/cli/            handoff-help.80.txt, handoff-help.100.txt
plugin/skills/human-review/     SKILL.md sandbox rule
scripts/check-tests.sh          host guard widened to Orca
README.md, AGENTS.md, CONTRIBUTING.md
specs/001-loupe-v1/             contracts/cli.md, validation.md
specs/006-agent-plugins/spec.md superseded and amendment notes
```

**Structure Decision**: One package gains two host files. No `data-model.md`: the run layout and draft are unchanged, and the handoff result's fields are the contract's. No `contracts/` copy: the change lands in `specs/001-loupe-v1/contracts/cli.md`.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| A `Host` interface in `internal/pane`, which 006's plan rejected | Orca is the second concrete host (VI) | Two parallel `Detect` and `Open` pairs would duplicate the direction rule, command and refusal shape, and push host choice into `cli` |
| `pane-failed` messages change to `<host> <step>: <message>` for every host, so Herdr's `herdr pane layout: ...` becomes `herdr probe: ...` | One builder for every host; the skill now keys on `details.step`, not the message | Keeping Herdr's prefix makes the message shape host-specific; the contract pins `details`, not message text, and the pinning tests above are renamed |
| The direction is decided inside `Host.Open`, from the host's width first, not in `handoff` (amended 2026-09-21) | Only Herdr's layout gives a width from an agent's shell: Claude Code's Bash tool has no readable tty and Orca reports no size (research, "Width sources") | Deciding in `handoff` needs the host's width passed out of `Open` before the split, which splits one step in two or puts per-host logic in `cli` |
| `TTYWidth` is injected through `Deps` and `pane.Request` instead of `internal/pane` opening `/dev/tty` itself | Tests MUST NOT touch a real terminal (VII) and need fixed widths of 120, 119 and unreadable | Calling `pane.TTYWidth` inside a host leaves tests reading whatever tty the test runner has |
| The host guard allows `internal/cli/handoff.go` to name Orca as well as Herdr | Its `--help` and `no-pane-host` message have to name both hosts' conditions (I) | Carried over from 006; moving `cli`'s text into `pane` splits one command's contract across two packages |
