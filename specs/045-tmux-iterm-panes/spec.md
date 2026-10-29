# Feature Specification: tmux and iTerm2 pane hosts

**Feature Branch**: `045-tmux-iterm-panes`

**Created**: 2026-10-28

**Status**: Draft

**Input**: Board draft "Draft spec: tmux and iTerm2 pane hosts for handoff". `loupe handoff` opens review in a pane only under Herdr or Orca, and everyone else gets a command to run in another terminal. tmux covers most terminal users, and iTerm2 covers many macOS users. A coworker's tool, sift, detects four hosts in the order Herdr, tmux, Orca, iTerm2 (`internal/pane/pane.go` of `github.com/tyrantkhan/sift`). The draft left open the detection order when hosts nest, focus and the width rule, how `review-open` sees a pane that is still open, and whether iTerm2 goes on the Unverified list.

## Relationship to earlier specifications

This specification builds on `specs/019-pane-hosts` and `specs/036-review-pane-guards`, and amends 019:

- **`specs/019-pane-hosts`**: FR-003's order (Herdr, then Orca) becomes Herdr, tmux, Orca, iTerm2 (FR-001 here). FR-004's `no-pane-host` message names four hosts (FR-012). FR-013's `host` values gain `tmux` and `iterm2`. FR-014's help, FR-015's host guard and FR-016's contract section widen to the new hosts (FR-013 to FR-015). The `probe` step, the `pane-failed` shape, the command line of FR-008 and the fix of FR-012 are unchanged and apply to both new hosts. Its Assumption "A third host joins by adding its detection and steps to the pane package" is the path this specification takes.
- **`specs/036-review-pane-guards`**: FR-005's width rule holds as amended there: the host's report of the agent pane's width, else the agent's terminal, else `down`. The brief's "host, then tty, then right" is 019's original rule, which 036 replaced. FR-006's `review-open` guard needs no change (see "How `review-open` sees an open pane" below).
- **`specs/003-herdr-handoff`**: FR-015's pane lifetime (close when review exits cleanly, stay open on a refusal) applies to tmux and iTerm2 panes.
- **`specs/006-agent-plugins`**: FR-008's rule that the agent never sends to, reads, resizes, closes or reuses the pane is unchanged.

The CLI contract in `specs/001-loupe-v1/contracts/cli.md` changes in its `handoff` section and in the `pane-failed` row of the refusal table. No refusal code is added or removed. The draft model, the publication state machine and the published comment format are unchanged.

### Why this fits the constitution

Principle I: tmux and iTerm2 are terminals the human sits in, not agent hosts. `loupe handoff` stays one command any agent reaches from `--help`, and the skill names no host command.

Principle III: loupe runs a local `tmux` client, which talks to the tmux server over a local socket, and a local `osascript`, which sends Apple events to iTerm2. Neither is network access by loupe.

Principle VI: no runtime dependency is added. The host seam already exists (019), and each host is a third and fourth concrete use of it.

Principle VII: tests drive a fake `tmux` and a fake `osascript` on the injected `PATH`, as they drive a fake `herdr` and `orca`. No test starts a tmux server, runs AppleScript or opens a pseudo-terminal. What only a live host can show goes on the Unverified list (FR-018).

### How `review-open` sees an open pane

It does not ask the host, and this feature MUST NOT make it. `review-open` is decided by `run.OpenSession`, which probes the shared lock a running `loupe review` holds on its run directory (`internal/run/session.go`, 036 Key Entities "Review session"). The kernel drops that lock when review exits, whatever pane it ran in. So a tmux or iTerm2 pane needs no pane-id lookup, no `tmux list-panes` and no `display-message`. A pane where review is still running holds the lock and refuses a second hand-off. A pane where review exited, or a pane the human closed, does not.

## Clarifications

### Session 2026-10-28

- Q: What does a tmux session with no client attached get? → A: Detection skips tmux and goes on to Orca, then iTerm2, when the tmux probe succeeds and reports zero attached clients. A probe that fails still refuses `pane-failed` at `probe` (owner, 2026-10-28, option B).

### Decided by the drafting agent, 2026-10-28

