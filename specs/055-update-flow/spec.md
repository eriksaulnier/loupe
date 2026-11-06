# Feature Specification: Install and update the binary and the agent plugins together

**Feature Branch**: `update-flow`

**Created**: 2026-11-06

**Status**: Draft

**Input**: Owner decision, 2026-11-06. The install script installs only the binary and then links to the plugin docs. A person adds the plugin by hand for each agent, from `main`. So a plugin is forgotten, or never added to an agent installed later. The plugin also tracks `main` while the binary tracks a release, so a skill can name a flag that the binary lacks. Owner: the script installs and updates the plugins for each agent it finds, at the binary's tag. A `loupe update` command runs the same flow. The update warning names that command. There is no silent auto-update.

## Relationship to earlier specifications

- Supersedes the line in `specs/048-update-warning` that put a `loupe update` command out of scope, and amends its FR-006: the warning names `loupe update` instead of `docs/install.md`.
- Extends `specs/047-install-script`: the script gains a plugin step and one environment variable. The binary install, its refusals and its output before the plugin step do not change.
- Changes `specs/006-agent-plugins` in one respect: the documented Claude Code and Codex install pins the plugin to a release tag instead of `main`.
- `specs/001-loupe-v1/contracts/cli.md` gains the `update` command, its result and its refusal codes. Each is additive.
- `docs/comment-format.md` does not change.
- Constitution 3.2.0 becomes 4.0.0, a MAJOR amendment. Principle III gains loupe's own release downloads. Principle I states that managing loupe's own plugin through a host's plugin command is not invoking a reviewer.

## Clarifications

### Session 2026-11-06

- Q: Does the script install the plugins, or only point at them? → A: Owner: it installs and updates them by default, for each of Claude Code, Codex and Pi that it finds on `PATH`.
- Q: Which plugin version? → A: Owner: the release tag of the binary that the script installed. The skills then always match the binary.
- Q: Auto-update, an update command, or rerunning the script? → A: Owner: an explicit `loupe update` command, plus the existing warning. No silent auto-update. Capture runs inside an agent session, and an update there would swap the skill under a running agent and run downloaded code with no human present.
- Q: How does this square with the constitution? → A: Owner: an amendment to Principles I and III, rather than a per-feature exception. It is MAJOR, because widening Principle III redefines it, as 3.0.0 did for Principle II.
- Q: How does `loupe update` find the latest release? → A: From the latest release's `checksums.txt`, as the script does. This needs no token and meets no rate limit.
- Q: Is there an uninstall? → A: Owner: yes, a `loupe uninstall` command (2026-11-06). It runs the verified script of its own release in an uninstall mode, so the knowledge of what an install adds stays in one file. It needs the network for that reason.
- Q: Which script does `loupe update` run? → A: The one attached to the release it installs, verified against that release's checksums. A copy built into the binary would freeze the update path, so a fix to the script would never reach a person who updates.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One command sets up everything (Priority: P1)

A person runs the curl install. It installs the binary, then installs the loupe plugin into each agent it finds. The output says what it did for each agent.

**Why this priority**: A forgotten plugin is the failure this feature exists to remove.

**Independent Test**: A test runs the script with stand-in agent commands on `PATH` and reads the commands each one received.

**Acceptance Scenarios**:

1. **Given** Claude Code, Codex and Pi on `PATH` and no loupe plugin, **When** the script runs, **Then** each agent has the plugin at the binary's tag, and the output has one line for each agent.
2. **Given** no supported agent on `PATH`, **When** the script runs, **Then** the binary installs and one line points at the plugin docs.
3. **Given** `CI` or `LOUPE_NO_PLUGINS` set, **When** the script runs, **Then** no agent command runs and the output says that the plugins were skipped.

---

### User Story 2 - Updating keeps the binary and the plugins in step (Priority: P1)

A person on an older release runs `loupe update`. The binary moves to the latest release, and each agent's plugin moves to the same tag.

**Why this priority**: Drift between the skill and the binary is the second failure this feature removes.

**Independent Test**: A test serves a fake release and a stand-in script, runs `loupe update` and reads the version and directory that the script received.

**Acceptance Scenarios**:

1. **Given** a release newer than the running binary, **When** `loupe update` runs, **Then** it runs that release's verified script for the directory that holds the running binary, and reports both versions.
2. **Given** the latest release already running, **When** `loupe update` runs, **Then** it still runs the script, so a plugin that is missing or on another tag is corrected.
3. **Given** a plugin from an older tag, **When** the script runs, **Then** it replaces that plugin with the new tag and leaves no second copy.

---

### User Story 3 - The warning names the fix (Priority: P2)

The daily update warning tells a person to run `loupe update`.

**Why this priority**: The warning already reaches the person. It needs to name a command they can run.

**Independent Test**: A test seeds the cache with a newer version and reads the warning text.

**Acceptance Scenarios**:

1. **Given** a cached version newer than the running binary, **When** capture or review shows the warning, **Then** it names both versions and `loupe update`.

---

### User Story 4 - A failure is loud and contained (Priority: P2)

One agent's plugin step fails. The binary stays installed, the other agents still get the plugin, and the output gives the manual commands for the agent that failed.

**Why this priority**: A broken plugin step must not leave a person thinking everything is current.

