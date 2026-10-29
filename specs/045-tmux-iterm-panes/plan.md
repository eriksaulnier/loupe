# Implementation Plan: tmux and iTerm2 pane hosts

**Branch**: `045-tmux-iterm-panes` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/045-tmux-iterm-panes/spec.md`

## Summary

`internal/pane` gains two hosts on 019's seam: `tmux.go` and `iterm.go`. Detection returns every host whose conditions are met, in the order Herdr, tmux, Orca, iTerm2, and a new `pane.Open` opens in the first, passing over tmux only when its probe reports no attached client. tmux runs `display-message`, `split-window -d` and `send-keys`, each with `-S <socket>`. iTerm2 runs three `osascript` calls whose scripts take their values as arguments. `Deps` gains `GOOS` so a Linux test can detect iTerm2. Help, the contract, docs, the host guard, the skill pin and `validation.md` follow. tmux ships first as complete commits (US1 and US3), then iTerm2 (US2), so the two can split into two pull requests.

## Technical Context

**Language/Version**: Go 1.25, Markdown, Bash (guard).

**Primary Dependencies**: None added. tmux and iTerm2 are hosts, run through the existing `Runner`.

**Storage**: N/A. `loupe handoff` reads a run and writes nothing.

**Testing**: `internal/pane` unit tests with the injected `fakeRunner`, one file per new host; `internal/cli/handoff_test.go` with fake `tmux` and `osascript` shell scripts on the injected `PATH` and a fixed `TTYWidth`; `internal/cli/plugin_test.go` for the skill; the `handoff-help` goldens; `scripts/check-tests.sh`.

**Target Platform**: tmux on Linux and macOS; iTerm2 on macOS only.

**Project Type**: CLI.

**Constraints**: Constitution 3.1.0. No tmux server, AppleScript or pseudo-terminal in tests (VII). No refusal code added or removed.

**Scale/Scope**: Two host files and their tests, one detection change, one `Deps` field, one command's help and no-host message, one guard, one skill test, docs and spec 001's contract and validation.

## Research

The tmux samples and every design decision are in [research.md](research.md). Two samples change the design: a probe of a missing pane exits 0 with empty fields, so the probe echoes `#{pane_id}` and checks it, and `-d` was observed to keep the agent's pane active.

## Design

### `internal/pane`

- **`pane.go`**:
  - `Detect(getenv func(string) string, goos string) []Host` returns every host whose conditions are all met, in the order Herdr, tmux, Orca, iTerm2. A partly met host is left out.
  - `Open(ctx context.Context, hosts []Host, req Request) (Host, Opened, error)` calls each host's `Open` in order and returns the first that opens or refuses. A host that returns the unexported `errPass` is passed over. When every host passed, it returns a nil `Host` and no error.
  - The package comment names the four hosts.
- **`tmux.go`**: `detectTmux(getenv)` needs `TMUX` non-empty with its text before the first comma naming an existing file, `TMUX_PANE` non-empty and `tmux` on `PATH`. It reads no other variable. `Tmux{path, socket, paneID, run}`, `Name()` `tmux`. `Open`:
  - `probe` runs research's `display-message`. A non-zero exit refuses with `tmuxMessage(stderr, err)`. The reply is parsed as research says. A zero `session_attached` returns `errPass` with no further call.
  - The direction is `direction(width, width > 0, req.TTYWidth)`.
  - `split` runs `split-window -d -P -F '#{pane_id}' -h|-v -t <pane>`. The trimmed stdout is the new id, and empty refuses at `split`.
  - `run` runs `set-option -p -t <new> synchronize-panes off` when the probe reports the pane synchronized, then `send-keys -t <new> -l <command>`, then `send-keys -t <new> Enter`.
  - The result is `Opened{PaneID: new, Direction: dir, Focused: false}`.
  - `tmuxMessage` returns trimmed stderr, else the exit error.
- **`iterm.go`**: `detectITerm(getenv, goos)` needs `goos == "darwin"`, `TERM_PROGRAM == "iTerm.app"`, a non-empty id after the last colon of `ITERM_SESSION_ID`, and `osascript` on `PATH`. `ITerm{path, sessionID, run}`, `Name()` `iterm2`. `Open` runs the `probe`, `split` and `run` scripts of research, each as `osascript -e ... <args>`. The probe's columns go through `direction` as the host's width, where a reply that is not a positive integer is unknown. An empty split reply refuses at `split`. The message is trimmed stderr, else the exit error. The result's `Focused` is `false`.
- **Tests**: `pane_test.go` gains the detection-order table of SC-003 over `Detect` and an `Open` table: tmux passes to Orca, every host passes to nil, and a refusal stops the loop. `tmux_test.go` covers detection, the calls in order with exact argv, the probe's three refusals (exit 1, missing pane, garbled reply), the zero-client pass with one call, widths 120, 119 and unreadable with no tty, a split with no id, and a `run` failure that refuses at `run` and does not close the pane. `iterm_test.go` mirrors it: detection on `darwin` and `linux`, the argv with the session id and the command as separate arguments after the script, no script containing `activate`, a missing session and a refused Apple event at `probe`, widths, and an empty split reply.

`fakeRunner` keys on one argument index today. tmux keys on index 2. iTerm2 needs `keyOf func(args []string) string`, which matches the script text as research says.

