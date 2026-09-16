# Feature Specification: Unattended publish for any review pipeline

**Feature Branch**: `007-unattended-publish`

**Created**: 2026-09-15

**Status**: Draft

**Input**: Owner design in `specs/007-unattended-publish/design.md`, reframed on owner review on 2026-09-15: "Let people publish reviews from any GitHub review action in loupe's standard format, whether the reviewer is Claude's action, a custom agent or another pipeline. Capture the pull request with loupe, have the reviewer file structured findings and a summary through loupe, then run `loupe publish --unattended` in a separate step. loupe owns formatting, location validation, publication and recovery from uncertain sends; the caller chooses the reviewer, the model and the review method. Unattended publication requires a GitHub App installation token, sends only a COMMENT review and marks it unattended. The human workflow stays as it is. A Claude-based workflow in this repository is an example integration, not the point. Bring your reviewer; publish through loupe."

## Relationship to earlier specifications

This specification rests on constitution 2.0.0, whose Principle II was redefined for it. Under 1.0.1 every published finding needed a human's accept and an interactive confirmation. Under 2.0.0 that holds for every finding posted under a human's identity. An unattended publication MAY skip both only with a GitHub App installation token, and MUST post as that App, send COMMENT and be marked unattended.

It amends `specs/001-loupe-v1/spec.md` for `loupe publish --unattended` only. Every rule below stays in force, unchanged, for publish without the flag:

- **FR-025**: the refusals without an interactive terminal (`tty`), on the viewer's own pull request (`own-pr`) and on a draft that is not ready (`not-ready`) do not apply. The `head-moved` refusal for a captured commit that left the pull request's history and the `empty` refusal still apply. The approve-only refusals cannot arise, since the action is always comment.
- **User Story 3, acceptance scenario 7** ("no interactive terminal … refuses before showing anything") does not apply, and neither do scenario 6 (pending finding or open note refuses), FR-027 (show the review and send only on confirmation) or FR-028's viewer recheck and confirmation-window checks.
- **The composition rule** that a review is built only from accepted findings, recorded as the `ready` state in `data-model.md` and enforced when the review is composed, does not apply. Unattended composes from the whole publishable set. Skipping the readiness gate alone is not enough; composition MUST also accept pending findings.
- **FR-031**: reconciliation still matches by the hidden marker, and an unmatched attempt still refuses until a retry is explicitly requested. For an unattended attempt, which has no recorded viewer, the match tests the review's author by kind instead of by login (FR-017 here).
- **FR-037**: a run is still addressable by pull request, but `--unattended` MUST NOT use the fallback to the current branch's pull request (FR-001 here).
- **FR-042**: the round an unattended review names comes from the pull request's reviews on GitHub, not from local run state (FR-015 here).

FR-025 in spec 001 carries a one-line pointer to this specification. The draft model, the send-back loop, the review interface and the plugin skill are unchanged. The skill's ban on an agent publishing (`plugin/skills/human-review/SKILL.md`) stays as written.

The CLI contract in `specs/001-loupe-v1/contracts/cli.md` gains one flag, one refusal code, a run-selection rule for that flag and a wider meaning for `viewer`. The published format in `docs/comment-format.md` gains one footer segment and one `loupe-meta` key. Both are contracts, so this specification lists the changes in FR-025.

### Scope and positioning

The feature is a publication layer, not a reviewer. loupe owns the published format, location validation, the one review request, the receipt and the recovery from an uncertain send. The caller owns everything else: which reviewer runs, which model it uses, how it reads the code and what it decides to say.

- loupe MUST NOT invoke, prompt or authenticate the reviewer, per Principle I. A pipeline calls loupe; loupe never calls the pipeline.
- This complements hosted reviewers such as Greptile rather than competing with them. It is for people who already run their own review pipeline and want its findings published in one consistent, reviewable format.
- A review action that posts its own comments needs an adapter, or an output-only mode, on the caller's side: something that turns its findings into `loupe add` input. loupe ships no adapters and names no specific action in its contract.
- The Claude workflow in this repository (User Story 4) is an example integration and the live check for what a fake GitHub cannot prove. Nothing in the feature depends on it.