- Q: Which host wins when hosts nest? → A: The inner one, as 019 decided for Herdr inside Orca. The order is Herdr, tmux, Orca, iTerm2. Herdr inside tmux gives Herdr, tmux inside Orca gives tmux, and tmux inside iTerm2 gives tmux. tmux inside Herdr gives Herdr, the outer host: its pane still opens where the human sees it, beside the Herdr pane that holds tmux. sift uses the same order.
- Q: Does a tmux split take focus? → A: No. The split passes `-d`, so the agent's pane stays the active pane and the result's `focused` is `false`. A split that takes focus would catch keys the human types for the agent, which is why 036 FR-003 drops review's first keys. Orca already opens without focus.
- Q: How does tmux report the width? → A: The `probe` step reads `#{pane_width}` of the agent's pane, which is the host's own report under 036 FR-005. The split takes tmux's default size, half the pane.
- Q: How does the review command reach the tmux and iTerm2 panes? → A: Each host opens the pane with the user's own shell, then types 019 FR-008's command line into it, as Herdr's `run` step and Orca's `--command` already do. A command passed to `split-window` or to iTerm2's `split ... command` replaces the shell, so the pane would close on a refusal too and break 003 FR-015 (tmux(1), `new-window`: "When the shell command completes, the window closes". iTerm2's scripting page: the command "is run in place of the profile's command"). tmux's `remain-on-exit failed` would keep a failed pane. It needs a second call that races the command, and a tmux recent enough to have the `failed` value.
- Q: How does loupe treat a stale `TMUX`? → A: By what is gone. When the socket path in `TMUX` does not exist, tmux is skipped as a host met only in part, and detection goes on to the next host. When the socket exists but no server answers, the `probe` fails and loupe refuses `pane-failed` at `probe`, which opens nothing.
- Q: Does an iTerm2 split keep focus? → A: loupe asks it to. Right after the split, in the same script, it selects the agent's session again, which iTerm2's scripting page says "makes the session active in its tab". The result's `focused` is `false`. Whether this leaves the view and keyboard focus exactly where they were is unverified (FR-018).
- Q: Is iTerm2 on the Unverified list? → A: Yes, whole. Tests run on Linux and cannot run AppleScript. They pin the calls loupe makes and how it reads the replies, not what iTerm2 does with them.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The agent opens review beside itself in tmux (Priority: P1)

A developer drives an agent from a tmux pane in a plain terminal. When the agent has filed its findings and summary it runs `loupe handoff --run <ref> --json`. A new tmux pane opens beside the agent's pane, running review for that run. The agent's pane keeps focus, and the human moves to the review pane with their usual tmux key. The agent blocks on `loupe wait`. Today the same command refuses `no-pane-host`, and the human types `loupe review` in another terminal.

**Why this priority**: tmux is the terminal multiplexer most terminal users already run, and it works the same on Linux and macOS. It can ship without iTerm2.

**Independent Test**: With a fake `tmux` on `PATH` that records its arguments and returns canned replies, `TMUX` naming a socket file that exists, and `TMUX_PANE=%3`, `loupe handoff` makes the `probe`, `split` and `run` calls in order and returns `host: tmux`, `paneId` as the new pane's id, the direction and `focused: false`.

**Acceptance Scenarios**:

1. **Given** `TMUX` names a socket path that exists, `TMUX_PANE` is non-empty and `tmux` is on `PATH`, and the Herdr conditions are not all met, **When** the agent runs `loupe handoff --run <ref> --json`, **Then** loupe probes the agent's pane, splits it without moving focus, types the review command into the new pane, and prints one result with `host: tmux`, `paneId` set to the new pane's id (for example `%7`), `direction`, and `focused: false`.
2. **Given** the probe reports the agent's pane is 120 columns or wider, **When** the agent hands off, **Then** the pane opens to the right. **Given** it reports a narrower pane, **Then** the pane opens below.
3. **Given** `TMUX_PANE` names a pane that no longer exists, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.host: tmux` and `details.step: probe`, carrying tmux's message, and opens nothing.
4. **Given** `TMUX` names a socket that exists but no server answers on it, **When** the agent hands off, **Then** loupe refuses `pane-failed` at `probe` and opens nothing.
5. **Given** `TMUX` names a socket path that does not exist, and the Orca conditions are met, **When** the agent hands off, **Then** loupe runs no `tmux` command and opens the pane in Orca.
6. **Given** the split's reply carries no pane id, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.step: split` and types nothing.
7. **Given** review in the tmux pane exits cleanly, **When** the shell runs the command's `&& exit`, **Then** the pane closes. **Given** review refuses, **Then** the pane stays open with `error:` and `fix:` on screen.
8. **Given** the probe reports zero clients attached to the agent's session and no later host's conditions are met, **When** the agent hands off, **Then** loupe makes no tmux call after the probe and refuses `no-pane-host`.

