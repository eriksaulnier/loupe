# Feature Specification: Apply mode, where the agent applies the findings the human accepts

**Feature Branch**: `049-apply-mode`

**Created**: 2026-10-29

**Status**: Draft

**Input**: User description: "Build sift's flow (`tyrantkhan/sift`) into loupe as a second mode, with sift's author contributing to loupe. This does not take sift over. What happens to sift is its author's call. sift reviews local plans, specs and code and hands the approved findings back to the agent to apply. Its strongest idea is the per-finding `proposal`, shown as 'If you approve, `<agent>` will', so the human approves an action, not a critique." The owner wrote this on 2026-10-29, while testing `main` before a release. It became the board draft "Draft spec: hand-back mode", and a sift round of 14 decisions settled it the same day.

loupe has one mode today, publish. The pull request's author acts on the findings the human accepts, and the run ends in one GitHub review. This spec adds a second mode, apply. In apply mode the agent acts on the findings the human accepts, and the run ends when loupe returns the decisions to that agent. Both modes share the decision loop: findings, decisions, send-back notes and replies. The mode follows who acts, not what the agent reviews.

The owner named three paths: posting a review of a pull request, sorting the findings of a local review, and making decisions on a plan or a spec. In the last two, the agent acts on what the human chose. Apply mode covers both of them. A finding on an apply run has one of two shapes. A **proposal** asks the human to approve an action. A **question** has no proposal and asks the human for an answer.

## Problem and goals

An agent that reviews local work, or that has decisions for the human, has no loupe path today. The owner uses sift for that job and loupe for pull requests. The two tools have the same decision loop, two installs, two review interfaces, two sets of pane hosts and two sets of skills to keep current. A fix to one loop does not reach the other.

Goals:

- One install and one review interface for every path the owner named.
- One decision loop, so that a send-back, a rewrite or a pane-host fix lands once.
- An agent that already knows loupe needs no second tool for local work.
- Apply mode is no heavier than sift for the job sift does (see Parity under Risks).

Non-goals: this spec does not replace sift, does not change the PR flow, and does not add apply mode on a pull request.

## Terms

- **loupe**: a command line tool and terminal interface. An agent files review findings into it, a human decides each one, and loupe then acts on the decisions.
- **Run**: one directory of files under loupe's data root. It holds one capture and the decisions on it.
- **Round**: a PR run's place in the sequence of captures of one pull request. A local run has no round.
- **Draft**: the run's findings and the human's decisions, stored in `draft.json` with a schema number.
- **Receipt**: the record that a PR run published a GitHub review.
- **Handoff**: `loupe handoff`, which opens review for the human in a pane beside the agent.
- **Finding**: one item for the human to decide. The review interface shows one finding at a time, as a card.
- **PR-only concepts** (FR-022): `blocking` (a finding field), a location's diff `side` (a finding field), earlier findings and `previous` (a capture result), assessment and `assess` (a command), and sticky rounds (a published-review option).
- **Target kind**: `pr` or `local`. It decides the mode: a `pr` run publishes, and a `local` run applies.
- **Local target**: a prefix and repository, a worktree root, a branch, an optional base, an optional title, an optional applier and the capture form. It has no pull request and no round.
- **Local run reference**: `<prefix>:local#<n>`, or `local#<n>`. n counts across the data root. The prefix only helps a reader.
- **Proposal**: the action that the applier will take if the human accepts. It is Markdown, written for the human.
- **Question**: a finding with no proposal.
- **Return**: the end step of an apply run. It writes `return.json`, and the run accepts no change after it.

## Clarifications

### Session 2026-10-29

The answers below come from three sources:

- The owner's sift round `hand-back-mode-decisions-20261029-201530`, 14 decisions, labeled F1 to F14. The owner revised three of them in discussion.
- A later owner decision on the name.
- A Spec Kit clarify round, labeled C1 to C4.

Each sift round is a local run with no URL. Answers that the session of 2026-11-03 replaced say so.