### Why this fits the constitution

Principle II exists so that nothing reads as a person's judgment unless that person read it. An unattended review posts under the App's bot account, says `unattended` in its footer and marker, and can only COMMENT, so it never approves or blocks a merge. In CI, publishing is a step a human wrote and merged, not something the reviewer chooses to do.

Principle I forbids code paths only one host can reach. The gate is the token's kind, not `GITHUB_ACTIONS` or any other environment variable, which any process can set and which would tie the path to one CI system. Any pipeline holding an App installation token qualifies.

Principle VI asks for a second concrete use before a new abstraction. Reporting the token's kind has two: capture and publish both branch on it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pipeline publishes its reviewer's findings (Priority: P1)

A team runs its own reviewer in CI: an agent, a static analyzer with a wrapper, anything that can write JSON. Their job captures the pull request with loupe, which prints the run reference. Their reviewer, or a small adapter, files findings with `loupe add --from` and a summary with `loupe summary`. A separate step runs `loupe publish <ref> --unattended --json`, with a token their CI system already holds. One COMMENT review appears on the pull request, in loupe's format, under the App's bot account, marked unattended, naming their pipeline in the footer through `--source`.

**Why this priority**: It is the feature. Everything else is a refusal, a recovery or an example.

**Independent Test**: With a fake GitHub in installation-token mode, capture a pull request, file two findings and a summary from a JSON file, and run `loupe publish <ref> --unattended --json` with no terminal. Confirm one review request with event COMMENT, both findings in the body, ` · unattended` in the footer, `unattended=1` in `loupe-meta`, and a receipt naming the bot login.

**Acceptance Scenarios**:

1. **Given** a run captured with an installation token and a draft with pending findings and a summary, **When** `loupe publish <ref> --unattended` runs without a terminal, **Then** exactly one review request is sent with event COMMENT, holding every pending and accepted finding, and a receipt is written.
2. **Given** that review, **When** it is read on GitHub, **Then** the footer's round segment is followed by ` · unattended`, and `loupe-meta` carries `unattended=1` directly after `round=`.
3. **Given** a capture made with `--source`, **When** the review is published unattended, **Then** the footer names that source exactly as it does for an attended review.
4. **Given** `LOUPE_RUN` names the run, **When** `loupe publish --unattended` runs with no positional reference, **Then** it acts on that run.
5. **Given** a pull request already holding two reviews by a `[bot]` author whose bodies carry a `loupe-meta` marker, **When** a fresh pipeline publishes unattended, **Then** the review names round 3.
6. **Given** a pull request holding a loupe review a human published, and no bot loupe reviews, **When** a pipeline publishes unattended, **Then** the review names round 1.
7. **Given** a draft with a withdrawn finding, **When** the pipeline publishes unattended, **Then** the withdrawn finding is not in the review.
8. **Given** `loupe publish <ref> --unattended` with `--action` omitted, **When** it runs, **Then** it publishes as comment.
9. **Given** a run that already has a receipt, **When** `loupe publish <ref> --unattended` runs again, **Then** nothing is sent and the review URL is printed.
10. **Given** a data root carried to a second job and restored at a different absolute path, **When** publish runs there with the same run reference, **Then** it publishes the same review as it would have in the first job, with no clone present.
11. **Given** a restored data root holding an unknown attempt whose review did land, **When** publish runs, **Then** reconciliation matches it, writes a receipt and sends nothing.
12. **Given** a restored data root holding an unknown attempt with no matching review on the pull request, **When** publish runs without `--retry-unknown`, **Then** it refuses with `attempt` and sends nothing.
13. **Given** the same draft, **When** a human publishes it without `--unattended` in the attended flow, **Then** the body is byte for byte what it was before this feature.

---

