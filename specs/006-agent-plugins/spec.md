# Feature Specification: loupe handoff and agent plugins

**Feature Branch**: `006-agent-plugins`

**Created**: 2026-09-15

**Status**: Draft

**Input**: Owner request: "loupe owns the review hand-off and ships as a plugin to Claude Code, Codex and Pi." A `loupe handoff` command opens review for the human in a Herdr split; the plugin's one skill is the loupe workflow any review skill can follow; one plugin route per harness, versioned by the release. Ruled by the owner on 2026-09-15: loupe is workflow tooling and ships no review skill or command.

## Relationship to earlier specifications

This specification is stacked on `specs/004-agent-hosts` and amends three earlier ones:

- **`specs/003-herdr-handoff`**: the skill composed three `herdr` calls. That handoff moves into the binary as `loupe handoff` (FR-001 to FR-009). FR-017 ("no code path in the binary MAY depend on Herdr") is superseded by FR-010. FR-018 is superseded by FR-024. The rest of 003 stays in force, including FR-015's pane lifetime and the reasoning in "Why the human-only rule can relax".
- **`specs/002-review-ux`**: UX-013 ("no runtime path MAY depend on Herdr") is superseded by FR-010. `loupe review` itself still has no Herdr path.
- **`specs/004-agent-hosts`**: FR-005 (`npx skills add`) and FR-007 (no Codex plugin manifest), and its Deferred Codex plugin and Pi package, are superseded by FR-018 to FR-021. Its Agent Skills limits (FR-002) now apply to every skill, and its sandbox rule (FR-003) names `loupe handoff` in place of `herdr`.

The CLI contract in `specs/001-loupe-v1/contracts/cli.md` gains one command and two refusal codes. The draft model, the publication state machine and the published comment format are unchanged.

### Why this fits the constitution

Principle I says every workflow MUST be completable from `loupe --help` alone and host integrations MUST NOT add code paths only one host can reach. Today the Herdr handoff exists only in skill text, so an agent without the plugin cannot find it. `loupe handoff` is reachable by any agent in any harness, from `--help`. Herdr is the terminal the human sits in, not the agent host.

Principle II is unchanged in substance. The agent opens review; only the human decides findings and publishes. That this rests on instructions, not a mechanism, is recorded in spec 003 and still holds.

Principle III allows no network beyond Git and GitHub. `herdr` talks to a local socket, which is not network access.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One command opens review beside the agent (Priority: P1)

An agent inside Herdr has filed findings and a summary. It runs `loupe handoff --run <ref> --json`. A split opens beside the agent's pane with review showing that run, and focus moves to it. The agent blocks on `loupe wait`. Outside Herdr, the same command refuses and tells the agent to ask the human to run review.

**Why this priority**: It is the whole point: one command an allow rule can name, instead of three `herdr` calls that cannot be allowed safely, quoting a user-controlled ref through zsh.

**Independent Test**: With a fake `herdr` on `PATH` that records its arguments and returns canned JSON, `loupe handoff` makes the layout, split and run calls in order and returns the pane id and direction. With `HERDR_ENV` unset it refuses `no-pane-host`.

**Acceptance Scenarios**:

1. **Given** `HERDR_ENV=1`, `HERDR_PANE_ID` set and `herdr` on `PATH`, **When** the agent runs `loupe handoff --run <ref> --json`, **Then** loupe reads the agent pane's width, opens a split beside that pane with focus, starts `loupe review` for the resolved run in it, and prints one result with `host`, `paneId` and `direction`.
2. **Given** the agent pane is at least 120 columns wide, **When** it hands off, **Then** the split opens to the right; otherwise it opens below.
3. **Given** any of the three Herdr conditions is missing, **When** the agent runs `loupe handoff`, **Then** it refuses `no-pane-host` with exit 1 and a fix telling the agent to ask the human to run `loupe review '<ref>'`, and runs no `herdr` command.
4. **Given** a `herdr` call exits non-zero or its result lacks the field loupe needs, **When** the agent hands off, **Then** loupe refuses `pane-failed` with Herdr's message and the same fix, and makes no further `herdr` call.
5. **Given** the run cannot be resolved, **When** the agent runs `loupe handoff`, **Then** it refuses exactly as `loupe review` would for the same selection, and runs no `herdr` command.

---

### User Story 2 - Any review skill follows the loupe workflow (Priority: P2)

A review skill outside loupe, such as one that fans out reviewers, has findings for a pull request. It follows the plugin's `human-review` skill, which captures the pull request, files the findings and summary, hands off, waits, answers send-back notes and hands off again until the human publishes. The skill brings no review method: finding and judging belongs to the caller.

**Why this priority**: The workflow is duplicated today in another repository's skills. A skill that ships with loupe gives any reviewer the same contract and rules.

**Independent Test**: The plugin tests pin the skill's commands and rules, that it names no `herdr` command, that it leaves the review method to its caller, and that the plugin ships no command.