- Q: How many skills does the plugin ship for the three paths? → A: Two. `human-review` stays PR-only and unchanged. A new `human-decide` skill covers every apply run, for both local review findings and plan or spec decisions. Each skill stands alone (owner, F1).
- Q: Is "making decisions" a third mode? → A: No. Apply mode covers it. A finding is a proposal or a question (owner, F2).
- Q: Does a question carry a list of options that the human picks from with a key? → A: No `options` field. The agent recommends one choice as the proposal and names the other choices in the body. The human accepts, or accepts with a note that names another choice (owner, F3).
- Q: Can a run have no files, for a decision made before any document exists? → A: Yes, and every finding on such a run is general (owner, F4). The flag is `--no-files` since 2026-11-03.
- Q: How is a local run referenced? → A: Replaced on 2026-11-03. It was `<owner>/<repo>:local#<n>`, counted per repository (owner, F5).
- Q: Do local runs have rounds? → A: No. Each local run stands alone, with no earlier findings and no assessment (owner, F6).
- Q: Does "accept with a note" count as `applyWithChanges`? → A: Yes, on an apply run only. A PR run keeps today's keys, because a note on an accepted finding there posts nowhere and nobody answers it. On a PR run, send-back stays the blocking route that makes the agent read and answer the note (owner, F7).
- Q: Does this spec include apply mode on a pull request target, for the owner's own pull request? → A: No. This spec defers it (owner, F8). Since 2026-11-03 no input can ask for it, so capture needs no refusal.
- Q: Does the agent report what it applied? → A: No (owner, F9). Since 2026-11-03 `stats` does not count local runs.
- Q: Does a `--files` run need a plain-file view? → A: No second view. The diff view draws a whole-file addition as plain text (owner, F10).
- Q: Should review skills outside loupe drive local runs? → A: Yes, directly, by loading `human-decide`. That skill MUST be enough on its own. No adapter or wrapper is part of loupe (owner, F11).
- Q: Does the end step of an apply run need a constitution amendment? → A: Replaced on 2026-11-03. The preamble gets a MINOR amendment (owner, F12).
- Q: The draft named the mode `hand-back`, but loupe already uses "hand back" for send-back notes (`handback.json`, `specs/017-live-review` and the `wait` contract). What is the mode called? → A: `apply`. `wait` returns `returned` on an apply run where a PR run returns `published`. The skill stays `human-decide` (owner, 2026-10-29, after F13 approved the old name before anyone saw the collision).
- Q: Does this spec wait until sift's author agrees to contribute? → A: No. Their role stays an open question, and it does not gate the spec (owner, F14). On 2026-11-03 the owner said to build it and talk to the author afterwards.
- Q: Can the human return an apply run while a finding is still undecided? → A: No. Return needs every finding decided, so feedback has no `open` group (owner, C1).
- Q: Is `proposal` accepted on a PR run, where nothing shows it? → A: No. `loupe add` and `loupe edit` refuse it on a PR run with `input` and a fix that names `suggestedFix`. The refusal lifts when apply mode on a pull request ships (owner, C2).
- Q: Can the human give a reason when excluding a finding on an apply run? → A: Yes, optionally. `x` opens the one-line input that the tweak key uses, and an empty input excludes with no reason. PR runs keep today's `x` (owner, C3).
- Q: Two repositories with the same directory name and no GitHub origin get the same key. Is that acceptable? → A: Replaced on 2026-11-03, because keys no longer number runs (owner, C4).

### Session 2026-11-03

The answers below come from an EGM review of this spec, with five fresh-context reviewers. The reports are in `reviews/spec/`. The owner decided the results in the sift round `spec-049-the-9-calls-that-matter-20261103-175348`. Cards 1 to 8 were single decisions, and card 9 approved decisions D9 to D36 of `reviews/spec/consolidated_review_report.md` together.

- Q: Why does apply mode belong in loupe rather than beside it? → A: One install, one review interface and one decision loop (see Problem and goals). The constitution preamble gets a MINOR amendment to cover findings returned to an agent (owner, D1).
- Q: Does a run store its mode and its target kind? → A: Only the kind. The mode follows from it. A `mode` field arrives with apply mode on a pull request (owner, D2).
- Q: How is a local run referenced? → A: `<prefix>:local#<n>`, with n counted once across the data root. The prefix names the repository for a reader. loupe neither checks it nor uses it to find the run, and `local#<n>` alone also resolves (owner, D3, and Stage 8 card 1).
- Q: Does loupe flag findings whose file changed before return? → A: No. The agent locates each change by content or heading in any case, so the flag and the per-file hashes are gone (owner, D4).
- Q: Does `handoff` refuse a finding with no proposal and no `question` label? → A: No. A finding with no proposal is a question, whatever its label (owner, D5).
- Q: Does `stats` count local runs? → A: No. Local-run stats are out of scope (owner, D6).
- Q: Is `capture --tree` part of the contract? → A: No. The plan MAY keep a tree step as internal plumbing (owner, D7).
- Q: Does `x` keep its reason input on an apply run, at the cost of a second key? → A: Yes. SC-006 counts the keys that follow from it (owner, D8).
- Q: Do PR runs change? → A: A PR `target.json` keeps its fields and gains no `kind`. Its schema number is 3, like every new target. D10 kept it at 2, but the plan review found that draft schema 5 already reaches PR runs, so the owner reversed D10 (plan card 1).
- Q: Can `--files` name a file outside the repository? → A: Yes, any readable regular file (D13).
- Q: Does a command with no `--run` ever pick a local run? → A: No (D14).
- Q: Does a local capture make the daily release lookup from `specs/048-update-warning`? → A: No. It warns from the cached result only (D15).
- Q: How does the end step seal the run? → A: With `return.json`. After that file exists, every draft change refuses with `returned` (D16).

## Constitution check

The constitution's principles assume one pull request and one review. Each principle still holds for an apply run. The preamble gets a MINOR amendment (version 3.2.0) to say that loupe also returns decisions to an agent.