### User Story 2 - Publication without an App token is refused (Priority: P2)

A local agent in a human's session, holding the human's `gh auth` token, runs `loupe publish --unattended`. loupe refuses before sending anything, naming the token requirement. The same happens for a personal access token of a bot user account, and for a token an App issued to act for a user. A pipeline that asks for `--action approve` gets a usage error, as does one that names no run at all. A human who publishes a run a pipeline captured, or a pipeline that publishes a run a human captured, is refused because the token kinds differ.

**Why this priority**: These refusals are what keep Principle II's line where the constitution drew it. The happy path without them posts unread reviews under human names.

**Independent Test**: For each token kind other than an installation token, run `loupe publish <ref> --unattended` against a ready fake GitHub and confirm refusal with the new code and no review request. Then capture with one kind and publish with the other, in both directions, and confirm the `viewer` refusal.

**Acceptance Scenarios**:

1. **Given** a token with prefix `gho_`, `ghp_`, `github_pat_` or `ghu_`, **When** `loupe publish <ref> --unattended` runs, **Then** it refuses with code `token`, a fix line naming `GITHUB_TOKEN` and `permissions: pull-requests: write`, and sends no review request.
2. **Given** `--unattended` with `--action approve` or `--action request-changes`, **When** the command parses its flags, **Then** it exits 2 with a `usage` error before resolving a run or a token.
3. **Given** `--unattended` with no positional reference and no `LOUPE_RUN`, **When** the command runs, **Then** it exits 2 with a `usage` error naming both ways to select the run, and MUST NOT fall back to the current branch's pull request.
4. **Given** a run captured with a user token, **When** `loupe publish <ref> --unattended` runs with an installation token, **Then** it refuses with code `viewer` and sends nothing.
5. **Given** a run captured with an installation token, **When** a human runs `loupe publish` in a terminal with a user token, **Then** it refuses with code `viewer` and sends nothing.
6. **Given** an installation token, **When** `loupe publish` runs without `--unattended`, **Then** it refuses with code `token`, naming `--unattended`.
7. **Given** an installation token, **When** the pipeline runs `loupe capture`, **Then** capture succeeds without asking GitHub for the authenticated user and records an empty viewer.
8. **Given** no token at all, **When** `loupe publish <ref> --unattended` runs, **Then** it refuses with the existing `auth` code.

---

### User Story 3 - Recover an ambiguous send (Priority: P3)

The publish step's review request times out, so loupe cannot tell whether GitHub recorded the review. It keeps the attempt as unknown and exits with a refusal. A later step, or a later job holding the same data root, runs `loupe publish <ref> --unattended`. loupe lists the pull request's reviews and looks for the one carrying this attempt's digest and publication marker, authored by a `[bot]` login. If it is there, loupe writes a receipt instead of posting again. If it is not, loupe refuses and sends nothing until the pipeline asks for `--retry-unknown`.

**Why this priority**: Timeouts are rare, but posting twice is visible to everyone on the pull request. Spec 001's reconciliation already handles this for humans; it needs one change to work without a recorded viewer.

**Independent Test**: With a fake GitHub that records the review and then fails the response, run unattended publish with an explicit run reference, confirm an unknown attempt, then run it again and confirm a receipt with the review's id and the bot login and no second review.

**Acceptance Scenarios**:

1. **Given** an unknown unattended attempt whose review landed, **When** `loupe publish <ref> --unattended` runs, **Then** reconciliation finds the review by marker, commit and `[bot]` author, writes a receipt recording the review's author login, and sends nothing.
2. **Given** an unknown unattended attempt whose review did not land, **When** `loupe publish <ref> --unattended --retry-unknown` runs, **Then** every unattended gate runs again and exactly one review request is sent.
3. **Given** an unknown unattended attempt and a review on the pull request carrying the same marker but authored by a login that does not end in `[bot]`, **When** reconciliation runs, **Then** that review does not match, and publish refuses with `attempt`.
4. **Given** a successful unattended send, **When** the receipt is written, **Then** it records the author login GitHub returned for the created review.

