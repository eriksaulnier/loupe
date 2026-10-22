# Feature Specification: Review pane guards

**Feature Branch**: `fix/review-pane-guards`

**Created**: 2026-10-22

**Status**: Draft

**Input**: Owner brief, 2026-10-22. A human ran `loupe handoff` with two other panes already open. The review pane opened about 38 columns wide. `loupe review` then picked plain mode without a word, because the pane was under 60 columns, and plain mode reads whole lines. The human took it for a frozen pane that printed by itself. Separately, an agent can open a second review pane for the same pull request, because the `review-open` guard checks only the run it resolved, and a new round is a new run. This feature makes a narrow pane usable and makes a second pane hard to open by accident.

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/contracts/cli.md` and `specs/001-loupe-v1/research.md`, and the human-review skill.

- **`specs/001-loupe-v1` research, "Plain mode"**: plain mode is no longer chosen for a terminal under 60 columns by 12 rows. `--plain`, `TERM=dumb` and a failed raw-mode probe still choose it. `specs/037-remove-plain-mode` removes those later, and nothing here builds toward or against it.
- **`specs/002-review-ux`**: the 60 by 12 minimum stays. Below it the full-screen interface now shows a "too small" screen in place of the view (FR-002).
- **`specs/019-pane-hosts` FR-005**: when no source gives the agent pane's width, `handoff` now chooses `down`, not `right` (FR-005 here). A known width keeps its rule.
- **`specs/017-live-review` FR-012**: `review-open` now covers every round of the pull request, not only the resolved run, and its refusal gains `details.run` (FR-006, FR-007). The code, its exit and the "opens nothing, writes no run state" rule are unchanged.
- **`contracts/cli.md`, `loupe wait`**: the result gains `reviewUrl` when `reason` is `published` (FR-012). The change only adds a key.
- **`contracts/cli.md`, `loupe review`**: no change to `--json`. The URL a session published goes to the interface's output (FR-011).
- `docs/comment-format.md` and the published review do not change.

## Clarifications

### Settled by the owner's brief, 2026-10-22

- Q: Which way should a pane split when the agent pane's width is unknown? → A: Down. Review needs 60 columns but only 12 rows, so width is the scarce resource. Orca reports no pane size, and an agent's shell usually cannot read its terminal, so on Orca this is every hand-off.
- Q: How wide is the per-PR guard? → A: Every round of the same `owner/repo#N`. A different pull request MUST still pass, because a batch review hands off several pull requests at once.
- Q: What follows a publish in the full-screen review? → A: A screen that says "Published" and shows the review's link. `o` opens the link and quits. Any other key quits.

### Decided by the implementing agent

- Q: What do `q` and `ctrl+c` do while the window is too small? → A: What `ctrl+c` does on the screen underneath. In the list or a finding it quits. In the publish confirmation it declines, so nothing is sent. While a confirmed review is sending it does nothing, as today. Any other key would act on a screen the human cannot see.
- Q: Which run does `details.run` name when more than one round has a review open? → A: The resolved run when it is open, else the newest open round. The skill only needs to know whether it is its own.
- Q: Does `loupe review --json` gain the URL? → A: No. Its payload is not in this brief's list of contract changes. Under `--json` the line goes to stderr, where the interface draws, and `loupe wait` carries the URL as data.
- Q: Is the link drawn under `NO_COLOR`? → A: The URL is drawn as text in every mode. The terminal hyperlink escape is added only where `internal/style` emits escapes, so `NO_COLOR` output holds no escape sequence, as it does everywhere else.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A narrow pane says it is too small (Priority: P1)

A hand-off opens review in a pane 38 columns wide. The pane shows one centered message: its size, the size review needs, and that `q` quits. The human widens the pane, and the finding list appears with nothing lost.

**Why this priority**: It is the failure that started this feature. A silent mode switch read as a frozen pane.

**Independent Test**: Drive the full-screen model at 40 by 10, then send a resize to 100 by 40.

**Acceptance Scenarios**:

1. **Given** a terminal 40 columns by 10 rows, **When** review starts, **Then** it runs full screen, not plain, and shows the "too small" screen with `40` and `10` and the needed `60` and `12`.
2. **Given** the "too small" screen, **When** the human presses `a`, `x`, `enter` or `p`, **Then** nothing is recorded and the screen does not change.
3. **Given** the "too small" screen, **When** the window grows to 100 by 40, **Then** the list shows again with its cursor where it was.
4. **Given** the list at 100 by 40, **When** the window shrinks to 50 by 30, **Then** the "too small" screen shows, and growing it again restores the view the human was on.
5. **Given** the publish confirmation with a message typed, **When** the window shrinks below 60 by 12 and grows again, **Then** the message and the confirmation are as they were.
6. **Given** the "too small" screen over the publish confirmation, **When** the human presses `q`, **Then** the confirmation declines and nothing is sent.

---

### User Story 2 - A second pane for the same pull request is refused (Priority: P1)

The human has round 1 of `acme/widgets#42` open in review. The agent captures round 2 and runs `loupe handoff`. Handoff refuses `review-open` and names round 1 in `details.run`, so the agent tells the human which round is open and asks them to finish or quit it.

**Why this priority**: Two panes on one pull request split the human's decisions across rounds without telling them.

**Independent Test**: Hold a review session on round 1, resolve round 2, run handoff.

**Acceptance Scenarios**:

1. **Given** a session holds round 1 of `acme/widgets#42`, **When** `loupe handoff --run 'acme/widgets#42@2' --json` runs, **Then** it refuses `review-open` with `details.run` `acme/widgets#42@1`, opens no pane and writes no run state.
2. **Given** a session holds round 1 of `acme/widgets#42`, **When** handoff runs for `acme/widgets#43`, **Then** the guard passes and handoff goes on to detect a host.
3. **Given** a session holds the resolved run, **When** handoff runs, **Then** it refuses `review-open` with `details.run` equal to the resolved run, as before.
4. **Given** the agent's skill gets `review-open`, **When** `details.run` equals its own ref, **Then** it goes on to wait. **When** it differs, **Then** it tells the human which round is open and asks them to finish or quit it, then hands off again.

---

### User Story 3 - A narrow agent pane on Orca splits down (Priority: P2)

On Orca no width is known, so the hand-off splits below the agent's pane, which keeps the pane's full width.

**Independent Test**: Call the direction rule with no host width and no terminal width.

**Acceptance Scenarios**:

1. **Given** no host width and an unreadable terminal, **When** the direction is chosen, **Then** it is `down`.
2. **Given** a known width of 120 or more, **When** the direction is chosen, **Then** it is `right`. A known width under 120 gives `down`.

---

### User Story 4 - Publishing ends on the review's link (Priority: P2)

The human confirms the review. The screen changes to "Published" with the review's link. They press `o`, the browser opens the review, and the pane closes. The link is also printed once the full-screen view has closed, so it stays in the pane's scrollback. The agent's `loupe wait` returns `published` with the same link, and the agent gives it to the human.

**Independent Test**: Publish through the full-screen model against the fake GitHub with an injected opener.

**Acceptance Scenarios**:

1. **Given** a confirmed publish that succeeds, **When** the outcome arrives, **Then** the screen says "Published" and shows the review's URL.
2. **Given** the published screen, **When** the human presses `o`, **Then** the injected opener receives the URL and the program quits.
3. **Given** the published screen, **When** the human presses any other key, **Then** the program quits and the opener is not called.
4. **Given** review exits after a publish in that session, **When** the full-screen view has closed, **Then** the URL is printed on the interface's output.
5. **Given** a run with a receipt, **When** `loupe wait --json` returns `published`, **Then** its result carries `reviewUrl` from the receipt.

### Edge Cases