**Acceptance Scenarios**:

1. **Given** a run with findings and a summary, **When** an agent follows the `human-review` skill to its hand-off, **Then** it runs `loupe handoff --run <ref> --json`, and on a refusal tells the human to run `loupe review '<ref>'` in their own terminal.
2. **Given** either outcome of the handoff, **When** it is done, **Then** the agent blocks on `loupe wait --run <ref> --json`, under a Monitor where the harness has one and in a foreground `--timeout` loop otherwise.
3. **Given** `loupe wait` returns `reason: notes`, **When** the agent has replied to every awaiting note, **Then** it hands off again with a new `loupe handoff`.
4. **Given** `loupe wait` returns `reason: published`, **Then** the agent stops.

---

### User Story 3 - Install loupe's plugin in Claude Code, Codex or Pi (Priority: P3)

The owner installs loupe's skill with the harness's own plugin command. Claude Code's route is unchanged. Codex adds the repository as a plugin marketplace. Pi installs the repository as a package, pinned to the release tag that matches the installed binary.

**Why this priority**: Distribution, not behavior. One route kind for every harness keeps the README and the release consistent.

**Independent Test**: The plugin tests parse every manifest, check each version equals the Claude Code manifest's, and check each points at `plugin/skills`. A live install is owner-only (FR-025).

**Acceptance Scenarios**:

1. **Given** Codex 0.154 or later, **When** the owner runs the README's `codex plugin marketplace add` and `codex plugin add` commands, **Then** Codex lists the `human-review` skill.
2. **Given** Pi 0.84 or later, **When** the owner runs the README's `pi install git:…@<tag>`, **Then** Pi lists the `human-review` skill.
3. **Given** a release, **When** release-please bumps versions, **Then** every plugin manifest carries the release version.

### Edge Cases

- **Relative data root.** `LOUPE_HOME` is relative. The split's shell starts in a different directory, so loupe MUST pass the absolute path.
- **Unusual paths.** The loupe executable's path or the ref contains characters a shell treats specially. Both MUST reach the pane's shell as literal words.
- **Several panes in the tab.** The layout lists every pane in the tab. loupe MUST read the entry whose id is the agent's own.
- **Review refuses in the pane.** The pane stays open with `error:` and `fix:` (spec 003 FR-015). `loupe handoff` has already succeeded; it does not watch the pane.
- **A second handoff.** Each call opens a new split. loupe MUST NOT look for, reuse or close an earlier pane.
- **Published run.** `loupe handoff` does not check readiness or publication. Review shows its own state or refusal in the pane.
- **Stale `HERDR_ENV`.** A shell started from a Herdr pane into another terminal can inherit `HERDR_ENV`. Detection does not guard against it; the `herdr` call then fails and loupe refuses `pane-failed`.
- **Sandboxed harness.** Codex's default sandbox blocks the Herdr socket. The skill's sandbox rule tells the agent to rerun `loupe handoff` with the harness's approval.

## Requirements *(mandatory)*

### Functional Requirements

#### `loupe handoff`

- **FR-001**: loupe MUST provide an agent command `loupe handoff [<ref>]` with `--run <ref>` and `--json`, selecting the run exactly as `loupe review` does.
- **FR-002**: The command MUST detect Herdr only when `HERDR_ENV` is `1`, `HERDR_PANE_ID` is non-empty and `herdr` is found on `PATH`. Otherwise it MUST refuse `no-pane-host`.
- **FR-003**: The command MUST read the agent pane's width from Herdr's layout, matching the pane by `HERDR_PANE_ID`, and MUST choose `right` when it is at least 120 columns and `down` otherwise.
- **FR-004**: The command MUST open one split beside the agent pane with focus, setting `LOUPE_HOME` in it to the absolute data root the command itself resolved.
- **FR-005**: The command MUST start, in that split's shell, the running loupe executable with `review` and the resolved run's canonical reference, followed by `&& exit`. The executable path and the reference MUST each reach the shell as one literal word, single-quoted whenever it holds a character a shell could expand. The reference MUST come from the resolved run, never from agent text.
- **FR-006**: Any `herdr` call that exits non-zero, prints unparsable output, or lacks a needed field MUST refuse `pane-failed`, carrying Herdr's message. The command MUST NOT retry, and MUST make no `herdr` call after the failed one.
- **FR-007**: Both refusals MUST exit 1 with the fix `ask the human to run loupe review '<ref>'`, naming the resolved reference when the run resolved.
- **FR-008**: The command MUST NOT look for, reuse, resize, read or close any pane.
- **FR-009**: On success the `--json` result MUST carry `host` (`herdr`), `paneId` and `direction`, in the standard envelope. Without `--json` it MUST print one line naming the run and the direction.

#### Contract and help

