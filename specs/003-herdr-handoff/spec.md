# Feature Specification: Herdr review handoff

**Feature Branch**: `003-herdr-handoff`

**Created**: 2026-09-15

**Status**: Draft

**Input**: User description: "Herdr integration for loupe. (1) Skill change: when running inside herdr, the agent MAY open `loupe review <ref>` for the human in a new herdr split, then block on `loupe wait` as today; after answering send-back notes it opens a fresh split each time. The agent MUST NOT send keys or text to, or read, that pane; `loupe publish` stays human-only; outside herdr the existing handoff is unchanged. This replaces the skill's blanket 'MUST NOT run loupe review' rule. (2) A herdr plugin shipped in this repo with a pane entrypoint running `loupe review` and a keybound action so the human can open review themselves, choosing from open runs. No loupe binary code path may depend on herdr. Live reload in the review TUI is out of scope. Builds on specs/002-review-ux."

## Relationship to earlier specifications

This specification amends the handoff in the Claude Code plugin's `loupe` skill (sections "Rules" and "6. Hand off and wait"). A Herdr plugin with a run picker was also requested; it is deferred (see Deferred).

It relies on `specs/002-review-ux`, which requires `loupe review <run>` to behave identically in a Herdr split and forbids any Herdr-dependent runtime path (UX-013). That requirement stays in force here.

The constitution, the CLI contract in `specs/001-loupe-v1/contracts/cli.md`, the draft model, the publication state machine and the published comment format remain authoritative and unchanged.

### Why the human-only rule can relax without a constitution amendment

Principle II forbids agents from publishing and requires the human to confirm the review in one interactive command. It does not forbid an agent from opening the interface for the human.

The skill's rule "MUST NOT run `loupe review`" was stricter than the constitution so that an agent could not drive decisions through a terminal it controls. A Herdr pane is such a terminal: an agent that opens it can also type into it. After this change, the guarantee that the agent does not decide findings rests on the skill's instructions, not on any mechanism. This was already true of any agent with Herdr or tmux access; this specification makes the boundary explicit rather than implicit.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The agent opens review beside itself (Priority: P1)

A human runs `/loupe <pr-url>` in Claude Code inside Herdr. When the agent has filed its findings and summary, a split opens next to the agent pane with the review interface already showing that run, and focus moves to it. The human decides each finding. If they send notes back and quit, the pane closes, the agent wakes, answers the notes, and opens a fresh split with the revised run. When the human publishes from review, the agent wakes and stops.

**Why this priority**: This removes the only manual step in the loop today: switching terminals and typing `loupe review <ref>`. It is also the smallest slice, needing only a skill change.

**Independent Test**: Inside Herdr, run the skill against a captured run with findings filed; confirm a split opens showing that run, that quitting with a note wakes the agent, and that the agent's second handoff opens a new split.

**Acceptance Scenarios**:

1. **Given** the agent runs inside Herdr and has set the summary, **When** it hands off, **Then** a new split running `loupe review <ref>` for that run opens beside the agent pane with focus, and the agent then blocks on `loupe wait --run <ref> --json`.
2. **Given** the review split is open, **When** the human quits review leaving a note for the agent, **Then** the review pane closes, `loupe wait` returns `reason: notes`, and the agent answers only the awaiting notes.
3. **Given** the agent has replied to every awaiting note, **When** it hands off again, **Then** it opens a new split for the same run rather than reusing or searching for the earlier pane.
4. **Given** the human publishes from the review split, **When** `loupe wait` returns `reason: published`, **Then** the agent stops and opens no further pane.
5. **Given** the agent does not run inside Herdr, **When** it hands off, **Then** it tells the human to run `loupe review <ref>` in their own terminal, exactly as today.

---

### Edge Cases

- **Herdr control fails.** The agent is inside Herdr but the split cannot be opened (Herdr CLI missing, socket unreachable, a sandbox blocks it, or the command errors). The agent MUST fall back to the non-Herdr handoff, telling the human the error in one line, and still block on `loupe wait`.
- **Review refuses.** `loupe review` exits with a refusal (for example `tty`, `lock`, or a stale draft). The pane MUST stay open showing the `error:` and `fix:` lines until the human dismisses it; it MUST NOT vanish with the message.
- **Review exits normally.** The pane closes on its own. The agent MUST NOT close, read or inspect it to find out.
- **Human closes the pane mid-review.** Decisions already made are persisted by review's immediate persistence. `loupe wait` keeps waiting, because no note was handed back and nothing was published; the human can reopen with `loupe review <ref>`.
- **Two review panes on one run.** A human opens review in another terminal while an agent-opened pane is already showing the same run. Both are ordinary `loupe review` processes; the existing lock and stale-version protection govern them and no new handling is added.
- **Narrow agent pane.** The split direction MUST leave the review pane at least the 60 by 12 cells spec 002 requires where the agent pane allows it: split right when the agent pane is wide enough, down otherwise.
- **Human is elsewhere in Herdr.** Opening the split moves focus to it even if the human is looking at another pane in the tab. Handoff is the moment the agent needs the human, so this is intended.
- **Typing when the split opens.** The split can take focus while the human is still typing into the agent pane. `loupe review` MUST drop keys that arrive within 500 milliseconds of its start, so typeahead cannot open a finding, decide it or quit. Ctrl+C still ends review at once. The guard names no multiplexer (FR-017).
- **Several agents.** Each agent opens splits beside its own pane only, never beside the UI-focused pane of another client.