- **I. A tool for agents.** loupe still invokes no reviewer and no applier. The new capture forms, `proposal`, the `returned` outcome and the feedback groups are all reachable from `loupe --help`. `human-decide` packages instructions, and it adds no code path that only one host can reach.
- **II. Nothing posts unread under a human's name.** This principle covers what reaches GitHub. An apply run never contacts GitHub (FR-008), so it posts nothing under anyone's name. The agent that applies the accepted findings edits files in the human's session, which the human already allows. Per-finding sign-off still holds. The human decides one finding at a time, with that finding on screen, and no key decides several findings. The settle guard covers the new keys (FR-019).
- **III. Local files, no service.** An apply run is plain files in one run directory. A local capture makes no network call (FR-008).
- **IV. Never touch the user's checkout.** FR-004 states what `--changed` MUST NOT touch. `--files` and `--no-files` write no Git object.
- **V. Machine contract first.** Every new flag, refusal and result shape is in `--help` and `contracts/cli.md`. The new record fields follow `docs/versioning.md`.
- **VI. Simplicity over ceremony.** The mode interface (FR-030) has two concrete uses from the start, publish and apply. That meets the second-use test. The run stores no `mode` field until a second pairing exists.
- **VII. Verified means ran.** An apply run never touches GitHub. Its tests run against local Git repositories and injected terminal input, and they assert that a fake GitHub receives no request (SC-004).

## Relationship to earlier specifications

This spec adds a target kind beside the pull request that every spec so far assumes. It reuses three earlier specs:

- `specs/046-edit-then-accept`: the rewrite pane lets the human rewrite a proposal.
- `specs/044-heading-anchors`: heading anchors let a finding point at a section of a plan or a spec.
- `specs/041-withdraw-reason`: withdraw reasons let the agent withdraw a finding with a reason. Its reason rules also apply to the human's exclude reason.

It amends `specs/048-update-warning`. That spec's FR-001 becomes "`loupe capture` of a pull request MUST look up …", because a local capture makes no network call.

For a PR run, the keys, the publish steps, `docs/comment-format.md`, the published review and the fields of `target.json` stay the same. Four things change on a PR run:

- `loupe add` and `loupe edit` refuse `proposal`.
- `target.json` says schema 3, and `draft.json` says schema 5.
- `--run` also accepts a local reference.
- `loupe --help` and `loupe handoff --help` name the new forms.

This spec amends these documents:

- `specs/001-loupe-v1/contracts/cli.md`: the local capture forms, `--applier`, the local run reference and its selection rule, the `returned` wait outcome, the feedback groups, the `list` shape of a local run, and the refusal codes `returned` and `wrong-mode`.
- `docs/versioning.md`: draft schema 5 for `findings.proposal` and `decisions.note`, target schema 3 for `kind` and the `local` block, and the new record `return.json` at schema 1.
- `.specify/memory/constitution.md`: the preamble.
- `README.md` and `AGENTS.md`: the opening line that says what loupe does.

"Hand back" keeps its meaning from `specs/017-live-review`: a send-back note handed to the agent while review stays open. This spec does not use the term for the end of a run.

## Alternatives considered

- **Keep sift separate, with a shared finding contract.** This costs the least now. But it keeps two installs, two review interfaces and two pane-host sets, and every fix to the decision loop lands twice. It is the fallback that Parity under Risks names.
- **Model a local target as a pseudo pull request, with a fake number and round.** That would reuse today's reference, layout and listing. But every PR-only path (publish, previous rounds, `stats`, the receipt) would then need a guard against a pull request that does not exist. A separate kind keeps those paths untouched.
- **A separate command group, such as `loupe local add`.** That would duplicate eight commands and their help and contracts. Agents would also learn two command sets for one loop.
- **Reuse `suggestedFix` as the proposal.** `suggestedFix` is published text for a pull request's author, and `proposal` is a private action for the human who decides. Their readers differ, so one field would carry two meanings (FR-014).
- **Store both `mode` and `kind`.** They are always 1:1 in this spec, so the second field has no use yet (Principle VI).
- **Number local runs per repository (`<owner>/<repo>:local#<n>`).** That needs a key derived three ways, a lock per key, a rule for unsafe directory names and an accepted collision. It also blocks plans kept outside the repository. A global sequence removes the per-key lock, the collision and the block on outside plans. The prefix still needs the repository, which `list` shows in any case, but loupe never checks it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Decide a local code review and let the agent apply it (Priority: P1)

An agent reviews the uncommitted and committed changes on a branch. It captures them with `loupe capture --changed --base main --applier Claude`, files its findings with a proposal on each, and runs `loupe handoff`. The human reads each card. Under the title, the card says "If you accept, Claude will" and then the proposal. The human accepts most cards, excludes one, and accepts one with a note. Then the human uses the return action. `loupe wait` returns `returned`. `loupe feedback --json` lists the findings in `apply`, `applyWithChanges` and `excluded`, each with the absolute file path. The agent applies them.

**Why this priority**: It is the job sift does today. If loupe cannot do it with no more effort than sift, apply mode does not belong in loupe.