### `internal/cli`

- **`root.go`**: `Deps.GOOS string`. Empty is not macOS.
- **`handoff.go`**: `hosts := pane.Detect(deps.Getenv, deps.GOOS)`. With none, refuse `no-pane-host`. Build the command, then call `pane.Open`. A nil host also refuses `no-pane-host`. The message names four hosts in order, as FR-012 lists. `handoffHelp` lists four hosts, says tmux needs a client attached to the agent's session and that tmux and iTerm2 open the pane without focus, and says `host` is `herdr`, `tmux`, `orca` or `iterm2`. The `no-pane-host` text is built once and shared by both refusals.
- **`handoff_test.go`**: `fakeHostBin` takes the argument position it keys on, so the tmux fake keys on `$3`. A separate `fakeOsascriptBin` matches the script text. New cases: tmux success with exact calls and result; tmux pane-failed at `probe` with one call; a stale socket path runs no tmux and opens in Orca; zero clients with Orca runs one tmux call and opens in Orca; zero clients alone refuses `no-pane-host`; Herdr with tmux runs no tmux; tmux with an attached client and Orca runs no orca; iTerm2 on `darwin` succeeds and on `linux` refuses `no-pane-host` with no osascript call; the four-host `no-pane-host` message.
- **`plugin_test.go`**: `TestPluginSkillNamesNoHerdrCommand` also rejects `tmux` and `osascript` (FR-016).

### Wiring

- **`cmd/loupe/main.go`** and **`cmd/loupe-demo/main.go`**: `GOOS: runtime.GOOS`.

### Documents

- **`specs/001-loupe-v1/contracts/cli.md`**: the `handoff` section and the `pane-failed` row (FR-014).
- **`docs/review-workflow.md`**: the "Handing off in a terminal pane" section names four hosts and their focus behavior. Its "do not allow" list gains the tmux and osascript commands (FR-017, research "Where the docs section lives").
- **`AGENTS.md`**: the `internal/pane` row (FR-017). **`CONTRIBUTING.md`**: the demo line names the four hosts.
- **`scripts/check-tests.sh`**: the widened guard (FR-015, research).
- **`specs/001-loupe-v1/validation.md`**: the new tests and the FR-018 Unverified rows.
- **Goldens**: `go test ./internal/cli/ -update`. Only `handoff-help.80.txt` and `handoff-help.100.txt` may move.

### Split into two pull requests

The tmux commits change `Detect`'s signature to take `goos` and add `Deps.GOOS` with no host reading it yet, so the iTerm2 commits touch only `iterm.go`, its tests, `pane.go`'s order list, help, contract and docs. Help and contract name tmux in the tmux commits and gain iTerm2 in the iTerm2 commits.

## Constitution Check

- **I. A tool for agents.** PASS. `loupe handoff` stays the one command, its `--help` names every host's conditions, and the skill names no host command.
- **II. Nothing posts unread under a human's name.** PASS. Handoff publishes nothing.
- **III. Local files, no service.** PASS. tmux talks to its server over a local socket and osascript sends Apple events on the same machine. No run state is written.
- **IV. Never touch the user's checkout.** PASS. No git call.
- **V. Machine contract first.** PASS. No refusal code added or removed. `host` gains two values and the contract records them.
- **VI. Simplicity over ceremony.** PASS. No dependency. `Detect` returning a list and `pane.Open` are justified by tmux's pass, a concrete use; see Complexity Tracking.
- **VII. Verified means ran.** PASS. Fake `tmux` and `osascript` on the injected `PATH`, an injected OS and width. Live behavior goes on the Unverified list.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/045-tmux-iterm-panes/     spec.md, plan.md, research.md, quickstart.md, tasks.md
internal/pane/                  pane.go, tmux.go, iterm.go, pane_test.go, tmux_test.go, iterm_test.go, herdr_test.go (fakeRunner)
internal/cli/                   root.go, handoff.go, handoff_test.go, plugin_test.go
cmd/loupe/main.go, cmd/loupe-demo/main.go
testdata/golden/cli/            handoff-help.80.txt, handoff-help.100.txt
scripts/check-tests.sh
docs/review-workflow.md, AGENTS.md, CONTRIBUTING.md
specs/001-loupe-v1/             contracts/cli.md, validation.md
```

**Structure Decision**: Two host files in the existing package. No `data-model.md`: no run file or draft field changes. No `contracts/` copy: the change lands in `specs/001-loupe-v1/contracts/cli.md`.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `Detect` returns a list and `pane.Open` walks it, instead of `Detect` returning one host | FR-005 skips tmux on a probe result, which only `Open` sees | Probing inside `Detect` runs the probe twice; a loop in `handoff` puts host order in `cli` |
| The tmux probe reads `#{pane_id}` as well as the two values FR-005 names | tmux 3.4 exits 0 with empty fields for a missing pane (research) | Reading only width and clients would take a dead pane as an unattended session and pass, or split a pane nobody chose |
| Acceptance scenario 1.3's refusal carries loupe's message, not tmux's | tmux prints no message for a missing pane on `display-message -p` (research) | No tmux message exists to carry |
| FR-017's README section is edited in `docs/review-workflow.md` | The section moved there before this feature | Re-adding it to the README duplicates it |