---

### User Story 4 - This repository reviews its own pull requests (Priority: P4)

The maintainer adds `.github/workflows/review.yml` as a worked example. They trigger it by hand for a pull request they name. The job checks out `main`, builds loupe from it, captures the pull request, lays out a review directory holding the files at the captured head and the captured diff, and runs Claude Code through OpenRouter over that checkout, pointed at the review directory with a prompt that says to file findings through loupe and not to publish. A second job, the only one able to write to the pull request and with no agent in it, restores that data root and publishes unattended. The data root is uploaded as an artifact whatever happens. Once a run looks right, a follow-up change lets a label ask for a round, and the run takes that label off again.

**Why this priority**: It is the live check for everything the fake GitHub cannot prove: the token prefix, the `/user` 403 and the bot login. It also tests the integration contract from the outside, since the agent learns filing from `loupe --help` alone. It depends on stories 1 to 3.

**Independent Test**: Not automatable here. The maintainer dispatches the workflow on a pull request they name and checks that one COMMENT review by `github-actions[bot]` appears, marked unattended, and that the artifact holds the data root.

**Acceptance Scenarios**:

1. **Given** the workflow as first committed, **When** a pull request opens, **Then** nothing runs; only a manual dispatch naming a pull request number starts it.
2. **Given** the repository variable `REVIEW_ENABLED` is anything but `true`, **When** the workflow is triggered, **Then** the job is skipped.
3. **Given** a draft pull request, a pull request from a fork, a release-please pull request or a Dependabot pull request, **When** the workflow is triggered, **Then** the job is skipped.
4. **Given** a pull request that changes loupe's source, **When** the job runs, **Then** the loupe binary that holds the write token is built from `main`, not from the pull request's head.
5. **Given** the job's steps, **When** any of them runs loupe, **Then** it acts on the dispatched pull request's run, through the job's `LOUPE_HOME` and `LOUPE_RUN`, although the checkout is on `main`.
6. **Given** the review directory, **When** the agent reads it, **Then** it holds the files at the captured head and the captured diff, the prompt says the checkout around it is `main` and not the pull request, and no credential is readable on the runner.
7. **Given** the agent step, **When** it runs, **Then** its allowed commands are `loupe --help`, `--help` on any subcommand, `loupe add`, `loupe edit`, `loupe summary` and `loupe show`, and nothing else: no `capture`, `review`, `publish`, `wait`, `handoff`, `feedback`, `reply` or `list`, and no git or other shell command.
8. **Given** the reviewing job, **When** it runs, **Then** its token cannot write to the pull request, and the job that can write runs no agent.
9. **Given** any outcome of the publish job, success, refusal or failure, **When** it ends, **Then** the data root is uploaded as an artifact.

---

### Edge Cases