**Independent Test**: In a local Git repository with a branch that has one commit and one unstaged edit, capture with `--changed --base main` and add two findings with proposals. Then use injected input to accept one, exclude one and return. `wait --json` returns `returned`, and `feedback --json` matches a golden.

**Acceptance Scenarios**:

1. **Given** a repository with committed, staged, unstaged and untracked changes on a branch, **When** the agent runs `loupe capture --changed --base main --json`, **Then** the run's diff holds all four kinds of change against the merge base, the result marks each file that `HEAD` does not hold, and the user's index, working files, branch, HEAD and refs are unchanged.
2. **Given** a local run, **When** the agent adds a finding with a `proposal`, **Then** the detail view shows the proposal directly under the title, headed "If you accept, `<applier>` will".
3. **Given** a local run whose findings are all decided, **When** the human uses the return action, **Then** review shows no Action, Inline comments or Message step. It asks for one confirmation and writes `return.json`.
4. **Given** a returned local run, **When** the agent runs `loupe wait --json`, **Then** the result is `"reason": "returned"` and carries the feedback payload.
5. **Given** a returned local run, **When** the agent runs `loupe feedback --json`, **Then** decided findings are grouped into `apply`, `applyWithChanges`, `excluded` and `withdrawn`. Each anchored entry carries the absolute path of its file and the finding's location.
6. **Given** a local run, **When** any of its commands runs, **Then** no request reaches GitHub.
7. **Given** a returned local run, **When** the agent runs `loupe add`, `loupe edit` or `loupe reply` on it, **Then** each refuses with `returned` and a fix that names a new capture.

---

### User Story 2 - Answer an agent's decisions on a plan or a spec (Priority: P1)

An agent has written a plan and has questions on it. It captures the plan with `loupe capture --files docs/plan.md` and files one finding per decision. Each finding proposes the choice the agent recommends, and its body names the other choices. The human accepts the findings they agree with. On one finding the human accepts with a note that names another choice. The agent reads `applyWithChanges` and edits the plan to follow the note.

**Why this priority**: It is the second path the owner named, and it shares apply mode with User Story 1. It needs whole-file capture, heading anchors and accept-with-note.

**Independent Test**: Capture one Markdown file with `--files`, then add one finding anchored to a heading and one general question. Accept the first with a note, accept the second, and return. `feedback --json` holds the first in `applyWithChanges` with the note as `suggestion`, and the second in `apply`.

**Acceptance Scenarios**:

1. **Given** a Markdown file, **When** the agent runs `loupe capture --files <path> --json`, **Then** the run stores the file as a whole-file addition, and the result lists its headings.
2. **Given** a Markdown file outside the repository, such as a plan in the agent host's own directory, **When** the agent captures it with `--files`, **Then** the capture succeeds, and feedback gives the file's absolute path.
3. **Given** a `--files` run, **When** the human opens a finding, **Then** the code pane shows the file as plain text, with no `+` gutter or added-line tint.
4. **Given** a finding anchored to a heading in a `--files` run, **When** the human opens it, **Then** the code pane shows that section, as `specs/044-heading-anchors` does.
5. **Given** an apply run, **When** the human presses the tweak key on a finding, types a note and confirms it, **Then** the finding is accepted, and the note is stored with the decision.
6. **Given** an apply run where the human rewrote a proposal with `w` and accepted it, **When** the agent reads feedback, **Then** the finding is in `applyWithChanges` with the human's rewritten proposal.
7. **Given** an apply run where the human accepted with a note, **When** the agent reads feedback, **Then** the finding is in `applyWithChanges` with the note as `suggestion`.
8. **Given** a PR run, **When** the human looks for the tweak key in the footer or help, **Then** it is not there, and the PR keys are unchanged.

---

### User Story 3 - Ask questions before any document exists (Priority: P2)

An agent starts a new piece of work and has three decisions to make before it writes anything. It runs `loupe capture --no-files --title "Cache design" --applier Claude` and files three general findings. The human decides them and returns. The agent writes its plan from the decisions.

**Why this priority**: It closes a gap that sift has today, where an agent writes a file only so that the human can answer questions about it. It matters less than User Stories 1 and 2, because a file usually exists.

**Independent Test**: Run `capture --no-files --title X` and add two general findings. Confirm that an anchored finding refuses. Decide and return. `feedback --json` holds both findings.

**Acceptance Scenarios**:

1. **Given** no files, **When** the agent runs `loupe capture --no-files --title "Cache design" --json`, **Then** a run starts with no files, and its reference has the form `<prefix>:local#<n>`.
2. **Given** a run started with `--no-files`, **When** the agent adds a finding with a `location`, **Then** the add refuses with a fix to file it with `"general": true`.
3. **Given** `--no-files` without `--title`, **When** the agent runs capture, **Then** it refuses with `usage` and a fix that names `--title`.
4. **Given** a question with no proposal, **When** the human accepts it without a note, **Then** feedback holds it in `apply`, which tells the agent to proceed as the body describes.

---

### User Story 4 - Send a finding back and get an answer (Priority: P2)