- A window that shrinks after launch shows the "too small" screen at once, from any view, including the help overlay and a note being written.
- A window exactly 60 by 12 is not too small.
- Under `--json`, `review` and `publish` read the size from stdout, which is then a pipe, so they read no size and always chose plain mode. With FR-001 they draw full screen on stderr, as `contracts/cli.md` already says the interface does under `--json`.
- A window too small to hold the message wraps it. One column is still drawn without a panic.
- A publish that is canceled or refused returns to the list with its notice, as today. Only a sent review reaches the published screen.
- The opener fails, for example when no browser can be found. The published screen stays, with the error as a warning, and the next key quits. The URL is still printed when review exits.
- A URL that is not `https://` or holds a control character is drawn as escaped text with no hyperlink.
- Out of scope: two hand-offs for one pull request at the same moment. Each probes before the other's review has started, so both may open a pane. Closing that race needs a lock held from the probe to the start of review, which crosses two processes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Review MUST NOT choose plain mode because of the terminal's size. `--plain`, `TERM=dumb` and a failed raw-mode probe MUST still choose it.
- **FR-002**: The full-screen review, and the publish confirmation that `loupe publish` runs on its own, MUST draw one centered message in place of the view while the window is under 60 columns or under 12 rows. The message MUST give the current size and the size needed, and MUST say that `q` quits.
- **FR-003**: While the message shows, `q` and `ctrl+c` MUST act as `ctrl+c` does on the screen underneath, and every other key MUST do nothing.
- **FR-004**: The message MUST keep all state. The next resize to 60 by 12 or more MUST show the screen underneath as it was.
- **FR-005**: `handoff` MUST choose `down` when neither the host nor the agent's terminal gives a width. A known width MUST keep the rule of `specs/019-pane-hosts` FR-005.
- **FR-006**: `handoff` MUST refuse `review-open` when a review session is open on any round of the resolved run's pull request, before it detects a host. It MUST open no pane and write no run state. A session on a different pull request MUST NOT refuse it.
- **FR-007**: The `review-open` refusal MUST carry `details.run`, the ref of the open run with its round. When the resolved run is open it MUST be that run, else the newest open round. When the open run is not the resolved run, the fix MUST tell the agent to ask the human to finish or quit that review and then hand off again.
- **FR-008**: After a review is sent from the full-screen review, the program MUST show a published screen that says "Published" and shows the review's URL. The URL MUST be a terminal hyperlink where the style emits escape sequences, and plain text otherwise.
- **FR-009**: On the published screen, `o` MUST open the URL in the system browser and then quit. Any other key MUST quit without opening it. A failure to open MUST be shown as a warning on that screen, and the next key MUST quit.
- **FR-010**: The browser MUST be opened through an injected function, so tests never start a browser.
- **FR-011**: When review exits after a review was sent in that session, it MUST print the URL after the full-screen view has closed, on stdout, or on stderr under `--json`. The `--json` result MUST NOT change.
- **FR-012**: `loupe wait`'s result MUST carry `reviewUrl`, read from the run's receipt, when `reason` is `published`. It MUST make no network call for it. The human output MUST show the URL.
- **FR-013**: The human-review skill MUST compare `details.run` with its own ref on `review-open`. When they are the same it MUST go to waiting. When they differ it MUST tell the human which round is open and ask them to finish or quit it, then hand off again. On `published` it MUST give the human `reviewUrl`, then stop.
- **FR-014**: `contracts/cli.md` and `research.md` MUST record FR-001, FR-005, FR-006, FR-007 and FR-012.

### Key Entities

- **Review session**: a running `loupe review`'s shared lock on its run. A pull request has a session open when any of its rounds has one.
- **Receipt**: the run's record of a sent review. It holds the review's URL as GitHub returned it.

## Success Criteria *(mandatory)*

- **SC-001**: A review started at 40 by 10 shows the "too small" screen, and after a resize to 100 by 40 shows the list. No key pressed between changes the draft.
- **SC-002**: With round 1 of a pull request open, a hand-off of round 2 refuses `review-open` naming round 1, and a hand-off of another pull request does not refuse.
- **SC-003**: With no known width, the hand-off direction is `down`.
- **SC-004**: A publish from the full-screen review reaches the published screen. `o` calls the opener with the review URL once, and any other key quits without calling it.
- **SC-005**: `loupe wait --json` on a published run returns `reviewUrl` equal to the receipt's.

## Assumptions

- Out of scope: removing plain mode, `--plain`, or the `TERM=dumb` and raw-probe fallbacks (`specs/037-remove-plain-mode`). A race between two hand-offs at the same moment. Any change to the published review format.
- Plain-mode review has no publish step, and `loupe publish` already prints the URL, so neither changes.
- `open` on macOS and `xdg-open` elsewhere open a URL in the default browser. No other platform is supported for `o`. Elsewhere `o` shows a warning.