- **A batch with one bad location.** `loupe add` stores a batch entirely or not at all, so an adapter that files ten findings with one off the diff files none, and gets the nearest valid lines for the one at fault. The adapter corrects it or files it as `general`.
- **No run named.** A pipeline that neither passes a reference nor sets `LOUPE_RUN` gets `usage`, not a review on whatever pull request the checkout's branch points at.
- **The data root is not shared between steps.** Each step starts from an empty root and the publish step refuses `no-run`.
- **A retried publish.** A job that publishes again MUST start from the state its own last attempt left, not from the draft it was handed: without the attempt or receipt, reconciliation has nothing to match and the same review can post twice.
- **Restoring a data root.** Restoring keeps the run's attempts and receipts, so a receipt replays and an unknown attempt reconciles. Capturing fresh instead gets a new publication id, which cannot match an earlier attempt, so the review MAY post twice. An explicit `--retry-unknown` can also duplicate a send that becomes visible only later; that is the documented cost of asking for it.
- **A labelled pull request from a branch in this repository.** Under `pull_request`, GitHub runs the workflow file from the merge of that branch into the base, so a pull request can change the workflow that reviews it. Its token cannot write to the pull request, but a same-repository branch does get the workflow's secrets, which a fork does not: whoever can push a branch here can read the model API key by editing the job that runs. Only someone with push access can do that, so this is accepted rather than guarded; a repository that cannot make that assumption MUST put the key behind an environment with required reviewers, or keep the label trigger off.
- **Fork pull requests.** Under `pull_request`, a fork's job gets a read-only `GITHUB_TOKEN` and no secrets, so the review request is rejected with 403. publish MUST report it as a definite rejection, remove the attempt and exit 1. There is no workaround in this specification; `pull_request_target` is out of scope. The example workflow skips forks before this can happen.
- **Job canceled during the send.** publish already holds signals while sending. A cancellation that kills the process outright leaves an unknown attempt, recoverable only if the data root survives.
- **Head moved.** When the captured commit left the pull request's history, unattended publish MUST refuse `head-moved`. When the captured commit is still in the history, it MUST send the review at the captured commit, as attended publish does, with nothing to confirm.
- **Empty draft.** A draft with no summary and no publishable finding MUST still refuse `empty`. A pipeline that wants a review on a clean pull request sets a summary saying so.
- **Open notes.** Readiness counts open send-back notes. Unattended publish ignores them, since no human is there to answer; the token-kind match keeps a human's run, the only kind that gathers notes, out of this path.
- **App token in a terminal.** Someone who exports an installation token locally and runs `--unattended` gets the same unattended publication, posted as the App and marked. The gate is the token, not the environment.
- **`--plain` with `--unattended`.** `--plain` selects a terminal mode that unattended never shows, so the pair MUST exit 2 with a `usage` error.
- **Attended and unattended reviews on one pull request.** The two count rounds independently: attended from local run state, unattended from bot reviews on GitHub. A human's review and a bot's review MAY both read `round 1`. Their authors and the `unattended` segment tell them apart.
- **Other bots' loupe reviews.** A second App posting loupe reviews on the same pull request counts toward the round, because the rule counts any `[bot]` author. This is accepted.
- **Prompt injection in the example workflow.** Pull request content can steer what the agent files, and loupe's `--from` input flags read any path the runner can reach, outside the agent's file-tool scope, though loupe parses each file as that command's input and never publishes raw file contents. The workflow skips forks, so only people with push access author the content the agent reads. Accepted for an example running on this repository; a pipeline whose reviewer reads untrusted content owns that risk.

## Requirements *(mandatory)*

### Functional Requirements

#### The integration contract

- **FR-001**: `loupe publish --unattended` MUST take its run from the positional reference or `LOUPE_RUN`, and MUST NOT fall back to the pull request of the current branch. With neither, it MUST exit 2 with a `usage` error naming both ways.
- **FR-002**: Capture MUST remain the only step that needs the Git clone. Its `--json` result already names the run reference, and that reference MUST be what later steps pass.
- **FR-003**: `loupe publish --unattended` MUST work from a data root alone, with no clone and no working tree, and MUST behave identically when that root is restored at a different absolute path.
- **FR-004**: Findings and the summary MUST enter through the existing `loupe add` input (one object or an array, stored entirely or not at all) and `loupe summary`. This feature MUST NOT add a second input format for pipelines.
- **FR-005**: A finding whose location is not in the captured diff MUST be refused with `location` and the nearest valid lines, on this path as on any other. loupe MUST NOT relocate or silently drop a finding.
- **FR-006**: On a restored data root, publish MUST replay a receipt, and MUST reconcile an unknown attempt before anything else: a match writes a receipt and sends nothing; no match refuses `attempt` and sends nothing until `--retry-unknown` is asked for, which passes every gate again.

#### Unattended publication

