# Feature Specification: Remove plain mode

**Feature Branch**: `feat/remove-plain-mode`

**Created**: 2026-10-22

**Status**: Draft

**Input**: Owner request, 2026-10-22. The human review interface has two front ends: the full-screen program and a line-by-line plain mode. `--plain` picks plain mode on `loupe review` and `loupe publish`, and loupe also picks it under `TERM=dumb`, when stdin cannot enter raw mode, and in a window under 60x12. Every feature had to be built twice: specs 005, 011, 013 and 017 each added plain parity. Plain mode also confused a human when loupe picked it silently in a narrow pane. Nobody is known to rely on it. The owner has decided to delete it.

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/contracts/cli.md`, which is a contract (CONTRIBUTING.md, Pull requests). It removes `--plain` from `loupe review` and `loupe publish`, and it replaces the plain-mode fallback with the existing `tty` refusal.

- It replaces US2 acceptance scenario 8 and the small-terminal edge case of `specs/001-loupe-v1/spec.md`, and drops the plain-mode clause of its FR-039.
- It replaces the plain-mode paragraph of `specs/001-loupe-v1/research.md`.
- The plain-mode parity that specs 005, 011, 013 and 017 added goes with it. Those specs stay as they were written.
- A window under 60x12 is `specs/036-*`'s, which adds a screen that says the window is too small. This specification does not handle it.
- `docs/comment-format.md` and the published review do not change.

## Clarifications

### Settled by the owner, 2026-10-22

- Q: A new refusal code for a terminal that cannot draw full screen? → A: No. It is the existing `tty` refusal, with a fix that asks for a terminal that can draw full screen.
- Q: Keep `--plain` as an accepted no-op? → A: No. It is removed, so it is a usage error like any unknown flag. This is a breaking change to the command line.
- Q: What happens to the tests that drove plain mode? → A: They drive the full-screen program instead, with equal coverage. A test MUST NOT get weaker in the rewrite.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One review interface (Priority: P1)

A human runs `loupe review` or `loupe publish` in any terminal that can draw full screen. They always get the full-screen program. Nothing switches them into a different interface with different keys.

**Independent Test**: Run review and publish against a fake GitHub and drive them by keys through the full-screen program.

**Acceptance Scenarios**:

1. **Given** a terminal that can draw full screen, **When** the human runs `loupe review`, **Then** the full-screen program opens, and every decision it records is the one plain mode recorded for the same answer.
2. **Given** a human passes `--plain` to `loupe review` or `loupe publish`, **When** the command parses its flags, **Then** it refuses with `usage` and exit code 2 before it reads run state.

---

### User Story 2 - A terminal that cannot draw full screen is told so (Priority: P1)

A human runs review or publish under `TERM=dumb`, or on a terminal whose stdin cannot enter raw mode. loupe refuses with `tty` and says why. It does not fall back to another interface.

**Independent Test**: Run each command with `TERM=dumb`, and with a stdin that cannot enter raw mode, and read the refusal.

**Acceptance Scenarios**:

1. **Given** `TERM=dumb` and an interactive terminal, **When** the human runs `loupe review`, **Then** it refuses with `tty` before it resolves the run, the message names `TERM=dumb`, and the fix is `run loupe review in a terminal that can draw full screen`.
2. **Given** a stdin that cannot enter raw mode, **When** the human runs `loupe review`, **Then** it refuses with `tty`, and the message says the terminal cannot enter raw mode.
3. **Given** either condition, **When** the human runs `loupe publish` and every gate passes, **Then** it refuses with `tty` before the confirmation, with the fix `run loupe publish in a terminal that can draw full screen`. Nothing is sent and no attempt is recorded.

### Edge Cases

- A publish that a gate refuses still refuses with that gate's code. The terminal is probed only once the gates have passed, as before.
- `--unattended` shows no confirmation, so the terminal checks do not apply to it. Its refusal of `--unattended --plain` goes away with the flag.
- A session that ends by Ctrl-C records its hand-back exactly as `q` does. End of input was the third clean exit only in plain mode.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe review` and `loupe publish` MUST NOT accept `--plain`. Passing it MUST refuse with `usage`.
- **FR-002**: loupe MUST NOT offer a line-by-line review or confirmation.
- **FR-003**: `loupe review` MUST refuse with `tty` under `TERM=dumb` or when stdin cannot enter raw mode, after the existing terminal check and before it resolves the run.
- **FR-004**: `loupe publish` MUST refuse with `tty` under the same conditions, once every gate has passed and before the confirmation shows.
- **FR-005**: The refusals of FR-003 and FR-004 MUST name their condition and MUST carry the fix `run loupe <command> in a terminal that can draw full screen`. No new refusal code MAY be added.
- **FR-006**: Every test that drove plain mode MUST drive the full-screen program instead and MUST assert at least what it asserted before. A behavior that exists only because of plain mode, such as reading answers as lines, MAY lose its test, and the plan MUST name each one.
- **FR-007**: Tests MUST drive the full-screen program with injected input and a fake terminal size, and MUST wait for what the screen shows, never for time to pass (Principle VII).
- **FR-008**: The contract, the help text and the documents that describe plain mode MUST say instead that review and publish draw full screen and refuse a terminal that cannot.

## Success Criteria *(mandatory)*

- **SC-001**: No command, help text or current document offers plain mode or `--plain`.
- **SC-002**: Each test that drove plain mode has a full-screen counterpart that fails when the behavior it pins breaks.
- **SC-003**: `mise run check` passes, and no test in the repository sleeps to wait for the review program.

## Assumptions

- Out of scope: the too-small screen, the split direction of the hand-off, the per-pull-request hand-off guard and the screen after publish. Those are spec 036's.
- The history in specs 002 to 035 stays as written. Their plain-mode text describes what they built at the time.
- A human who relied on plain mode moves to a terminal that can draw full screen. loupe offers no replacement.