---

### User Story 2 - The agent opens review beside itself in iTerm2 (Priority: P2)

A developer on macOS drives an agent from an iTerm2 session, with no multiplexer. The agent hands off, and a split opens beside the agent's session running review. loupe then selects the agent's session again, so keyboard focus stays with the agent.

**Why this priority**: iTerm2 is common on macOS, but only macOS can run it, so nothing here can check what iTerm2 does with loupe's calls. It ships after tmux and its live behavior stays on the Unverified list until the owner walks it.

**Independent Test**: With the operating system injected as macOS, a fake `osascript` on `PATH`, `TERM_PROGRAM=iTerm.app` and `ITERM_SESSION_ID=w0t0p0:<uuid>`, `loupe handoff` makes the `probe`, `split` and `run` calls in order, passing the session id and the review command as script arguments, and returns `host: iterm2`, the new session's id, the direction from the probe's columns and `focused: false`.

**Acceptance Scenarios**:

1. **Given** the operating system is macOS, `TERM_PROGRAM` is `iTerm.app`, `ITERM_SESSION_ID` holds a session id after its colon, `osascript` is on `PATH`, and no earlier host's conditions are all met, **When** the agent hands off, **Then** loupe finds the agent's session by that id, splits it, selects the agent's session again, types the review command into the new session, and prints one result with `host: iterm2`, `paneId` set to the new session's id, `direction`, and `focused: false`.
2. **Given** the probe reports the agent's session is 120 columns or wider, **When** the agent hands off, **Then** the split opens to the right. **Given** it reports fewer, **Then** it opens below.
3. **Given** no iTerm2 session has the agent's id, **When** the agent hands off, **Then** loupe refuses `pane-failed` with `details.host: iterm2` and `details.step: probe` and opens nothing.
4. **Given** macOS refuses loupe's Apple events to iTerm2, **When** the agent hands off, **Then** loupe refuses `pane-failed` at `probe`, carrying the message `osascript` printed, and opens nothing.
5. **Given** the same environment on any operating system but macOS, **When** the agent hands off, **Then** iTerm2 is not detected and loupe runs no `osascript` command.

---

### User Story 3 - Herdr and Orca keep working (Priority: P3)

An owner inside Herdr or Orca hands off as before. A Herdr pane inside tmux still opens in Herdr. An Orca terminal with no tmux in it still opens in Orca.

**Why this priority**: No new value, but the new hosts must not take a hand-off that Herdr or Orca handles today.

**Independent Test**: The existing Herdr and Orca handoff tests pass unchanged, and the detection-order table covers every pair of hosts.

**Acceptance Scenarios**:

1. **Given** the Herdr and tmux conditions are both met, **When** the agent hands off, **Then** loupe uses Herdr and runs no `tmux` command.
2. **Given** the tmux and Orca conditions are both met and the tmux probe shows a client attached to the agent's session, **When** the agent hands off, **Then** loupe uses tmux and runs no `orca` command.
3. **Given** only the Orca conditions are met, **When** the agent hands off, **Then** the calls and result are those of 019.
4. **Given** the tmux and Orca conditions are both met and the tmux probe reports zero attached clients, **When** the agent hands off, **Then** loupe makes no tmux call after the probe and opens the pane in Orca.

### Edge Cases

