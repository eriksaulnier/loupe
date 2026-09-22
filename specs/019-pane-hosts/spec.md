# Feature Specification: Pane hosts

**Feature Branch**: `019-pane-hosts`

**Created**: 2026-09-21

**Status**: Draft

**Input**: User description: "Pane hosts. `loupe handoff` opens review in an Orca split as it does in Herdr, detected from `ORCA_TERMINAL_HANDLE` and `orca` on PATH, through `orca terminal show` (probe), `split --command` and `switch`. The direction rule stays at 120 columns but reads the agent's own tty on every host, since Orca reports no pane size; unreadable is `down`. `internal/pane` gets a host interface with a fixed detection order (Herdr, then Orca), one direction rule, one refusal builder with `details.step`, and the skill's sandbox rerun rule keys on `step: probe` instead of a Herdr message. `LOUPE_HOME` travels in the command line on both hosts. Supersedes specs/006-agent-plugins FR-002 to FR-004 and FR-009's `host` value list. Builds on specs/003-herdr-handoff and specs/006-agent-plugins."

## Relationship to earlier specifications

This specification builds on `specs/003-herdr-handoff` and `specs/006-agent-plugins`, and amends 006:

- **`specs/006-agent-plugins`**: FR-002 (Herdr is the only detected host), FR-003 (width read from Herdr's layout) and FR-004 (`LOUPE_HOME` set on the split) are superseded by FR-001 to FR-008 here; FR-005 keeps Herdr's layout width as the first width source and adds the fallbacks. FR-009's `host` value list (`herdr`) gains `orca` (FR-013). FR-006's "any `herdr` call" now reads "any host call" (FR-010). FR-010's Herdr guard widens to Orca (FR-015). FR-014's "failure at the layout step" and FR-017's "the Herdr socket" are superseded by FR-017 and FR-018 here. Its Key Entity "Pane host: Herdr is the only one" no longer holds. Everything else in 006 stays in force, including FR-005's quoting, FR-007's fix, FR-008's "never look for, reuse, resize, read or close a pane", and the `review-open` amendment from `specs/017-live-review`.
- **`specs/003-herdr-handoff`**: unchanged. FR-015's pane lifetime (close on success, stay open on a refusal) and the reasoning in "Why the human-only rule can relax" now apply to an Orca pane as well.

The CLI contract in `specs/001-loupe-v1/contracts/cli.md` changes in its `handoff` section and in the `pane-failed` row of the refusal table. No refusal code is added or removed. The draft model, the publication state machine and the published comment format are unchanged.

### Why this fits the constitution

Principle I: `loupe handoff` stays one command any agent reaches from `--help`. Orca, like Herdr, is the terminal the human sits in, not the agent host, so supporting it adds no path only one agent host can reach.

Principle III: loupe runs a local `orca` executable, as it runs `herdr`. How that executable reaches the Orca application is Orca's transport, not network access by loupe.

Principle VI: a shared host seam was not justified while Herdr was the only host. Orca is the second concrete use, so the seam is now allowed and the direction rule and refusal shape move out of Herdr's code into it.

Principle VII: tests drive a fake `orca` on `PATH`, as they drive a fake `herdr`. No test runs a real Orca or a real terminal.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The agent opens review beside itself in Orca (Priority: P1)

The owner drives an agent from an Orca terminal. When the agent has filed its findings and summary it runs `loupe handoff --run <ref> --json`. A split opens beside the agent's terminal with review showing that run, and focus moves to it. The agent blocks on `loupe wait`. Today the same command refuses `no-pane-host` in Orca and every round falls back to "run `loupe review` yourself".

**Why this priority**: It is the ask. Orca is where the owner now works, and without it the handoff never fires.

**Independent Test**: With a fake `orca` on `PATH` that records its arguments and returns canned JSON, and a fixed terminal width, `loupe handoff` makes the probe, split and switch calls in order and returns `host: orca`, the new terminal's handle and the direction.

**Acceptance Scenarios**:

1. **Given** `ORCA_TERMINAL_HANDLE` is non-empty and `orca` is on `PATH`, and the Herdr conditions are not all met, **When** the agent runs `loupe handoff --run <ref> --json`, **Then** loupe checks the agent's terminal handle, opens a split beside it running review for the resolved run, moves focus to the new terminal, and prints one result with `host: orca`, `paneId` set to the new terminal's handle, and `direction`.
2. **Given** the agent's own terminal is known to be at least 120 columns wide, or its width cannot be read, **When** it hands off, **Then** the split opens to the right; when it is known to be narrower, the split opens below (FR-005, amended 2026-09-21).
3. **Given** `ORCA_TERMINAL_HANDLE` names a terminal that no longer exists, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.host: orca` and `details.step: probe`, carrying Orca's message, and opens nothing.
4. **Given** Orca's split result carries no handle for the new terminal, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.step: split` and does not try to switch focus.
5. **Given** neither host's conditions are met, **When** the agent hands off, **Then** loupe refuses `no-pane-host` with a message naming what each host needs, and runs no host command.

---

### User Story 2 - A sandboxed agent reruns the handoff only when it is safe (Priority: P2)

An agent in Codex's default sandbox runs `loupe handoff`. The sandbox blocks the terminal host, so the first call fails. The skill tells the agent to rerun outside the sandbox only when the refusal says the failure came from the step that opens nothing, whichever host it is.

**Why this priority**: The rerun rule matches a Herdr message today, so it would never fire for Orca, or worse, would need a second host-specific string. A structured step name keeps the rule host-neutral and safe.

**Independent Test**: The skill test pins the rerun rule on `error.details.step` being `probe`, and checks the skill names no `herdr` or `orca` command. Handoff tests show a probe failure on each host carries `details.step: probe` and makes no further call.

**Acceptance Scenarios**:

1. **Given** `loupe handoff` refuses `pane-failed` with `details.step: probe`, **When** the agent follows the skill, **Then** it reruns `loupe handoff` once with the harness's approval to run outside the sandbox.
2. **Given** `loupe handoff` refuses `pane-failed` with any other step, **When** the agent follows the skill, **Then** it does not rerun, because a pane can already be open, and tells the human to run `loupe review '<ref>'` with the message in one line.

---

### User Story 3 - Herdr keeps working through the same seam (Priority: P3)

An owner inside Herdr hands off as before. The split opens beside the agent's pane with focus and runs review. Two things change and neither is visible in normal use: when Herdr's layout gives the agent pane no width, the direction falls back to the agent's own terminal and then to `right`, and `LOUPE_HOME` reaches review through the command line instead of a split option.

**Why this priority**: No new value, but the seam must not regress the host that already works.

**Independent Test**: The existing Herdr handoff tests pass with the calls table changed only by the dropped split option, the layout width still decides the direction, and a Herdr pane inside an Orca terminal is detected as Herdr.

**Acceptance Scenarios**:

1. **Given** the Herdr conditions are met, **When** the agent hands off, **Then** loupe makes the probe, split and run calls in order, and the result carries `host: herdr`.
2. **Given** both the Herdr and the Orca conditions are met, **When** the agent hands off, **Then** loupe uses Herdr and runs no `orca` command.
3. **Given** the Herdr probe fails, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.host: herdr` and `details.step: probe` and opens nothing.

### Edge Cases

- **Herdr inside Orca.** A Herdr session started in an Orca terminal gives its panes both environments, and the agent sits in the Herdr pane. Herdr MUST win, which is why the detection order is fixed.
- **Orca inside Herdr.** Orca is a desktop application and cannot run inside a Herdr pane, so the reverse case does not arise.
- **tmux inside Orca.** tmux overwrites `TERM_PROGRAM`, and Claude Code teammates spawn into tmux. Detection MUST NOT depend on `TERM_PROGRAM`, so an agent in tmux inside an Orca terminal still detects Orca.
- **Stale `ORCA_TERMINAL_HANDLE`.** A shell that inherited the variable from a closed terminal fails at the probe, which opens nothing. Orca reports this on stdout with exit 1, not on stderr.
- **Orca's handshake line.** Every `orca` call prints a relay handshake line on stderr, on success and on failure. It MUST NOT be reported as the error.
- **No controlling terminal.** The agent's harness runs loupe without a readable terminal, or with one whose size cannot be read, and the host reports no width. Claude Code's Bash tool is this case (observed 2026-09-21, `research.md`, "Width sources"). The direction MUST be `right` and the handoff MUST go on.
- **The agent's terminal is not the agent pane.** The tty width read is of the terminal loupe's process is attached to. If a harness attaches loupe to a terminal other than the agent's pane, the direction can be wrong in either way; the pane still opens. Herdr's layout width avoids this, which is why the host's report comes first.
- **Split succeeds, a later step fails.** An Orca `switch` failure, or a Herdr `run` failure, leaves a pane open. loupe MUST still refuse `pane-failed` with that step, MUST NOT close the pane (006 FR-008), and the skill MUST NOT rerun.
- **Split without focus.** Whether an Orca split takes focus is not known. loupe MUST switch focus explicitly so the pane has focus either way (003 acceptance scenario 1).
- **Unusual data root.** The absolute data root holds characters a shell treats specially. It MUST reach the pane's shell as one literal word, like the executable path and the reference.

## Requirements *(mandatory)*

### Functional Requirements

#### Detection

- **FR-001**: `loupe handoff` MUST detect Herdr when `HERDR_ENV` is `1`, `HERDR_PANE_ID` is non-empty and `herdr` is found on `PATH`, unchanged from 006 FR-002.
- **FR-002**: `loupe handoff` MUST detect Orca when `ORCA_TERMINAL_HANDLE` is non-empty and an executable `orca` is found on `PATH`. Detection MUST NOT require `TERM_PROGRAM` or any other Orca variable.
- **FR-003**: Detection MUST try Herdr first and Orca second, and MUST use the first host whose conditions are all met. A host whose conditions are met only in part MUST be skipped, not refused.
- **FR-004**: When no host is detected the command MUST refuse `no-pane-host`, and its message MUST name what each host needs: `HERDR_ENV=1`, `HERDR_PANE_ID` and `herdr` on `PATH`, or `ORCA_TERMINAL_HANDLE` and `orca` on `PATH`. It MUST run no host command.

#### Direction

- **FR-005** (amended 2026-09-21, owner's decision): The direction MUST come from the first of these that gives a width: the host's own report of the agent pane's width (Herdr: the agent pane's columns in the `probe` step's `pane layout` output, matched by `HERDR_PANE_ID`; Orca: none), then the width of the agent's controlling terminal. A known width of 120 columns or more MUST choose `right` and a smaller one `down`. When neither gives a width the command MUST choose `right` and MUST NOT refuse.
- **FR-006** (amended 2026-09-21): One shared function in the pane package MUST hold the threshold and the order of width sources, and each host MUST decide the direction through it inside its own steps, so the `handoff` command carries no per-host width logic. It replaces the rule that no host's own report MAY decide: that rule assumed the agent's tty is readable, which it is not from Claude Code's Bash tool.
- **FR-007**: On Orca, `right` MUST open a left-and-right split (`--direction vertical`) and `down` a top-and-bottom split (`--direction horizontal`): Orca names the divider, not the placement, as one live split recorded on 2026-09-21 in `research.md`. The result's `direction` MUST stay `right` or `down` on every host.

#### The pane's command

- **FR-008**: On every host the pane's command MUST be `LOUPE_HOME='<absolute data root>' '<executable>' review '<ref>' && exit`, with the data root the command itself resolved and each of the three values reaching the shell as one literal word under 006 FR-005's quoting rule. Herdr MUST no longer set `LOUPE_HOME` as a split option.

#### Host steps

- **FR-009**: Each host MUST open review in named steps, and the first step on every host MUST be `probe`: a call that checks the agent's pane or terminal exists and opens nothing. Herdr's steps MUST be `probe` (`herdr pane layout --pane <id>`, whose result MUST list the agent's pane and whose width for that pane feeds FR-005), `split` and `run`. Orca's steps MUST be `probe` (`orca terminal show --terminal <handle> --json`), `split` (`orca terminal split --terminal <handle> --direction <dir> --command <command> --json`) and `switch` (`orca terminal switch --terminal <new handle> --json`).
- **FR-010**: Any host call that exits non-zero, prints unparsable output, or lacks a needed field MUST refuse `pane-failed`. The command MUST NOT retry, and MUST make no host call after the failed one. An Orca split whose result carries no handle for the new terminal MUST refuse at `split`.
- **FR-011**: A `pane-failed` refusal MUST carry `details.host` (the host's name) and `details.step` (the failing step). Its message MUST name the host and the step and carry the host's own error text: for Herdr the `error.message` from its stderr JSON, for Orca the `error.message` from its stdout JSON, and otherwise the last stderr line that is not Orca's relay handshake. After any step but `probe` the message MUST also say that a pane may already be open, because the fix still tells the human to run `loupe review` and a second review of one run is allowed.
- **FR-012**: The fix for both refusals MUST remain 006 FR-007's `ask the human to run loupe review '<ref>'`.

#### Result, help and contract

- **FR-013**: On success the `--json` result MUST carry `host` as `herdr` or `orca`, `paneId` as the new pane's id (Herdr) or the new terminal's handle (Orca), and `direction`, in the standard envelope. Without `--json` it MUST print one line naming the run and the direction, as today.
- **FR-014**: `loupe handoff --help` MUST describe the handoff without assuming a host, with one line per host giving its detection conditions, followed by the detection order, the direction rule, both refusals with `pane-failed`'s `details.step`, and the result. The `handoff-help` golden MUST be regenerated and its diff read.
- **FR-015**: Host logic MAY live only in the one internal package that opens panes. Outside that package and its tests, only the `handoff` command's file MAY name Herdr or Orca, in its help and its `no-pane-host` refusal. `scripts/check-tests.sh` MUST fail when any other non-test Go file under `cmd/` or `internal/`, or `go.mod`, names Herdr or Orca, with the same exclusions as today.
- **FR-016**: The `handoff` section of `contracts/cli.md` MUST name both hosts and their detection, the detection order, that the direction comes from the host's report of the agent pane's width, else the agent's terminal width, else `right`, the `host` values `herdr` and `orca`, and that `pane-failed` carries `details.host` and `details.step`, with `probe` as the step that opens nothing on every host. The `pane-failed` row of the refusal table MUST name those details.

#### Skill

- **FR-017**: The `human-review` skill's sandbox rule MUST say that `loupe handoff` needs the terminal host's socket, and MUST allow rerunning `loupe handoff` outside the sandbox only when `error.details.step` is `probe`, because a later step can already have opened a pane. It replaces the match on a message starting `herdr pane layout`.
- **FR-018**: The skill MUST name no `herdr` or `orca` command. The test that pins the skill's lack of `herdr` commands MUST also cover `orca`.

#### Documentation and validation

- **FR-019**: The README's handoff section MUST become "Handing off in a terminal pane" and name Herdr and Orca. The Codex sandbox note MUST say "the terminal host's socket". The Commands table row for `handoff` MUST say "new terminal pane". The warning against blanket allow rules MUST also name `orca terminal split`.
- **FR-020**: The `AGENTS.md` layout row for `internal/pane` MUST describe it as opening review in a new Herdr or Orca pane and as the only package that runs either. `CONTRIBUTING.md`'s demo handoff line MUST say "inside Herdr or Orca".
- **FR-021**: `specs/006-agent-plugins/spec.md` MUST carry a "superseded on 2026-09-21 by `specs/019-pane-hosts`" note on FR-002, FR-003 and FR-004, and an amendment note on FR-009's `host` value list and on FR-006, FR-010, FR-014 and FR-017 as the Relationship section describes, in the form 006 used for 003.
- **FR-022**: `specs/001-loupe-v1/validation.md` MUST record the new tests, and MUST record the live checks in this Orca session when they run: from an agent terminal at least 120 columns wide, `LOUPE_DEMO_HOME=.demo mise run demo -- handoff 'acme/widgets#42' --json` returns `direction: right` and review opens in the split with focus; quitting review closes the pane; a refusal in the pane (review on a published run) leaves it open.
- **FR-023**: The Unverified list in `specs/001-loupe-v1/validation.md` MUST gain: whether the controlling terminal is readable from Codex outside its sandbox (from Claude Code's Bash tool it is not, verified 2026-09-21); whether an Orca pane closes when review itself exits with `q` (a shell running `echo && exit` closed its pane on 2026-09-21, see `research.md`); a `down` split in Orca; and a Herdr handoff after `LOUPE_HOME` moved into the command line. Any FR-022 check not run MUST be listed there instead.

### Key Entities

- **Pane host**: A terminal that can open a pane beside the agent's for review. Herdr and Orca, detected in that order.
- **Host step**: One named call in a host's handoff. `probe` opens nothing on every host; the other names are host-specific.
- **Handoff result**: `host`, `paneId`, `direction`, and the run reference.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Inside Orca, a full review round (agent files findings, human decides, sends notes back, agent revises, human publishes) needs no command typed by the human, as it already does inside Herdr.
- **SC-002**: On each host, a failure at the probe step opens no pane and makes no further host call, shown by a test per host.
- **SC-003**: The skill's rerun rule contains no host name, host command or host message text, shown by the skill test.
- **SC-004**: With a fixed tty width of 120 and 119 columns and an unreadable width, the result's direction is `right`, `down` and `right` on Orca; on Herdr a layout width of 120 and 119 gives `right` and `down` whatever the tty says, shown by tests.
- **SC-005**: `mise run check` passes after the final edit, including the widened host guard.

## Assumptions

Observed in Orca on 2026-09-21:

- Orca's managed shells carry `ORCA_TERMINAL_HANDLE=term_<uuid>` naming the terminal, plus `ORCA_PANE_KEY`, `ORCA_TAB_ID`, `ORCA_WORKTREE_ID` and `TERM_PROGRAM=Orca`. `orca` is on `PATH` from `$ORCA_REMOTE_CLI_BIN_DIR`.
- `orca terminal split --terminal <handle> --direction horizontal|vertical --command <text> --json` opens a split. `vertical` is left-and-right and `horizontal` is top-and-bottom, as observed on 2026-09-21; Orca's own CLI guide states the opposite and is wrong. It has no focus and no environment option, and `--command` is typed into the new shell, which is why `LOUPE_HOME` travels in the command line.
- `orca terminal switch --terminal <handle> --json` focuses a terminal. `orca terminal show --terminal <handle> --json` validates a handle and opens nothing; a stale handle prints `{"ok":false,"error":{"code":"terminal_handle_stale","message":...}}` on stdout and exits 1.
- No Orca command reports a terminal's columns, so on Orca only the agent's own terminal can give a width (FR-005).
- Every `orca` call prints `[relay-connect] Handshake OK at version=...` on stderr, success or not.

Not yet observed, and settled by one live split the owner approves before the Orca result parser is written, recorded in the plan's research:

- The split result carries the new terminal's handle at `result.split.handle`, as one live split recorded on 2026-09-21 in `research.md`. The parser MUST follow that recorded result.
- Whether a split takes focus. FR-009's `switch` makes the answer irrelevant to behavior.
- Whether an Orca pane closes when its shell exits. 003 FR-015's lifetime depends on it; if it does not, that is recorded as a gap, not worked around.

Also:

- Herdr 0.9.0 behavior recorded in specs 003 and 006 still holds, including the agent pane's width in `pane layout`.
- A `NAME=value command` prefix works in the shells 003 already requires (zsh, bash, fish 3.1+), so moving `LOUPE_HOME` into the command line adds no shell requirement.
- The controlling terminal is unreadable from Claude Code's Bash tool (observed 2026-09-21), so on Orca every handoff from Claude Code opens `right`. That is acceptable on the wide screens the owner uses; a narrow Orca window gets a cramped right split.
- A third host joins by adding its detection and steps to the pane package; this specification does not name one.
- No commit, push, pull request, release, live review, or run of `orca` beyond the one approved probe split is authorized by this specification.
