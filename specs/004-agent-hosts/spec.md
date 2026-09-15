# Feature Specification: loupe skill for Codex and Pi

**Feature Branch**: `004-agent-hosts`

**Created**: 2026-09-15

**Status**: Draft

**Input**: Owner request: "Run loupe reviews from Codex and Pi as well as Claude Code." MCP is out of scope.

## Relationship to earlier specifications

loupe's binary already serves any agent with a shell (constitution I). The `loupe` skill in `plugin/skills/loupe/SKILL.md` is already host-neutral apart from the Monitor line, which has a fallback. The gap is distribution: only Claude Code can install the skill, as a plugin. This specification adds an install route for other hosts and one rule for sandboxed shells. It amends the skill's Rules section from `specs/003-herdr-handoff` and changes nothing else.

The constitution, the CLI contract, the run layout and the published comment format remain authoritative and unchanged.

### Why not MCP

The constitution defers MCP. `loupe wait` blocks for minutes, which fights MCP call timeouts, and a shell already reaches every command.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Install the skill in Codex or Pi (Priority: P1)

The owner runs one install command. Codex and Pi then list the `loupe` skill, and a review request in either host loads it.

**Independent Test**: With a scratch `HOME`, run the documented install command and confirm each host lists the skill.

**Acceptance Scenarios**:

1. **Given** Codex 0.154 or later and the skill installed with the documented command, **When** Codex starts, **Then** its skill list contains `loupe`.
2. **Given** Pi and the same install, **When** Pi starts, **Then** its commands contain `skill:loupe`.
3. **Given** the documented command, **When** it runs, **Then** it installs only `loupe`, not the repository's development skills.

### User Story 2 - Review from a sandboxed host (Priority: P2)

In Codex's default sandbox, the agent follows the skill. When a `loupe` or `herdr` command fails because of the sandbox, the agent asks to run it outside the sandbox, and the owner approves.

**Independent Test**: Owner-only; see FR-008.

### Edge Cases

- The skill is installed from `main` while the binary is a release. A skill line can name a flag the installed binary lacks. The README tells the owner to update both.
- An agent that cannot write loupe's data directory could point `LOUPE_HOME` into the workspace. Its runs would then be invisible to the human's `loupe review`. The skill forbids this.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `plugin/skills/loupe` MUST remain the single source of the skill for every host. The directory is `plugin/skills/human-review` since `specs/006-agent-plugins`. No copy of `SKILL.md` MAY be added.
- **FR-002**: The skill MUST meet the Agent Skills limits: `name` equal to its directory, at most 64 characters of `a-z` and `0-9` joined by single hyphens; `description` from 1 to 1024 characters. A test MUST pin these limits.
- **FR-003**: The skill MUST tell an agent in a sandboxed shell to rerun a `loupe` or `herdr` command that the sandbox blocked with the host's approval, and MUST forbid working around the sandbox by setting `LOUPE_HOME` or `XDG_DATA_HOME`. A test MUST pin the line. Amended by `specs/006-agent-plugins` FR-017: the rule names `loupe handoff` instead of `herdr`.
- **FR-004**: Every other skill line MUST stay host-neutral.
- **FR-005**: The README MUST document installing the skill for Codex and Pi with `npx skills add eriksaulnier/loupe --skill loupe -g -a codex -a pi`, note that the skill and binary can drift, and say what Codex's sandbox needs. Superseded on 2026-09-15 by `specs/006-agent-plugins` FR-019 to FR-022: every harness installs the plugin, and the README has no `npx skills` route.
- **FR-006**: The loupe binary MUST NOT change. No `loupe skill install` subcommand MAY be added.
- **FR-007**: A Codex plugin manifest MUST NOT be added while Codex discovers the skill from `~/.agents/skills`. Superseded on 2026-09-15 by `specs/006-agent-plugins` FR-019.
- **FR-008**: A live review round in Codex and in Pi MUST be added to the Unverified list in `specs/001-loupe-v1/validation.md`.

## Success Criteria *(mandatory)*

- **SC-001**: `mise run check` passes with the tightened plugin test.
- **SC-002**: A scratch-`HOME` install with the documented command shows `loupe` in Codex's and Pi's skill lists.

## Assumptions

- While the repository is private, the owner is logged in to `gh` with access to it; the installer clones through `gh`. Making the repository public removes this need and turns on the installer's telemetry.
- Codex's default approval policy lets the model request escalation. Hosts that forbid escalation cannot run loupe inside their sandbox.

## Deferred

- A native Codex plugin (`.codex-plugin/plugin.json`) and a Pi package (`pi install git:...`). Neither is needed while both hosts read `~/.agents/skills`. Delivered by `specs/006-agent-plugins`.
- A Codex or Pi equivalent of the `/loupe` command.