## Requirements *(mandatory)*

### Functional Requirements

#### Skill handoff

- **FR-001**: The skill MUST detect Herdr only from the environment Herdr injects into managed panes, and MUST use the Herdr handoff only when detected.
- **FR-002**: Inside Herdr, at each handoff the agent MAY open a new split beside its own pane running `loupe review <ref>` for the run, with focus on the new pane.
- **FR-003**: The agent MUST open the split before blocking on `loupe wait`, and MUST block on `loupe wait --run <ref> --json` under the same Monitor or foreground rules as today.
- **FR-004**: After answering send-back notes, the agent MUST open a new split for the next handoff and MUST NOT look for, reuse, or close an earlier review pane.
- **FR-005**: The agent MUST NOT send keys or text to, read output from, resize, or close any pane running `loupe review` or `loupe publish`.
- **FR-006**: The agent MUST NOT open `loupe publish` in a pane or by any other route. Publication happens only inside review or from a command the human types.
- **FR-007**: The skill's blanket prohibition on running `loupe review` MUST be replaced by the narrower rules FR-002 to FR-006. The prohibitions on allocating a pseudo-terminal, piping confirmation, and posting reviews by other routes MUST remain, with the Herdr pane named as the only exception to the pseudo-terminal rule.
- **FR-008**: If opening the split fails, the agent MUST report the failure in one line and fall back to telling the human to run `loupe review <ref>`.
- **FR-009**: Outside Herdr, the handoff MUST be unchanged from the current skill.

#### Review pane

- **FR-015**: A review pane opened by the skill MUST close when review exits successfully and MUST stay open until dismissed when review exits with a refusal or error.

#### Boundaries

- **FR-017**: No code path in the loupe binary MAY depend on Herdr or read Herdr environment variables (constitution I; spec 002 UX-013). Superseded on 2026-09-15 by `specs/006-agent-plugins` FR-010, which moves the handoff into `loupe handoff`.
- **FR-018**: The README MUST note, next to the Claude Code plugin instructions, that inside Herdr the skill opens review in a split, and name the Bash permission patterns a user MAY allow for it. Superseded on 2026-09-15 by `specs/006-agent-plugins` FR-024: the README names `loupe handoff` allow rules and no `herdr` rule.
- **FR-020**: A live check of the skill handoff inside Herdr MUST be added to the Unverified list in `specs/001-loupe-v1/validation.md`, since tests MUST NOT use a real terminal or a real Herdr.

### Key Entities

- **Review pane**: A Herdr pane whose shell runs `loupe review <ref>`. Opened by the agent; owned by the human once open.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Inside Herdr, a full review round (agent files findings, human decides, human sends notes back, agent revises, human publishes) needs no command typed by the human.
- **SC-003**: Every refusal from review stays visible until the human dismisses it.
- **SC-004**: Outside Herdr, the skill's handoff output is identical to the current skill's.
- **SC-005**: No file under `cmd/` or `internal/` references Herdr, and all automated repository checks pass after the final edit.

## Assumptions

Observed in Herdr 0.9.0 on 2026-09-15:

- Herdr injects `HERDR_ENV=1`, `HERDR_PANE_ID` and `HERDR_SOCKET_PATH` into managed panes.
- `herdr pane split --pane <id> --direction right|down --focus` opens a shell beside that pane and takes no command. Its JSON result carries `result.pane.pane_id`. `--env KEY=VALUE` sets a variable in the new shell, which otherwise loads the user's profile rather than the caller's environment. API errors print JSON on stderr and exit 1.
- `herdr pane run <pane-id> <command>` types the command and Enter into that pane's shell.
- `herdr pane layout --pane <id>` reports every pane in the caller's tab with `rect.width` and `rect.height`.
- A split pane closes when its shell exits, and stays open while the shell lives. So `loupe review '<ref>' && exit` meets FR-015: success closes the pane and a refusal leaves `error:` and `fix:` on screen. This needs a shell with `&&` (zsh, bash, fish 3+; not nushell).
- The ref MUST be single-quoted: zsh with `EXTENDED_GLOB` treats `#` as a glob operator.
- Claude Code's Bash tool reaches the Herdr socket from inside the pane. Where a sandbox or permission rule blocks it, FR-008's fallback applies.

Also:

- Live reload in the review interface, reusing panes, notifications, and integrations with multiplexers other than Herdr are out of scope.
- This feature SHOULD land after spec 002 so review looks right in a split, but does not depend on it to function.
- No commit, push, pull request, release, live review, or plugin publication is authorized by this specification.

## Deferred

The Herdr plugin with a run picker is deferred. It adds nothing to the agent handoff, and it carries most of the cost and open questions: a popup interface, the picker's language, what context a plugin action receives, and keeping a TOML manifest version in step with releases. The material below is kept for a later specification.

### Deferred Story 2 - The human opens review from a Herdr keybinding (Priority: P2)

A human with the loupe Herdr plugin installed presses a key. A popup lists the runs that are not yet published, newest capture first, each showing the pull request title, `owner/repo#number`, round, and pending and open-note counts. The human picks one and review opens for it in a split. This works whether or not an agent is involved, for example when returning to a review after a break.

**Why this priority**: It covers re-entry and agent-less use, but the P1 flow already reaches the main value.

**Independent Test**: With the plugin linked and two unpublished runs captured, press the bound key, choose the older run, and confirm review opens for that run.

**Acceptance Scenarios**:

1. **Given** two or more unpublished runs, **When** the human invokes the plugin's review action, **Then** a picker lists every unpublished run newest capture first with title, reference, round, pending count and open-note count.
2. **Given** the picker is showing, **When** the human chooses a run, **Then** the picker closes and `loupe review <ref>` for that run opens in a split with focus.
3. **Given** the picker is showing, **When** the human cancels, **Then** the picker closes and nothing else opens.
4. **Given** exactly one unpublished run, **When** the human invokes the action, **Then** review opens for that run without showing the picker.
5. **Given** no unpublished runs, **When** the human invokes the action, **Then** the human sees that there is nothing to review and how to capture a run, and no review pane opens.

---

### Deferred Story 3 - Install and bind the plugin (Priority: P3)

A human installs the plugin from this repository with Herdr's installer, binds a key to its review action in their Herdr configuration, and finds both steps in the README next to the Claude Code plugin instructions.

**Why this priority**: Required for Story 2 to reach anyone, but it is documentation and packaging.

**Independent Test**: Follow the README on a machine with Herdr and loupe installed; the plugin appears in Herdr's plugin list and the bound key runs the review action.

**Acceptance Scenarios**:

1. **Given** Herdr 0.9.0 or later, **When** the human runs the documented install command, **Then** the plugin installs from a subdirectory of this repository and is listed as enabled.
2. **Given** the plugin is installed, **When** the human adds the documented key binding, **Then** pressing it invokes the review action.

---

### Deferred requirements

- **FR-010**: The repository MUST ship a Herdr plugin in its own subdirectory, installable with Herdr's `plugin install` from GitHub shorthand, declaring Herdr 0.9.0 as its minimum version.
- **FR-011**: The plugin MUST provide a review action that lists unpublished runs, lets the human choose one, and opens `loupe review <ref>` for the chosen run in a split with focus.
- **FR-012**: The picker MUST show, per run, the pull request title, reference, round, pending count and open-note count, newest capture first, and MUST support cancel.
- **FR-013**: The action MUST skip the picker when exactly one run is unpublished, and MUST report that there is nothing to review, with the capture command, when none is.
- **FR-014**: The plugin MUST obtain runs only through `loupe list --json` and MUST open review only through `loupe review <ref>`; it MUST NOT read loupe's run files directly.
- **FR-016**: The plugin MUST NOT invoke `loupe publish` or any agent-facing loupe command other than `loupe list`.
- **FR-015** (plugin half): a review pane opened by the plugin MUST close when review exits successfully and MUST stay open until dismissed when review exits with a refusal or error.
- **FR-019**: The plugin's manifest version MUST be kept in step with releases by the existing release automation rather than edited by hand.
- **FR-017** (picker half): if a later plan moves any picker behavior into the binary, it MUST be host-agnostic, reachable from `loupe --help`, and recorded as an amendment to `contracts/cli.md`.
- **Edge case**: if `loupe list --json` refuses or fails, the popup MUST show loupe's error and fix and open nothing.
- **Unpublished run**: a run whose `state` in `loupe list --json` is not `published`. The set the picker offers.
- **SC-002**: From pressing the plugin key, the human reaches a chosen run's review in at most two interactions (the key, then a choice), and in one when a single run is open.
- Herdr's plugin manifest supports `[[actions]]`, `[[panes]]` with `split` and `popup` placements, argv commands run without a shell and with the plugin directory as working directory, and key binding via `[[keys.command]]` with `type = "plugin_action"`. A plan MUST confirm what context (focused pane, cwd) an action receives.
- The picker's implementation language and whether it runs in a Herdr popup or inside the split is a plan decision, constrained by constitution VI and FR-017.
- Release automation (release-please) can update a TOML version field; if not, a plan MUST record the alternative.