- **Herdr inside tmux.** Herdr's panes carry both environments, and the agent sits in the Herdr pane. Herdr MUST win.
- **tmux inside Herdr.** Herdr wins, so review opens beside the Herdr pane that holds tmux, not beside the agent's tmux pane. The pane is still where the human sees it. loupe cannot tell this case from Herdr inside tmux by the environment alone.
- **tmux inside Orca, or inside iTerm2.** tmux MUST win. tmux overwrites `TERM_PROGRAM` (019 research, "Orca facts"), so the iTerm2 condition fails inside tmux on its own.
- **iTerm2's tmux integration (`tmux -CC`).** Each tmux pane is an iTerm2 session, and its shell carries `TMUX` and `TMUX_PANE`. tmux wins, and iTerm2 draws the new tmux pane as a native split.
- **A tmux session with no client attached.** A tmux server can run a session nobody is watching, for example one an agent harness starts for its own sub-agents. A pane opened there would be invisible, and the review in it would hold the run's session lock, so every later hand-off would refuse `review-open` until someone kills that pane. So tmux is skipped after a probe that reports zero attached clients, and detection goes on to Orca, then iTerm2 (FR-005). An agent in such a session inside Orca keeps the Orca pane it gets today.
- **The agent's tmux window is not the one on screen.** The split goes into the agent's window and the client keeps showing the window it was on, as an Orca split into a background tab does. The human finds review beside the agent when they return to that window.
- **A zoomed agent pane.** tmux(1) documents `-Z` as keeping a zoomed window zoomed, so a split without it unzooms the window. The human then sees the agent's window split. loupe MUST NOT pass `-Z`, because a zoomed review pane would hide the agent.
- **An environment inherited outside the host.** A program started from a tmux or iTerm2 shell, such as an editor whose terminal is not inside tmux, inherits `TMUX` and `TMUX_PANE`, or `ITERM_SESSION_ID`. loupe then splits the pane that started the program, which is not beside the agent. Herdr and Orca have the same weakness today. Telling them apart needs a process-ancestry check that this feature does not add. iTerm2's `TERM_PROGRAM` condition catches the common case, because the editor's terminal sets its own.
- **A synchronized agent window.** With `synchronize-panes on`, keys sent to the new pane would reach every pane in the window. loupe turns the option off on the new pane before typing (FR-007). The agent's pane and its siblings keep theirs.
- **Stale `TMUX`.** A missing socket skips tmux. A socket with no server behind it fails at `probe`, which opens nothing (Clarifications).
- **A `tmux` client that does not match the server's version.** The probe fails with tmux's own message, and loupe refuses at `probe`.
- **Exactly 120 columns.** The divider takes one column, so one half of a 120-column right split is 59 columns wide. If review gets that half it shows 036's "too small" screen until the human widens it. The 120 threshold is 002's and this feature does not change it.
- **A shell that is not ready when the command is typed.** tmux and iTerm2 queue typed text in the new pane's terminal until its shell reads it, as Orca does. A shell setup that discards typed-ahead input would leave the pane at a prompt with nothing run. That case is unverified (FR-018).
- **Unusual values.** The tmux pane id, the iTerm2 session id and the review command MUST each reach the host as one argument. The iTerm2 script MUST read them from its own arguments and MUST NOT splice them into AppleScript source, so a quote in a data root cannot change what the script does.
- **Split succeeds, a later step fails.** A `run` failure leaves a pane open with a shell and no review. loupe MUST refuse `pane-failed` at `run`, MUST NOT close the pane (006 FR-008), and the skill MUST NOT rerun.

## Requirements *(mandatory)*

### Functional Requirements

#### Detection

- **FR-001**: Detection MUST try Herdr, tmux, Orca and iTerm2 in that order, and MUST use the first host whose conditions are all met. A host whose conditions are met only in part MUST be skipped, not refused. tmux MUST also be skipped when its probe succeeds and reports no client attached to the agent's session (FR-005). Herdr's and Orca's conditions are unchanged from 019 FR-001 and FR-002.
- **FR-002**: loupe MUST detect tmux when `TMUX` is non-empty, its socket path (the text before the first comma) names a file that exists, `TMUX_PANE` is non-empty and an executable `tmux` is found on `PATH`. Detection MUST NOT read `TERM` or `TERM_PROGRAM` for tmux.
- **FR-003**: loupe MUST detect iTerm2 when the operating system is macOS, `TERM_PROGRAM` is `iTerm.app`, `ITERM_SESSION_ID` holds a non-empty session id after its last colon, and an executable `osascript` is found on `PATH`. The operating system MUST be injected into detection, so a test on Linux can detect iTerm2. Rationale for `TERM_PROGRAM`: iTerm2 comes last, so the condition costs nothing inside an earlier host, and it rejects an `ITERM_SESSION_ID` inherited by a program in another terminal.
- **FR-004**: Every tmux call MUST name the server with `-S <socket path>` taken from `TMUX`, so the server loupe talks to is the one detection read. Every tmux call MUST name the agent's pane or the new pane with `-t`.

#### tmux steps