- **FR-007**: `loupe publish` MUST accept `--unattended`, alongside the existing `--inline`, `--retry-unknown` and `--json`.
- **FR-008**: loupe MUST report whether the resolved GitHub token is an App installation token, identified by the prefix `ghs_`, from the same token resolution publish and capture already use. Every other token, including `gho_`, `ghp_`, `github_pat_` and `ghu_`, is a user token.
- **FR-009**: `loupe publish --unattended` MUST refuse with a new code, `token`, when the token is a user token. The fix MUST name `GITHUB_TOKEN` and `permissions: pull-requests: write`. The refusal MUST come before any review request.
- **FR-010**: `loupe publish` without `--unattended` MUST refuse with code `token` when the token is an installation token, with a fix naming `--unattended`. That refusal MUST come before the terminal refusal, so a pipeline is told to add the flag rather than to find a terminal it has not got. Where no client can be built at all, the terminal refusal still answers first.
- **FR-011**: With `--unattended`, `--action` MAY be omitted and defaults to `comment`. Any other `--action` value, or `--plain`, MUST be a `usage` error with exit 2. The publication package MUST refuse the same combination before anything is sent, so the COMMENT-only rule does not rest on one caller.
- **FR-012**: Unattended publish MUST NOT require or read a terminal, MUST NOT read stdin, and MUST NOT show a confirmation.
- **FR-013**: Unattended publish MUST compose the review from the publishable set, accepted and pending findings, and MUST NOT refuse for pending findings or open notes, either as a gate or while composing. Excluded and withdrawn findings MUST stay out. The Markdown allowlist check at composition MUST still run.
- **FR-014**: Unattended publish MUST keep the receipt replay, unknown-attempt reconciliation, `head-moved` for a captured commit that left the pull request's history, and `empty` refusals. It MUST skip `tty`, `not-ready`, `own-pr`, the confirmation and the viewer recheck after confirmation.
- **FR-015**: Unattended publish MUST number its review as 1 plus the number of the pull request's reviews whose author login ends in `[bot]` and whose body carries a `loupe-meta` marker. It MUST read the pull request's reviews once before sending to derive this; a failure to read them MUST refuse with nothing sent. The run's own round, references and records MUST keep the capture round.
- **FR-016**: An unattended review's footer MUST insert ` · unattended` directly after `round N`, and its `loupe-meta` MUST carry `unattended=1` directly after `round=`. A review published without `--unattended` MUST be unchanged byte for byte.
- **FR-017**: Reconciliation of an attempt with no recorded viewer MUST match a non-pending review at the attempt's commit whose body holds the attempt's marker line and whose author login ends in `[bot]`.
- **FR-018**: A receipt from an unattended publication MUST record the author login GitHub returned, from the created review or the matched one.
- **FR-019**: Capture with an installation token MUST NOT ask GitHub for the authenticated user and MUST record an empty viewer. Capture with a user token MUST behave as today.
- **FR-020**: Publish MUST refuse with code `viewer` when the token's kind differs from the kind capture recorded: an empty viewer published with a user token, or a recorded viewer published with an installation token.
- **FR-021**: Unattended publish MUST send exactly one review request per attempt, record the attempt before sending, hold signals while sending, and handle definite rejections and ambiguous outcomes as spec 001 FR-029 to FR-032 require.

#### Documentation and contracts

- **FR-022**: `loupe publish --help` MUST document `--unattended`, its token requirement, its run-selection rule, and that it posts only as comment. The top-level help MUST still say that agents MUST NOT run review or publish in a human's session.
- **FR-023**: The plugin skill MUST keep forbidding the agent from publishing, with no mention of `--unattended` as an agent option.
- **FR-024**: The documentation MUST carry an integration guide for pipeline authors: the three steps (capture, file, publish), how the run reference passes between them, the data root, the token and permissions an unattended publication needs, and what an adapter must emit for `loupe add`. It MUST say loupe ships no adapters and names no particular review action.
- **FR-025**: The contracts MUST be updated to match: `specs/001-loupe-v1/contracts/cli.md` gains `token` in the error codes table, widens `viewer` to cover a capture and publish token-kind mismatch, and documents `--unattended` and its run-selection rule in the publish section; `docs/comment-format.md` documents the segment under Footer and the key under Markers; the publish help goldens are regenerated.
- **FR-026**: `docs/github-facts.md` MUST record, once a live run has shown them, the prefix of Actions' `GITHUB_TOKEN`, the 403 on `GET /user` for an installation token, the author login `github-actions[bot]`, and the read-only token a fork pull request gets under `pull_request`.
- **FR-027**: `specs/001-loupe-v1/validation.md` MUST list as Unverified, until the first live dispatch on a named pull request: the token prefix, the `/user` 403, the bot author, and a job cancellation during the send.