- **FR-010**: Herdr logic MAY live only in one internal package of the binary. Outside that package and its tests, only the `handoff` command's file MAY name Herdr, in its help and its `no-pane-host` refusal. `scripts/check-tests.sh` MUST fail when any other non-test Go file under `cmd/` or `internal/`, or `go.mod`, names Herdr.
- **FR-011**: `contracts/cli.md` MUST list `handoff`, its result, and the `no-pane-host` and `pane-failed` codes. The refusal code table test MUST match.
- **FR-012**: `loupe --help` MUST state that `review` and `publish` are human-only; that an agent MUST NOT operate them, pipe confirmation into them, drive them through a pseudo-terminal, or start `publish` by any route; that an agent MAY run `loupe handoff` to start review in a new pane, and MUST NOT then send to, read, resize, close or reuse that pane; and that otherwise it tells the human to run `loupe review`.
- **FR-013**: `loupe review --help` MUST carry the same rule for review, and `loupe handoff --help` MUST describe detection, the direction rule, both refusals and the result.

#### Skills

- **FR-014**: The plugin MUST carry exactly one skill, `plugin/skills/human-review/SKILL.md`: the loupe workflow for a skill or agent that has review findings. It covers capture and the previous round before the caller's review, then filing findings, the summary, the hand-off through `loupe handoff`, the wait and send-back notes, with the rules on `loupe publish`, pseudo-terminals, confirmation, the review pane, other review routes, running the pull request's code and the user's checkout, refusals and the sandbox. Its sandbox rule MUST allow rerunning `loupe handoff` only after a failure at the layout step, which opens no pane.
- **FR-015**: The skill MUST NOT prescribe a review method, and MUST NOT name any `herdr` command.
- **FR-016**: The skill MUST meet the Agent Skills limits of spec 004 FR-002, and every line except the Monitor instruction MUST stay harness-neutral.
- **FR-017**: The sandbox rule MUST name `loupe handoff` as the command that needs the Herdr socket, and MUST NOT name `herdr`.

#### Plugins

- **FR-018**: The Claude Code plugin and marketplace stay, and the plugin MUST ship no command: `commands/loupe.md`, which started a review, is removed.
- **FR-019**: A Codex plugin manifest at `plugin/.codex-plugin/plugin.json` and a Codex marketplace at `.agents/plugins/marketplace.json` MUST expose `plugin/skills`.
- **FR-020**: A root `package.json` MUST make the repository a Pi package whose skills are `plugin/skills` and nothing else.
- **FR-021**: release-please MUST bump the version in every plugin manifest. A test MUST fail when any manifest's version differs from the Claude Code manifest's.
- **FR-022**: No `npx skills` install route MAY remain in the README.

#### Documentation and validation

- **FR-023**: The README MUST list `handoff` in the Commands table and show install commands for Claude Code, Codex and Pi.
- **FR-024**: The README MUST name the allow rules for `loupe handoff`: `Bash(loupe handoff:*)` for Claude Code and a `prefix_rule` for `["loupe", "handoff"]` for Codex. It MUST NOT recommend allowing any `herdr` command.
- **FR-025**: The Unverified list in `specs/001-loupe-v1/validation.md` MUST gain: a live `loupe handoff` in Herdr under Claude Code, Codex and Pi; the Codex and Pi plugin installs from the repository; and whether the two allow rules remove the prompt.

### Key Entities

- **Pane host**: The terminal multiplexer that can open a pane for review. Herdr is the only one.
- **Handoff result**: `host`, `paneId`, `direction`, and the run reference.
- **Plugin manifest**: One per harness, each pointing at `plugin/skills` and carrying the release version.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Inside Herdr, handing off takes one agent command where it took three, and that command can be allowed by one rule in Claude Code and one in Codex.
- **SC-002**: A reference containing `#` and `@` reaches the review pane as one literal word, shown by a test.
- **SC-003**: Every plugin manifest carries the same version after a release, shown by a test.
- **SC-004**: `mise run check` passes after the final edit, including the narrowed Herdr guard.

## Assumptions

- Herdr 0.9.0 behavior observed for spec 003 still holds: `pane layout`, `pane split … --focus --env` returning `result.pane.pane_id`, `pane run <id> <command>`, JSON on stdout, and JSON errors on stderr with exit 1.
- Codex 0.154 installs plugins with `codex plugin marketplace add <owner/repo>` and `codex plugin add <plugin>@<marketplace>`, and allows commands with `prefix_rule(pattern=[…], decision="allow")` in `~/.codex/rules/default.rules`. Whether an allowed command also runs outside the sandbox is unverified.
- Pi 0.84 installs a git package with `pi install git:<host>/<owner>/<repo>@<ref>` and reads skills from the `pi.skills` paths in its root `package.json`.
- The follow-up that removes the copied handoff from another repository's skills is out of scope.
- No commit, push, pull request, release or live run is authorized by this specification.