The human does not understand a proposal and sends it back with a note. The agent reads the note, revises the proposal with `loupe edit`, and replies. The human sees the reply while review stays open, accepts the revised finding, and returns the run.

**Why this priority**: It is the shared decision loop. It already works on PR runs. This story pins that it works the same way on an apply run.

**Independent Test**: On an apply run, send one finding back with injected input. `wait --json` returns `notes`. Edit the proposal, reply, accept with injected input, and return.

**Acceptance Scenarios**:

1. **Given** an apply run, **When** the human sends a finding back, **Then** `wait` returns `"reason": "notes"` while review stays open, as on a PR run.
2. **Given** a sent-back finding, **When** the agent runs `loupe edit <id>` with a new `proposal`, **Then** the decision clears, and the detail view shows the new proposal.
3. **Given** a discussed finding in a returned run, **When** the agent reads feedback, **Then** the finding's entry carries its notes and replies.

---

### User Story 5 - Two skills for three paths (Priority: P2)

An agent that reviews a pull request loads `human-review`, as it does today. An agent that reviews local work or has decisions for the human loads `human-decide`. Neither skill tells the agent to read the other. A review skill from outside loupe loads `human-decide` and needs nothing else to capture, file, hand off, wait and apply.

**Why this priority**: It is how an agent finds the right path without reading PR rules it does not need.

**Independent Test**: The plugin test lists both skills. `human-decide` names only the commands of FR-027. It does not mention `previous`, `assess`, `blocking`, a diff side, sticky rounds or `publish`.

**Acceptance Scenarios**:

1. **Given** the plugin, **When** a host lists its skills, **Then** it lists `human-review` and `human-decide`, and `human-review`'s description is unchanged.
2. **Given** `human-decide`, **When** an agent follows it from capture to applying the decisions, **Then** it runs no command beyond those in FR-027.
3. **Given** a rule that both skills carry, **When** the plugin test runs, **Then** it finds the rule's line verbatim in both skills.

---

### User Story 6 - Find a local run again (Priority: P3)

The human has local runs in two worktrees of one repository and a PR run of the same branch. They run `loupe list` and see every run with its state.

**Why this priority**: It keeps local runs visible without a new command. It matters less than the decision loop itself.

**Independent Test**: Capture one local run in each of two worktrees of one repository, and one PR run. `list --json` shows two local runs with consecutive n and the PR run, each with its state.

**Acceptance Scenarios**:

1. **Given** two worktrees of one repository, **When** each captures a local run, **Then** the two references share the prefix and have consecutive n.
2. **Given** local runs and PR runs, **When** the human runs `loupe list --json`, **Then** each local run shows `ref`, `repository`, `title` and `state` (`captured`, `ready` or `returned`), with no `url` or `round`.

---

### Edge Cases

- **Nothing changed.** `--changed` with no change against the base refuses, with a fix that names `--base` or `--files`.
- **Binary, empty, deleted or large file.** The capture skips a binary, empty or deleted file, and any file over 1 MiB. It lists each one under `skipped` with the reason `binary`, `empty`, `deleted` or `too-large`, as sift does. A submodule is skipped as `submodule`.
- **A missing `--files` path.** The capture refuses and names the path. A typo is never skipped in silence.
- **An unborn HEAD or a sparse checkout.** `--changed` refuses, with a fix that names `--files`.
- **A local run and a PR run of the same branch at once.** They are separate runs and share no state. A command with no `--run` still resolves to the PR run (FR-007).
- **Two local runs open at once.** They are separate runs. `handoff` refuses with `review-open` only for the same run.
- **Worktrees.** All worktrees share one local run sequence (FR-006). Each run records the worktree root and branch it captured, and feedback paths point into that worktree.
- **No GitHub origin.** The prefix is the main worktree's directory name. Characters outside `A-Z`, `a-z`, `0-9`, `.`, `_` and `-` become `-`.
- **Outside a Git repository.** `--files` and `--no-files` work there. The run's root is the working directory, it has no branch, and its reference is `local#<n>` with no prefix. `--changed` refuses with a fix that names `--files`.
- **A finding about deleted code.** It anchors to the nearest remaining line, or it is general (FR-015).
- **A PR-only field on an apply run.** `add` and `edit` refuse `blocking`, `side` and `suggestedFix` (FR-013, FR-014).
- **A PR-only command on a local run.** `publish`, `assess`, `show --previous` and `show --comments` refuse with `wrong-mode` (FR-025).
- **A proposal on a PR run.** `loupe add` and `loupe edit` refuse it with `input`, and the fix names `suggestedFix`.
- **The human leaves a finding undecided.** Return refuses while any finding is undecided (FR-020), so feedback never holds an undecided finding.
- **Every finding excluded.** Return succeeds. The agent changes nothing.

## Requirements *(mandatory)*

### Functional Requirements

#### Targets and capture