#### The example workflow

- **FR-028**: `.github/workflows/review.yml` MUST first be committed with a `workflow_dispatch` trigger taking a pull request number and no other trigger. Any further trigger MUST be a separate change, made only after the maintainer has judged a live run on a pull request they named. The first such change adds `pull_request` for type `labeled`, acting only on the `ai-review` label: a round is asked for by labelling. The workflow MUST take that label off once a round has answered the request, whether it succeeded or failed, and MUST leave it on when no round ran, so that putting it back is how the next round is asked for; only `labeled` triggers the workflow, so removing it starts nothing. Rounds for one pull request MUST NOT run concurrently, since two publishing at once would post two reviews. A trigger that reviews a pull request nobody asked about, such as `opened`, remains a later decision.
- **FR-029**: The workflow's job MUST run only when the repository variable `REVIEW_ENABLED` is `true`, and MUST skip draft pull requests, pull requests from forks, release-please pull requests and Dependabot pull requests.
- **FR-030**: Every job that runs loupe MUST build it from `main`, never from the pull request's head, into a path outside the checkout. The reviewing job MUST declare exactly `contents: read`, `pull-requests: read` and `checks: read`; only the publishing job MAY declare `pull-requests: write`, and no agent MAY run in it. The reviewing action writes the token it is given into the workspace it hands to the agent, so a write token in that job is a write token the agent can read.
- **FR-031**: Each job MUST set `LOUPE_HOME` for its steps, and the publishing job MUST name the run through `LOUPE_RUN`. Capture MUST happen in a workflow step of the reviewing job, not in the agent's step.
- **FR-032**: A workflow step MUST lay out a review directory holding the repository at `target.headSha`, with symlinks removed, and a copy of the run's captured diff. The reviewing action expects the workspace to be a checkout of the base, so that directory sits inside it and the prompt MUST say which is which: the checkout around the agent is the base branch and is not under review. The credentials in that workspace MUST NOT be able to write to the pull request: the action rewrites the remote with the token it is given before the agent starts, so the reviewing job holds a read-only one (FR-030). No file-tool confinement is claimed: a harness that grants a directory grants it in addition to what it already reads, so the scope rests on the prompt and on there being no secret to find. What a given action's file tools actually refuse goes on the Unverified list.
- **FR-033**: The agent step MUST run Claude Code through OpenRouter using the `OPENROUTER_API_KEY` secret, with the model taken from the repository variable `REVIEW_MODEL`. The workflow's own variables MUST NOT take loupe's `LOUPE_` prefix, which belongs to the variables the binary reads. Its allowed commands MUST be exactly `loupe --help`, `--help` on any subcommand, `loupe add`, `loupe edit`, `loupe summary` and `loupe show`. Its prompt MUST point it at `loupe --help`, tell it how to file findings in a form its harness allows, set a summary even with no findings, and not publish. It MUST also carry what the reviewer cannot otherwise know or check: where the pull request's files are and that nothing outside them is readable; what this repository's own checks already report for the captured commit, so a finding contradicting them is not filed as blocking; and that a blocking finding MUST rest on a path traced through code the reviewer read, since it can run nothing. The step MUST cap the agent's turns.
- **FR-034**: Publishing MUST be its own job, running `loupe publish --unattended --json` against the data root the reviewing job uploaded, restored rather than re-captured so a receipt replays and an unknown attempt reconciles. The data root MUST be uploaded as an artifact from both jobs, on success and failure alike, and the publishing job's upload MUST replace its own earlier one. A re-run of that job MUST restore the state it last published from, in preference to the reviewing job's snapshot, and MUST refuse to publish when that state is missing: starting again from the pre-publication draft would mint a new publication id and could post a review GitHub already has.