- **FR-005**: tmux MUST open review in the steps `probe`, `split` and `run`. `probe` MUST run `display-message -p -t <TMUX_PANE>` with a format that prints the agent pane's width, the number of clients attached to its session and whether the pane is synchronized, and opens nothing. A probe that exits non-zero or prints an unreadable reply MUST refuse `pane-failed` at `probe`. When the probe succeeds and reports zero attached clients, loupe MUST make no further tmux call and MUST go on detecting from Orca, then iTerm2. When neither is met it MUST refuse `no-pane-host`. Rationale: a pane in a session nobody watches would be invisible, and its review would hold the run's session lock, so every later hand-off would refuse `review-open`. The skip applies only to a probe that succeeds. A failed probe still refuses, so the sandbox rerun rule of 019 FR-017 keeps working, because a sandbox that blocks the tmux socket fails the probe (owner, 2026-10-28).
- **FR-006**: `split` MUST run `split-window -d -P -F '#{pane_id}' -t <TMUX_PANE>` with `-h` for `right` and `-v` for `down`, and no command, so the new pane starts the user's shell. It MUST NOT pass `-Z`, `-b`, `-f` or `-l`. The new pane's id is the trimmed stdout. An empty id MUST refuse at `split`.
- **FR-007**: `run` MUST type 019 FR-008's command line into the new pane as literal text with `send-keys -l`, then press Enter. When the probe reports the agent's pane synchronized, `run` MUST first run `set-option -p -t <new> synchronize-panes off`, so the typed command reaches no other pane. loupe MUST make no other call to the new pane. Rationale: the new pane inherits its window's `synchronize-panes`, and tmux copies keys sent to a synchronized pane, Enter included, to every other synchronized pane in the window, so the agent would get the command as a submitted prompt and each sibling shell would run it (review of #89, 2026-10-29).
- **FR-008**: The direction MUST come from 036 FR-005's rule, with the probe's width as the host's own report. A probe width that is not a positive integer MUST count as unknown, not as a refusal.

#### iTerm2 steps