- **FR-001**: A run's target kind MUST be `pr` or `local`. Every new target MUST write target schema 3. A local target MUST record `kind: local`. A PR target MUST keep today's fields with no `kind` field, and a missing `kind` MUST read as `pr`. The run's mode MUST follow from its kind: a `pr` run publishes, and a `local` run applies.
- **FR-002**: A local target MUST record its reference prefix and repository (`<owner>/<repo>` when `origin` names a GitHub repository, read from Git config with no network call), the worktree root, the branch, the base ref when one was given, the title when one was given, the applier when one was given, and the capture form.
- **FR-003**: `loupe capture --changed [--base <ref>]` MUST capture committed, staged, unstaged and untracked changes, excluding ignored files, as a diff against the merge base of `HEAD` and the base, or against `HEAD` when no base is given.
- **FR-004**: `--changed` MUST NOT read, write or lock the user's index. It MUST NOT change working files, the current branch, `HEAD` or any ref outside `refs/loupe/`. Whether capture writes Git objects into the user's repository is the plan's choice.
- **FR-005**: `loupe capture --files <path>...` MUST capture each named file as a whole-file addition. A path MAY lie outside any repository. The run MUST store a path inside the root as relative to it. A path under the home directory MUST show as `~/<rel>`, and any other path as `ext/<i>/<base name>`, since a diff header cannot hold an absolute path (research R3). `local.files[].abs` MUST hold every file's absolute path.
- **FR-005a**: `loupe capture --no-files --title <name>` MUST start a run with no files. Every finding on that run MUST be general.
- **FR-005b**: Every local capture MUST accept `--title <name>`, `--applier <name>`, `--source` and `--model`. `--applier` MUST be one line of at most 64 characters, and `--title` one line of at most 120 characters. Each MUST NOT hold a control character, and both MUST pass the Markdown allowlist. Otherwise capture refuses with `usage`. `--source` and `--model` MUST follow their PR-run rules, and on a local run they record provenance only.
- **FR-005c**: A local capture's result MUST list each captured file's path and, for Markdown, its headings. It MUST mark each file that `HEAD` does not hold, which covers untracked and newly staged files, and it MUST list skipped files under `skipped`, each with its reason (see Edge Cases). Telling the two apart would need the user's index (FR-004).

#### Run references and selection

- **FR-006**: A local run's reference MUST be `<prefix>:local#<n>`, or `local#<n>` outside a Git repository. n MUST count local runs from 1 across the whole data root, under one lock. The prefix MUST be the repository from FR-002, or the sanitized directory name of the main worktree when `origin` is not on GitHub. Local runs MUST live in a directory of their own, so the layout of PR runs is unchanged.
- **FR-006a**: Every command that takes `--run` MUST accept a local reference. A command MUST resolve a local reference by n alone and MUST NOT check its prefix, and `local#<n>` with no prefix MUST resolve. A malformed reference MUST refuse with `usage`, as today.
- **FR-007**: loupe MUST NOT select a local run unless the command names it through `--run`, `LOUPE_RUN` or a positional ref. A command with none of them MUST keep today's PR resolution. Every `next` hint of a local run and `human-decide` MUST pass `--run`. The pane that `handoff` opens runs `review '<ref>'` with the positional ref (research R10).
- **FR-008**: A command on a local run MUST NOT contact GitHub. A local capture MUST NOT run the release lookup from `specs/048-update-warning`. It MAY warn `update-available` from the cached result, as `review` does. Review on a local run MUST NOT offer the key that opens a pull request.

#### Findings

- **FR-009**: `proposal` MUST be an optional finding field on an apply run. It MUST be Markdown under the allowlist in `docs/comment-format.md`, at most 64 KiB, and the finding-id rule of `docs/comment-format.md` MUST apply to it. Adding it bumps the draft schema to 5 under `docs/versioning.md`. The change is additive, so it needs no migration step. On a PR run, `loupe add` and `loupe edit` MUST refuse `proposal` with `input` and a fix that names `suggestedFix`.
- **FR-010**: On an apply run, a finding with no `proposal` MUST count as a question, whatever its label. `human-decide` MUST tell the agent to give every finding a proposal, which for a question is the recommended answer, and to name the other choices in the body.
- **FR-011**: The run MUST record the applier, the agent that will act on accepted findings, from `--applier`. It MAY differ from the reviewer.
- **FR-012**: On an apply run, the detail view MUST show `proposal` directly under the title, in the good color (the `Good` tone of `internal/style`), headed "If you accept, `<applier>` will", or "If you accept, the agent will" when no applier was given. The list MUST mark each question.
- **FR-013**: On an apply run, `loupe add` and `loupe edit` MUST refuse `blocking` with `input` and a fix to drop it.
- **FR-014**: `suggestedFix` MUST stay a PR field, because it is written for a pull request's author. On an apply run, `loupe add` and `loupe edit` MUST refuse it with `input` and a fix that names `proposal`.
- **FR-015**: On an apply run, a finding's location MUST follow today's rule: a line inside a stored hunk, context lines included, or a heading whose section has exactly one changed hunk. The location always refers to the new side. `loupe add` MUST refuse `side` with a fix to drop it. On a `--files` run the whole file is one hunk, so every line and heading is valid. Heading anchors from `specs/044-heading-anchors` MUST work on `--files` and `--changed` runs.