### Key Entities

- **Token kind**: installation or user, derived from the resolved token's prefix. Capture records it implicitly, as an empty or present viewer; publish compares against it.
- **Unattended publication**: a publication made with `--unattended`. Its envelope has no viewer; its body carries the unattended footer segment and marker key; its receipt records the bot login GitHub returned.
- **Bot loupe review**: a review on the pull request whose author login ends in `[bot]` and whose body carries `loupe-meta`. The count of these sets an unattended review's round.
- **Data root**: the directory holding the runs. It is the unit a pipeline shares between steps or carries between jobs, and it is what makes an attempt recoverable. Its absolute path is not part of its meaning.
- **Adapter**: whatever the caller writes to turn their reviewer's output into `loupe add` input. Outside loupe.
- **Example workflow**: `.github/workflows/review.yml`, this repository's own integration. It checks out the base, builds, captures, lays out the review directory, runs an agent, publishes unattended and uploads the data root.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A pipeline whose reviewer can emit the documented finding JSON publishes through three loupe steps, with no change to loupe and no agent-host-specific code.
- **SC-002**: A caller reading the integration guide can name, without reading loupe's source, what token the publish step needs, how the run reference reaches it, and what their adapter must emit.
- **SC-003**: Each of the four user token prefixes is refused on the unattended path with zero review requests sent, and an installation token is refused on the attended path.
- **SC-004**: Every unattended review is identifiable from its body alone, by the footer segment and the marker key, and from its author.
- **SC-005**: An attended review of the same draft is byte for byte what it was before this feature.
- **SC-006**: On a pull request with K earlier bot loupe reviews, a fresh pipeline's review names round K+1.
- **SC-007**: A data root restored in another job at a different absolute path publishes the review the first job would have. A restored unknown attempt whose review landed becomes a receipt with nothing sent; one with no matching review refuses with nothing sent.
- **SC-008**: An ambiguous send reconciled from the same data root leaves exactly one review on the pull request.
- **SC-009**: The first manual dispatch of the example workflow on a pull request the maintainer names posts one review by the App, marked unattended, and leaves the data root in the job's artifacts.
- **SC-010**: All automated repository checks pass after the final edit.

## Assumptions

- The `ghs_` prefix identifies an App installation token, and Actions' `GITHUB_TOKEN` is one. `ghu_` is an App's user-to-server token, which acts as a person and posts under their name, so it is refused. Both are unverified until a live run.
- An installation token gets 403 on `GET /user`, which is why capture and publish avoid it. Unverified until a live run.
- Token resolution is unchanged: `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh`'s stored token.
- A job starts from an empty data root unless the caller restores one, so local round state cannot number a pipeline's reviews.
- `LOUPE_RUN` without a round selects that pull request's newest round, which is round 1 in a fresh data root.
- Open notes only arise from a human's send-back in the review interface, which a pipeline-captured run never has; ignoring them on the unattended path loses nothing.
- Counting any `[bot]` author, not only the App publishing, is simpler than recording App identity, and the rare collision is accepted.
- Laying out the review directory is the caller's work, not loupe's, so Principle IV's ban on loupe creating a checkout is untouched.
- Adapters for particular review actions, and any mode that lets another action hand loupe its findings directly, are outside this specification.
- The exact inputs of `anthropics/claude-code-action` and OpenRouter's Claude Code settings are confirmed against their documentation when the workflow is built.
- Trying non-Claude models or other agent harnesses in the example workflow is later work.
- No push, pull request, release, live review or workflow dispatch is authorized by this specification.