**Independent Test**: A test makes one stand-in agent fail and checks the exit status, the installed binary and the other agents' commands.

**Acceptance Scenarios**:

1. **Given** one agent whose plugin command fails, **When** the script runs, **Then** the binary is installed, the other agents are handled, the error names the failing agent with its manual commands, and the script exits 1.
2. **Given** a checksum that does not match, **When** `loupe update` runs, **Then** it refuses before it runs anything, and the installed binary is unchanged.

---

### User Story 5 - Removing loupe removes what the install added (Priority: P3)

A person runs `loupe uninstall`. The binary and each agent's loupe plugin go away. The review runs stay, and the output says where they are.

**Why this priority**: The install now changes three agents' plugin setups, so a person cannot easily undo it by hand.

**Independent Test**: A test runs the script's uninstall mode with stand-in agents that hold a plugin, and checks the calls and the removed binary. A second test runs `loupe uninstall` against a fake release.

**Acceptance Scenarios**:

1. **Given** the binary and the plugin in all three agents, **When** `loupe uninstall` runs, **Then** the binary and each plugin are removed, and the output names the data root that still holds the runs.
2. **Given** an agent without the loupe plugin, **When** the uninstall runs, **Then** that agent is left alone and the uninstall succeeds.
3. **Given** a Claude Code or Codex marketplace that points at a local directory, **When** the uninstall runs, **Then** it is left alone and the output says so.

### Edge Cases

- A Claude Code marketplace named `loupe` with a local directory source comes from `mise run link`. The script leaves it alone and says so, and `mise run unlink` removes it before it runs the script.
- A marketplace named `loupe` from any other source, such as `main` or an older tag, is replaced with the tag.
- A pre-release pin, such as `LOUPE_VERSION=0.22.0-rc.1`, pins the plugins to that pre-release tag.
- `loupe update` from a build that is not a release, or from a pre-release, refuses and names the install script. A dev build cannot tell what it would replace, and the latest release never points at a pre-release.
- `loupe update` from a binary that mise installed refuses and names the mise command, because the script would put a second copy beside mise's.
- `loupe update` follows a symlink to the real binary's directory, so it replaces the file that runs.
- A Claude Code or Codex session that is already running keeps the old skill until it restarts. The output says so.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: After the binary installs, the script MUST install or update the loupe plugin for each of Claude Code, Codex and Pi whose command is on `PATH`.
- **FR-002**: The plugin MUST come from the same release tag as the installed binary.
- **FR-003**: When an agent already has the plugin at that tag, the script MUST leave it as is. When the agent has it from another remote source, the script MUST replace it, so that one copy remains.
- **FR-004**: The script MUST NOT change a Claude Code or Codex marketplace named `loupe` whose source is a local directory. It MUST say that it skipped that marketplace.
- **FR-005**: With `CI` or `LOUPE_NO_PLUGINS` set to any non-empty value, the script MUST run no agent command.
- **FR-006**: A failed agent step MUST NOT remove or roll back the installed binary or stop the other agents. The script MUST print `error:` and `fix:` lines that name the agent and its manual commands, and MUST exit 1.
- **FR-007**: The script MUST print one line for each agent that it handled or skipped.
- **FR-008**: Each release MUST attach the install script as an asset, and the release checksums MUST cover it.
- **FR-009**: `loupe update` MUST find the latest release from its published checksums, as the script does, download that release's script, and refuse unless the script's checksum matches.
- **FR-010**: `loupe update` MUST run the verified script with that release's version and the directory of the resolved running binary.
- **FR-011**: `loupe update` MUST refuse, naming the reason and the fix, for a build that is not a release, for a binary under a mise install, for a lookup or download that fails, and for a script that exits non-zero.
- **FR-012**: The script's output MUST go to stderr. With `--json`, stdout MUST carry one result object that names the old and new versions.
- **FR-013**: The update warning MUST name the running version, the latest version and `loupe update`.
- **FR-015**: `loupe uninstall` MUST run the verified script of its own release in uninstall mode, for the directory of the resolved running binary, and MUST refuse a build that is not a release and a binary under a mise install, as `update` does.
- **FR-016**: The uninstall mode MUST remove the binary and the loupe plugin from each of Claude Code, Codex and Pi that holds it, and MUST NOT change a local-directory marketplace.
- **FR-017**: The uninstall MUST NOT delete the data root or any run, and MUST print the data root's path.
- **FR-014**: loupe MUST NOT update itself or a plugin without a person or an agent running `loupe update` or the script.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A person with any of the three agents installed reaches a working setup with one command, and no manual plugin step.
- **SC-002**: After `loupe update` or the script, the plugin tag and the binary version match on every agent found.
- **SC-003**: Running the script twice in a row changes nothing the second time.
- **SC-004**: With no agent on `PATH`, the script's output differs from today's only in its last line.

## Assumptions

- Each agent's plugin command can add a marketplace or package at a Git tag. Removing the Claude Code marketplace also uninstalls the plugin, as `mise.toml`'s `link` task already relies on. The plan records the observed behavior of each command.
- `loupe update` needs no GitHub token. It reads only public release downloads, so a person who reviews only local work can update too.
- The skills MUST NOT tell an agent to run `loupe update`. Updating is a person's choice, and the command does not otherwise check who runs it.
- Linux and macOS only, as for the binary.
