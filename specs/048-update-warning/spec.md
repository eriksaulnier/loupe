# Feature Specification: A warning when a newer loupe release exists

**Feature Branch**: `048-update-warning`

**Created**: 2026-11-02

**Status**: Draft

**Input**: Owner decision, 2026-11-02. `specs/047-install-script` added a curl install, which has no update path of its own, so an installed binary starts to go stale at the next release. The plugin installs from `main`, so a stale binary can also lack a flag that the skill names. Owner: warn when an update is available. A `loupe update` command is out of scope.

## Relationship to earlier specifications

- `specs/001-loupe-v1/contracts/cli.md` gains one warning code, one environment variable and an optional `warnings` array in the `capture` result. Each is additive. No existing field, refusal or exit code changes.
- `docs/comment-format.md` does not change.
- Constitution 3.1.0 is unchanged. Principle III holds because the check is a GitHub API call and the cache is a plain file. Principle V holds because the warning goes to stderr and follows the existing warning contract.

## Clarifications

### Session 2026-11-02

- Q: Where does loupe learn the latest version? → A: Owner: from the GitHub API, not from the Pages site. A file on Pages would be network access beyond Git and the GitHub API, which Principle III forbids.
- Q: Which command checks? → A: Owner: `loupe capture`, which already calls GitHub with the user's token. No other command gains a network call.
- Q: Who sees the warning? → A: Owner: `capture` output reaches the agent, not the human, so `loupe review` also shows the result. It reads the cached version and makes no network call.
- Q: Does the warning name the update command? → A: No. loupe cannot know how it was installed, so the warning links to `docs/install.md`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human learns that an update exists (Priority: P1)

A reviewer runs an old loupe. When review opens, one line says that a newer release exists and where to read how to update.

**Why this priority**: The human is the one who can update, and the review interface is what they look at.

**Independent Test**: A test seeds the cache with a newer version, opens review through the injected terminal and reads the notice.

**Acceptance Scenarios**:

1. **Given** a cached version newer than the running binary, **When** review opens, **Then** a notice names both versions and `docs/install.md`.
2. **Given** a cached version equal to or older than the running binary, **When** review opens, **Then** no notice appears.
3. **Given** no cache, **When** review opens, **Then** no notice appears and no network call runs.

---

### User Story 2 - The agent learns that an update exists (Priority: P2)

An agent runs `loupe capture`. Stderr carries a warning, and under `--json` the result lists its code.

**Why this priority**: An agent can pass it on, and a stale binary is a likely cause when a skill names a flag that fails.

**Independent Test**: A test runs capture against the fake GitHub with a newer release and reads stderr and the result.

**Acceptance Scenarios**:

1. **Given** a newer release, **When** capture succeeds, **Then** stderr carries `warning: <message>` and a `--json` result lists `update-available` in `warnings`.
2. **Given** no newer release, **When** capture succeeds, **Then** the output is byte-identical to today's.

---

### User Story 3 - The check never gets in the way (Priority: P1)

The network is down, GitHub returns an error, or the check is slow. Capture behaves as it does today.

**Why this priority**: A convenience MUST NOT break the command that starts every review.

**Independent Test**: Tests make the fake GitHub fail and stall, and check that capture succeeds with unchanged output.

**Acceptance Scenarios**:

1. **Given** a failing or stalled release lookup, **When** capture runs, **Then** it succeeds within the time limit and prints no warning.
2. **Given** a check within the last 24 hours, **When** capture runs, **Then** it makes no release lookup.
3. **Given** a dev build, a CI environment, an installation token or `LOUPE_NO_UPDATE_CHECK`, **When** capture or review runs, **Then** no lookup, warning or notice occurs.

### Edge Cases

- An upgrade clears the warning at once, because the versions are compared when read, not when cached.
- A corrupt or unreadable cache counts as absent, and the next check replaces it.
- A failed lookup still records the time of the check, so an offline machine pays the time limit at most once a day.
- `/releases/latest` returns neither drafts nor prereleases, so a prerelease never triggers the warning.
- A failed capture prints no warning, because its refusal is the only thing that matters.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe capture` MUST look up the latest release at most once per 24 hours, after the run is created.
- **FR-002**: The lookup MUST use the GitHub API with the token capture already holds, and a time limit of 2 seconds.
- **FR-003**: A failed lookup MUST NOT change capture's result, output or exit code.
- **FR-004**: loupe MUST cache the latest version and the time of the check in one plain file under the data root.
- **FR-005**: When the cached version is newer, a successful capture MUST warn under the code `update-available`, on each capture, and not only on the day of the lookup.
- **FR-006**: The message MUST name the running version, the latest version and `docs/install.md`.
- **FR-007**: `loupe review` MUST show the same fact as a notice when it opens, from the cache alone.
- **FR-008**: The check, warning and notice MUST NOT run for a version that is not `X.Y.Z`, under `CI`, with an installation token (for the lookup), or with `LOUPE_NO_UPDATE_CHECK` set to any non-empty value.
- **FR-009**: The cache file MUST NOT disturb `loupe list`, `loupe stats` or any other reader of the data root.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A person on an old release sees the notice at the next review after a capture has looked up the new release.
- **SC-002**: A failing or stalled lookup adds at most 2 seconds to a capture, at most once a day.
- **SC-003**: With no newer release, capture's output is byte-identical to today's. The only golden that changes is the environment list in `--help`.

## Assumptions

- `GET /repos/eriksaulnier/loupe/releases/latest` works with any token loupe accepts, since the repository is public. A token that fails counts as a failed lookup.
- The notice appears once per review session, and it clears on the next action.
- A fork that builds its own binary sees the upstream version. That is acceptable, because a fork's build usually reports a non-`X.Y.Z` version.