- **FR-009**: iTerm2 MUST open review in the steps `probe`, `split` and `run`, each one `osascript` call whose script takes its values as arguments. `probe` MUST find the session whose `unique id` is the id from `ITERM_SESSION_ID` and print its `columns`, and opens nothing. `split` MUST split that session with the default profile and no command, `vertically` for `right` and `horizontally` for `down` (iTerm2's scripting page: "a vertical split has a vertical divider"), then `select` the agent's session, and print the new session's `unique id`. `run` MUST `write text` 019 FR-008's command line into the new session. Rationale for selecting in `split`: if the split takes keyboard focus, keys the human types before the agent's session is selected again land in the new shell, ahead of the typed command. Selecting in the same script as the split keeps that gap to the one call (review of #89, 2026-10-29).
- **FR-010**: The direction MUST come from 036 FR-005's rule, with the probe's columns as the host's own report.
- **FR-011**: loupe MUST NOT call `activate` on iTerm2, and MUST NOT select a window or a tab, so the split moves neither the view nor the frontmost application.

#### Result, help and contract

- **FR-012**: The `no-pane-host` message MUST name what each host needs, in detection order. Herdr needs `HERDR_ENV=1`, `HERDR_PANE_ID` and `herdr` on `PATH`. tmux needs `TMUX`, `TMUX_PANE`, `tmux` on `PATH`, and a client attached to the agent's session. Orca needs `ORCA_TERMINAL_HANDLE` and `orca` on `PATH`. iTerm2 needs macOS, `ITERM_SESSION_ID` and `osascript` on `PATH`.
- **FR-013**: On success the result's `host` MUST be `herdr`, `orca`, `tmux` or `iterm2`. `paneId` MUST be the new tmux pane's id or the new iTerm2 session's id. `focused` MUST be `false` on tmux and iTerm2. A `pane-failed` refusal from either MUST carry `details.host` and `details.step` under 019 FR-011, with the host's message taken from stderr, trimmed.
- **FR-014**: `loupe handoff --help` MUST list the four hosts with their conditions in detection order, and say that tmux and iTerm2 open the pane without focus. The `handoff-help` golden MUST be regenerated and its diff read. The `handoff` section and the `pane-failed` row of `contracts/cli.md` MUST name the four hosts, the order and the `host` values.
- **FR-015**: Host logic MAY live only in `internal/pane`. `scripts/check-tests.sh` MUST also fail when a non-test Go file outside `internal/pane` and `internal/cli/handoff.go` names `tmux`, `iterm` or `osascript`. `internal/style` reads `TMUX` to decide terminal colors, and the guard MUST allow that one read by name rather than by excluding the package.

#### Skill and documentation

- **FR-016**: The `human-review` skill MUST name no `tmux` or `osascript` command. The test that pins the skill against `herdr` and `orca` commands MUST also cover both. The sandbox rerun rule keyed on `details.step: probe` (019 FR-017) MUST need no change.
- **FR-017**: The README's "Handing off in a terminal pane" section MUST name the four hosts. The `AGENTS.md` layout row for `internal/pane` MUST say it opens review in a Herdr, tmux, Orca or iTerm2 pane and is the only package that runs any of them.

#### Validation

- **FR-018**: `specs/001-loupe-v1/validation.md` MUST record the new tests. Its Unverified list MUST gain each of these until it runs live:
  - A tmux handoff that opens `right` and `down`, keeps the agent's pane active, closes on a clean exit and stays open on a refusal.
  - The detached-session case of FR-005, and a stale socket with no server behind it.
  - The typed command reaching a freshly started shell under zsh, bash and fish, on tmux and on iTerm2.
  - On macOS, every iTerm2 behavior FR-009 to FR-011 assume: that the id after the colon in `ITERM_SESSION_ID` is the session's `unique id`, `columns`, the split direction words, `write text`, whether the split takes keyboard focus before `select`, that `select` keeps the view and keyboard focus where they were, whether macOS asks the human to allow the Apple events, and the pane closing on exit.

### Key Entities

- **Pane host**: A terminal that can open a pane beside the agent's for review. Herdr, tmux, Orca and iTerm2, detected in that order. tmux counts only when a client is attached to the agent's session.
- **tmux pane id**: The `%N` id tmux gives every pane and passes to the pane's shell as `TMUX_PANE` (tmux(1)).
- **iTerm2 session id**: The UUID after the colon in `ITERM_SESSION_ID`, which iTerm2's scripting dictionary calls the session's `unique id`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Inside tmux, a full review round (agent files findings, human decides, sends notes back, agent revises, human publishes) needs no command typed by the human beyond moving to the review pane, as it already does inside Herdr.
- **SC-002**: On tmux and on iTerm2, a failure at the probe step opens no pane and makes no further host call, shown by a test per host.
- **SC-003**: The detection-order test covers every pair of hosts with both conditions met, a stale tmux socket, a tmux probe that reports zero attached clients with and without the Orca conditions met, and iTerm2's conditions on Linux and on macOS, and picks the host FR-001 names in each.
- **SC-004**: With a probe width of 120 and 119 columns, and an unreadable width with no terminal, tmux and iTerm2 open `right`, `down` and `down`, shown by tests.
- **SC-005**: No test starts a tmux server, runs AppleScript or opens a pseudo-terminal, and `mise run check` passes after the final edit, including the widened host guard.

## Assumptions

From tmux(1), tmux 3.4 on this machine:

- `split-window -h` does a horizontal split and `-v` a vertical one. `-b` puts the new pane "to the left of or above" the target, so without it the new pane goes to the right or below. `split-window` takes its other options from `new-window`, where `-d` means the new window does not become the current one. That `-d` keeps the agent's pane active is read from that wording, not stated for panes. `-P -F <format>` prints the new pane's details in that format. `-Z` keeps a zoomed window zoomed. A pane created with no command runs `default-command`, else the user's shell. "When the shell command completes, the window closes."
- `display-message -p` prints to stdout. `pane_width`, `pane_id` and `session_attached` ("Number of clients session is attached to") are documented formats. `send-keys -l` sends its text as literal characters.
- tmux passes the pane id to the pane's shell as `TMUX_PANE`, sets `TMUX` "with some internal information", and sets `TERM` to `screen` or `tmux`. That `TMUX` starts with the socket path and a comma is how tmux has always written it and how sift reads it. The plan MUST record one live sample.

From iTerm2's scripting page, whose AppleScript section iTerm2 now marks deprecated in favor of its Python API:

- A session has `unique id`, `columns`, `rows` and `tty`. `split vertically` and `split horizontally` take a profile and an optional command and return the new session. `write text` types text "as though you had typed it". `select` "makes the session active in its tab". The Python API was rejected because it must be turned on by the human and needs a Python runtime.

From sift and 019's research, not from documentation:

- iTerm2 sets `ITERM_SESSION_ID=w<N>t<N>p<N>:<UUID>` and `TERM_PROGRAM=iTerm.app` in its sessions. tmux overwrites `TERM_PROGRAM`.

Also:

- The three shells 003 already requires run 019 FR-008's command line, so typing it into a tmux or iTerm2 pane adds no shell requirement.
- No commit, push, pull request, release, live review, or run of `tmux` or `osascript` against a live session is authorized by this specification.
