# Implementation Plan: A send-back reaches the agent at once, and the open review shows the answer

**Branch**: `017-live-review` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/017-live-review/spec.md`

## Summary

`draft.Mutate` hands back every note a mutation adds, under the run lock and before the draft is written, so a send-back from either review mode reaches a blocked `loupe wait` within its one-second poll. `loupe review` holds a shared flock on a run-local session file for its lifetime, and `loupe handoff` refuses with a new `review-open` code while any holder is alive. The review window re-reads the draft every 2 seconds in the list and detail views while a handed-back note awaits a reply, applies what changed in place with a notice, and offers `ctrl+r` to reload on demand. The skill waits again on `review-open` instead of stacking a pane. Help, README, spec 001's contract passages, and the skill are rewritten to match.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. The timer is `tea.Tick`, as the settle timers already are; the session marker is `syscall.Flock`, as the run lock already is.

**Storage**: One new run-local file, `.review`, empty, held under a shared flock by each running review. `handback.json` keeps schema 1 and gains writes at send-back. `draft.json` is unchanged.

**Testing**: Failing test first for each behavior: `draft` (a mutation that adds a note hands it back; one that adds none leaves `handback.json` untouched; the draft version moves once), `run` (the session marker is seen while held, released on close, and released when a holder process is killed), `cli` (handoff refuses `review-open` while a session holds the marker, before `no-pane-host`), `integration` (wait returns `notes` while a plain-mode review is still reading stdin; handoff refuses then, and not after the review exits), `tui` (a delivered poll message redraws with a notice; editors and publish views hold it; the timer only runs while something awaits; `ctrl+r`; a stale decision is still refused). Goldens: `go test ./internal/cli/ -update`, then read the diff; only `help.*` and `handoff-help.*` may move. `mise run check` closes every commit.

**Target Platform**: Unchanged. Linux and macOS, where `flock` exists; the run lock already depends on it.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.2. Refusal codes are a pinned contract; this specification is what adds `review-open`. Spec 001's `wait` contract and FR-039/FR-040 are amended here with dated pointers. `docs/comment-format.md` is untouched.

**Scale/Scope**: One branch in `Mutate`, one new file in `internal/run`, one refusal code, one check in `handoff`, a hold around both review modes, a timer and a reload key in `internal/tui`, and prose in the skill, three help texts, the README and three spec 001 documents.

## Research

- **Hand back inside `Mutate`, not in the send-back call sites.** Decision: `Mutate` counts notes before `fn`; when `fn` added any that are open and unanswered, it adds their ids to `handback.json` under the same lock, then writes the draft. Rationale: FR-001. The full-screen and plain modes each call `draft.SendBack` through their own decide paths, and both go through `Mutate`; putting the step there covers both and any future caller, and keeps "the only way a draft changes" the only place hand-back at write time lives. Alternative rejected: a `SendBackHandingBack` store function, which would need both modes' generic decide paths to grow a special case.
- **Hand-back before the draft write.** Decision: write `handback.json` first. Rationale: the edge case in the spec. A reader that sees the note must see it handed back; the reverse order leaves a window where `wait` reads the note as not handed back. If the draft write then fails, the set holds an id no note has; `Awaiting` already filters ids by existing open notes, and the next note to take that id would have been handed back at its own send-back anyway, so the dangling id changes nothing.
- **Clean-exit recording stays.** Decision: `RecordHandBack` is unchanged and still runs in `reviewDone`. Rationale: FR-002. It is now a backstop for a note an older binary wrote, and costs nothing when it finds nothing.
- **A shared flock for the session, an exclusive probe for handoff.** Decision: `run.HoldSession(dir)` opens `<run>/.review` and takes `LOCK_SH`; `run.SessionOpen(dir)` opens it separately and tries `LOCK_EX|LOCK_NB`, releasing at once, and reports open on `EWOULDBLOCK`. Rationale: FR-011. The kernel drops a flock when its holder dies, which is the property the run lock already relies on, so a crashed or killed review never leaves a marker that lies. Shared mode lets two review sessions on one run coexist, which spec 003 allows. The file is separate from `.lock`, because the run lock is exclusive and held only for a mutation. Alternative rejected: a pid file, which needs liveness checks and cleanup and can lie after a crash.
- **The probe is in handoff only.** Decision: nothing but `loupe handoff` reads the marker, and `wait`'s payload does not gain a field. Rationale: FR-012 and FR-014. The skill needs one decision at one moment — after answering, open a pane or not — and handoff is the command that makes it. A field in `wait` would be a second way to learn the same thing, stale by the time the agent acts.
- **`review-open` before `no-pane-host`.** Decision: handoff resolves the run, probes the marker, and only then detects Herdr. Rationale: FR-012. Outside Herdr the skill tells the human to run `loupe review`; if review is already open that advice is wrong, so the open session outranks the missing pane host. The fix names `loupe wait --run <ref> --json`.
- **The timer is a message the model schedules, not a goroutine.** Decision: a `pollMsg` scheduled with `tea.Tick(pollInterval, …)`, `pollInterval` a package variable of 2 seconds like `settleOnOpen`. At most one tick is in flight; the model tracks it with a flag. Tests deliver `pollMsg` directly with `teatest`'s `Send`, so no test sleeps on the interval. Rationale: FR-005 and the spec's controlled-clock criteria.
- **When the timer runs.** Decision: each tick loads the draft and `handback.json`; if the version moved it applies the change; it schedules the next tick while `draft.Awaiting` is non-empty, and once more after a tick that applied a change. A send-back from this window, the refresh key, and any reload that finds something awaiting start it. Rationale: FR-005 as clarified. The chain ends on the first quiet tick after the last reply, so an edit the agent writes just after replying still lands. The review round found the earlier rule, which stopped on the reply's own tick, dropping that edit.
- **Held means skipped, then checked on return.** Decision: a tick that arrives while an editor, a file diff, the publish steps, the confirmation or a publish in progress is on screen does nothing but reschedule. Closing an editor, leaving a file diff, and returning to the list from the publish flow run the same check at once. Rationale: FR-007. The confirmation already reloads after a publish finishes; the new check covers the cancel and back paths.
- **What the notice says.** Decision: compare the displayed draft with the fresh one. New agent replies name the note and its finding (`n-004 answered on f-003`); otherwise changed findings by id (`f-003 changed`); otherwise `draft changed`. Several changes in one tick make one notice with a count. Rationale: FR-006. The comparison runs only when the version moved, and the human's own writes already advance the displayed version, so they never reach it.
- **Scroll kept, cursor by id.** Decision: the detail view keeps its viewport offset across the redraw, clamped to the new content; the list cursor is found by id as `esc` from the detail view already does. Rationale: FR-006.
- **`ctrl+r` reloads.** Decision: `ctrl+r` in the list and the detail view, listed in both help overlays. Rationale: FR-008. `r` is resolve in the detail view, so the obvious letter is taken, and a capital `R` one shift away from resolving a note is a hazard on a surface where every letter decides something. `ctrl+r` is the conventional reload chord and is unbound in both views. On an unchanged draft the notice says it is current.
- **The send-back notice says where the note went.** Decision: `f-003 sent back as n-004` gains `; the agent has it` in both modes. Rationale: the protocol change is invisible otherwise, and a human who learned "quit to hand back" would quit. It is a notice string, pinned only by tests that assert it.
- **The skill's loop.** Decision: section 6 describes `wait` as returning on a send-back; section 7's closing paragraph runs `loupe handoff` after the last reply and, on `review-open`, tells the user the answers are in their open review and waits again. The sentence about notes not handed back is removed. Section 5's "each hand-off opens a new pane" stays true and stays. Rationale: FR-010, FR-013, FR-014.
- **A decision is checked against its finding, not the draft.** Decision: `draft.FindingState(d, id)` returns the JSON encoding of the finding, its decision, its notes and their replies; the review window's decide path runs `Mutate` without an expected version and, inside the lock, refuses with the existing `version` code when the stored state differs from the state computed from the displayed draft. Rationale: FR-009, the owner's decision after the review round. `Finding.Rev` alone was the obvious key and is part of the state, but it moves only on a content edit: a withdraw or restore, a new note or reply, and another session's decision all leave it where it was, and each of those changes what the human is deciding. `Decision.FindingRev` is part of the state too, which is where another session's decision shows. JSON rather than a struct comparison, because a draft that came back from `Mutate` still carries monotonic clock readings a loaded one does not. The whole-draft check stays in `Mutate` for `--expect-version` and is simply not asked for here.
- **Changes elsewhere are named with the decision.** Decision: when a decision records, the window diffs the draft it displayed against the one `Mutate` returned, leaving out the decided finding, and adds what it finds to the decision's notice. Rationale: FR-015. Before this change a stale refusal was the only way a decision could meet an outside write; now it records past them, and without the notice the other changes would appear unannounced.
- **What this change cannot check.** Decision: whether an agent following the amended skill opens exactly one pane across two send-backs is read, not run, and goes on `specs/001-loupe-v1/validation.md`'s Unverified list beside the other agent-behavior rows. Rationale: SC-005 and Principle VII.

## Constitution Check

- **I. A tool for agents.** PASS. The new refusal names its fix; `loupe wait --help` and `loupe handoff --help` describe the new behavior, so the loop is completable from help alone. The skill adds no code path only one host can reach; Claude Code, Codex and Pi load the same file.
- **II. Nothing posts unread under a human's name.** PASS. A send-back decides nothing and publishes nothing. The window never records a decision against content it has not shown (FR-009), and the confirmation is held.
- **III. Local files, no service.** PASS. One empty run-local file and a flock; no daemon, no watcher, no network. loupe never deletes the file.
- **IV. Never touch the user's checkout.** PASS. Nothing here touches git.
- **V. Machine contract first.** PASS. `review-open` is a new code with a message and a fix, added to `contracts/cli.md`. No existing envelope field or code changes.
- **VI. Simplicity over ceremony.** PASS. No dependency. A version comparison instead of a file watcher, a flock instead of a pid file, and no new abstraction: `HoldSession` and `SessionOpen` sit beside `Lock` in `internal/run`.
- **VII. Verified means ran.** PASS. Every behavior opens with a failing test. The one agent-behavior claim is put on the Unverified list rather than made.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/017-live-review/
├── spec.md
├── plan.md
└── tasks.md

internal/draft/store.go           # Mutate hands back notes the mutation added
internal/draft/handback.go        # shared append used by Mutate and RecordHandBack
internal/run/session.go           # new: HoldSession, SessionOpen
internal/refusal/refusal.go       # ReviewOpen
internal/cli/review.go            # hold the session around both modes
internal/cli/handoff.go           # review-open before no-pane-host; help text
internal/cli/wait.go              # help text
internal/cli/help.go              # root help's wait and send-back loop lines
internal/tui/app.go               # pollMsg, pollInterval, apply-with-notice, hold, ctrl+r, help overlay
internal/tui/list.go              # ctrl+r; check on return from the publish flow
internal/tui/detail.go            # ctrl+r; scroll kept; check on closing an editor; send-back notice
internal/tui/plain.go             # send-back notice; per-finding staleness
internal/draft/derive.go          # FindingState
plugin/skills/human-review/SKILL.md
README.md                         # wait row
specs/001-loupe-v1/spec.md        # FR-039, FR-040 with dated pointers
specs/001-loupe-v1/contracts/cli.md  # wait section, handoff section, refusal table
specs/001-loupe-v1/data-model.md  # hand-back set, the .review file
specs/001-loupe-v1/validation.md  # one Unverified row
specs/003-herdr-handoff/spec.md   # dated pointer on scenario 2 and FR-004
specs/006-agent-plugins/spec.md   # dated pointer on scenario 3
```

**Structure Decision**: No `research.md`, `data-model.md`, `quickstart.md` or `checklists/`, as in 005 to 015. The research is above; the data-model changes are one file and one new writer of an existing file, recorded in spec 001's `data-model.md` where readers look for them.

## Complexity Tracking

No Constitution Check violations. Departures from contracts are the amendments this specification makes: spec 001's wait and hand-back passages and FR-023's per-decision version, and a new refusal code on `loupe handoff`.