#### Decisions and the end of the run

- **FR-016**: On an apply run, the human MUST be able to accept a finding with a note, with one key, the tweak key `t`. The note MUST follow the text rules of a send-back note, except that an empty note MUST accept with no note. A rewrite of the proposal with `w` (`specs/046-edit-then-accept`) MUST stay available. loupe MUST NOT add a decision value. A PR run MUST NOT offer the tweak key.
- **FR-017**: On an apply run, `w` MUST let the human rewrite the proposal as well as the title and body.
- **FR-018**: On an apply run, `x` MUST open the same one-line input as the tweak key. Text confirmed there MUST exclude the finding, with that text as the human's reason, under the reason rules of `specs/041-withdraw-reason`: one line of at most 200 characters. An empty input MUST exclude the finding with no reason. A PR run MUST keep today's `x`.
- **FR-019**: The accept note and the exclude reason MUST live on the decision as `decisions[].note`. Only the human MAY set it, and it MUST clear with the decision. Draft schema 5 MUST list `findings.proposal` and `decisions.note`. The settle guard that drops typed-ahead decision keys MUST cover `t`, and on an apply run `x`. An input opened by either key MUST start empty.
- **FR-020**: The end step of an apply run is the return action, on `p` in the list view, labeled "return". It MUST refuse with `not-ready` while any finding is undecided or any note is open, and with `empty` when the run holds no finding. It needs no summary. A run whose findings are all excluded MUST return. After one confirming key, it MUST check that the draft has not changed since the human confirmed, and refuse with `changed` if it has, as publish does.
- **FR-021**: The return action MUST write `return.json`, a record of its own at schema 1. It holds `returnedAt` and the draft version it sealed, and nothing else. `feedback` and `wait` MUST read the decisions from the draft. After `return.json` exists, every draft change (`add`, `edit`, `reply`, `summary` and any decision) MUST refuse with the new code `returned`, and the fix MUST name a new capture. `loupe review` and `loupe handoff` on a returned run MUST refuse with `returned`, and the fix MUST name `loupe feedback`. After the human confirms return, review MUST show that the run was returned and let the human quit. The PR receipt MUST NOT change.
- **FR-022**: PR-only fields and states MUST NOT appear on an apply run: `blocking`, a location's diff side, earlier findings, assessment and sticky rounds. A local run MUST stand alone, with no `previous`, no `assess`, no carried findings and no round in its reference. `severity` MUST stay optional, because the human uses it to order a long list.
- **FR-023**: `wait` on an apply run MUST return `"reason": "returned"` once `return.json` exists, where a PR run returns `published`, and MUST carry the feedback payload.
- **FR-024**: `feedback --json` on an apply run MUST keep today's `readiness`, `notes` and `findings`. It MUST add `returned`, a boolean, and `groups`. `groups` holds the decided findings in `apply`, `applyWithChanges`, `excluded` and `withdrawn`. A finding accepted with a note or with a human rewrite MUST be in `applyWithChanges`, with the note as `suggestion` and the finding's current proposal. Each anchored entry MUST carry the absolute path of its file and the finding's location. An excluded entry MUST carry the human's reason as `reason` when one was given. Each entry MUST carry its notes and replies.
- **FR-025**: On a local run, `publish`, `assess`, `show --previous` and `show --comments` MUST refuse with the new code `wrong-mode`. The fix MUST name the apply-run equivalent: the return action in review, or `feedback`.

#### Listing

- **FR-026**: `loupe list` MUST include local runs, each with `ref`, `kind`, `repository` (when the run has a GitHub origin), `title`, `state` (`captured`, `ready` or `returned`), `counts` and `capturedAt`, and with no `url` or `round`. `loupe stats` MUST ignore local runs.

#### Parity with sift

- **FR-027**: An agent MUST need no commands beyond `capture`, `add`, `summary`, `handoff`, `wait`, `reply`, `edit` and `feedback`. They cover sift's `start`, `add`, `summary`, `handoff`, `wait`, `reply` and `feedback`. `reply` and `edit` together cover sift's `reply` and its revision flags. `edit` also withdraws a finding (`--exclude`, with `withdrawReason`) and restores one (`--include`), as on a PR run.
- **FR-028**: An apply run MUST meet FR-027 and FR-029 with no change to the PR-run contracts in `contracts/cli.md` and `docs/comment-format.md`, other than the additions this spec lists under Relationship to earlier specifications.
- **FR-029**: On an apply run, the human MUST need no keys beyond accept, tweak, exclude, send back, rewrite, restore and return, plus today's note and navigation keys. The PR key `e`, which sets `blocking`, MUST NOT appear. The Action, Inline comments and Message publish steps MUST NOT appear. Confidence, verification, impact and references MUST stay optional, and `human-decide` MUST NOT ask for them.

#### Skills and structure

- **FR-030**: Capture MUST choose its target kind from its flags. The command layer and the review interface MUST reach mode-specific behavior only through the run's mode: run reference and selection, command availability, the field and location rules, the review surface (keys, hints, help and end step), the end record, the wait reason, the feedback shape and the list entry. Code MUST NOT branch on the target kind outside the mode implementations, the run layer and capture.
- **FR-031**: The plugin MUST ship a second skill, `human-decide`, for every apply run. `human-review` MUST stay PR-only, and its description MUST NOT change. Each skill MUST NOT tell the agent to read the other. A rule that both skills carry MUST use the same wording in both, and a plugin test MUST check that each such line appears verbatim in both.
- **FR-032**: A review skill that loads `human-decide` MUST need no other instructions, adapter or wrapper to capture, file findings, hand off, wait and apply the decisions. `human-decide` MUST tell the agent four things: read the capture's list of new files before `handoff`, locate each change by content or heading, count a finding's thread alongside its proposal, and follow `suggestion` where it conflicts with `proposal`. loupe code and skills MUST NOT name a review skill from another repository.

### Key Entities

The entities are defined under Terms. Roles on an apply run:

| Role | Runs | Decides |
| :--- | :--- | :--- |
| Reviewer | `add`, `summary`, `edit` | Which findings to file |
| Applier | `capture`, `handoff`, `wait`, `reply`, `edit`, `feedback`, and the edits | How to apply each accepted finding |
| Human | `review` | Each finding, then the return |

One agent usually plays both the reviewer and the applier. When they differ, the applier MAY add proposals with `loupe edit` before `handoff`.

## Risks

- **Secrets in untracked files.** `--changed` takes every untracked file that `.gitignore` does not exclude, and loupe never deletes a run on its own (Principle III). A stray credentials file can land in the run. Mitigation: the result marks each file that `HEAD` does not hold, and `human-decide` tells the agent to read that list before `handoff` (FR-032).
- **Storage growth.** Local runs will outnumber PR rounds, and each holds copies of files. Mitigation: files over 1 MiB are skipped as `too-large`. Runs are plain directories that the human MAY delete.
- **Capture cost on a large worktree.** Building the diff without the user's index can rehash every tracked file. Mitigation: the plan measures `--changed` on a large repository and states the cost.
- **Fidelity of the applied change.** Nothing checks the agent's edits against the proposal that the human accepted, and sift has the same gap. Mitigation: `human-decide`'s rules, and the human sees the edits in their own session.
- **Parity.** If the design cannot meet FR-027 and FR-029 without changing the PR contracts (FR-028), apply mode does not belong in loupe, and a shared finding contract with sift is the fallback. The plan checks this before it adds any PR-visible change.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A seeded local run of one plan and one code file goes from capture to return with only the commands in FR-027, and `feedback --json` matches a golden.
- **SC-002**: Review-interface tests show the proposal under its heading and the single return step.
- **SC-003**: Every golden under `testdata/golden/cli` is byte-identical, except the `help.*` and `handoff-help.*` goldens, which change on purpose to name the new forms.
- **SC-004**: In every local-run test, the fake GitHub receives no request, the release endpoint included.
- **SC-005**: In every `--changed` test, the user's index, working files, branch, `HEAD` and refs are byte-identical before and after capture.
- **SC-006**: On a local run of ten findings, the human uses one key for each accept and two keys for each exclude with no reason (`x`, then Enter). Two keys return the run (`p`, then the confirming key), with no publish step.
- **SC-007**: `mise run check` passes after the final edit.

## Assumptions

- The tweak key is `t`. It is free in every view today.
- The return action reuses `p` in the list view, labeled "return" on an apply run, so the human's habit stays the same.
- The detail view keeps its code pane on apply runs. A `--no-files` run shows no code pane.
- Draft schema 5 assumes that no other spec takes schema 5 first. If one merges first, this spec takes the next number at merge, as `specs/046-edit-then-accept` described for `specs/041-withdraw-reason`.
- `loupe summary` stays optional before `handoff` on an apply run, as it is on a PR run today.
- A type40 change makes the engineer's local review skill load `human-decide` after loupe releases. type40 is the engineer's agent setup, in its own repository, so that change is not part of this spec.
- This specification authorizes no commit, push, pull request, release or live review.

## Open questions

- Does sift's author want to contribute, and in what role: co-maintainer, owner of apply mode, or design input? It does not gate this spec.

## Out of scope

These are future drafts, because each applies to both modes:

- `handoff --beside <terminal-handle>`, or honoring the caller's `ORCA_TERMINAL_HANDLE`, so that an agent driving other agents opens a child agent's review beside the human's session.
- `wait --timeout` reporting whether a review pane is open, like sift's `reviewOpen`.
- A read-only `loupe export --run <ref> --json` with findings, decisions and file hashes. Check first whether `show --json` and `return.json` already cover it.

These were excluded by decision:

- Apply mode on a pull request target (F8).
- An `options` field for questions (F3).
- An "applied" report from the agent (F9).
- `loupe stats` for local runs (D6).
- A flag on findings whose file changed before return (D4).
- `capture --tree` (D7).
- A closing note on the run as a whole, like sift's `note`. The Message step stays a PR step (Stage 8).
