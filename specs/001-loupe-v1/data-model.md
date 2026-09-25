# Data Model: loupe v1

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Source of decisions**: [research.md](research.md) "Run identity and storage" and "Draft model"

Every record is a JSON file in one run directory. Field names are camelCase in JSON. Times are RFC 3339 UTC. Nothing derived is stored.

## Run directory

`<data root>/runs/<owner>/<repo>/<number>/<round>/`

| File | Written by | Present when |
| :--- | :--- | :--- |
| `target.json` | capture, once | the run exists |
| `pr.diff` | capture, once | the run exists |
| `draft.json` | capture (empty), every mutation | the run exists |
| `.lock` | any mutating command, publish | first mutation; content is `<pid> <command>` of the current holder |
| `handback.json` | review, at each send-back and on a clean exit | the human sent a finding back at least once |
| `.review` | review, for as long as it runs | a review was ever opened; empty, and meaningful only through the shared flock each running review holds on it (`specs/017-live-review`) |
| `attempt.json` | publish | a send is in flight or its outcome is unknown |
| `receipt.json` | publish | the review landed |

Run reference: `owner/repo#123` names the newest round; `owner/repo#123@2` names round 2.

Derived run state: `published` if `receipt.json` exists, else `ready` if readiness holds, else `captured`.

## Hand-back set

`handback.json` is `{"schema": 1, "notes": ["n-001", "n-003"]}`: the append-only set of note ids the human handed back. A send-back adds its note under the run lock before the draft is written, so a reader never sees the note without it; a clean exit of `review` then adds every note that is open, has no reply and is not yet listed. Both write only when something was added, so the draft version never moves for the set. Amended on 2026-09-21 by `specs/017-live-review`; before it, only the clean exit recorded. A downgraded binary never reads it.

Derived: a note is awaiting the agent while it is in the set, open and without a reply. `wait` returns on any awaiting note or on a receipt; a reply to one of two handed-back notes leaves the other awaiting.

The exit's recording runs only on a clean exit: `q`, Ctrl-C in the full-screen program, and `q` or end of input in plain mode. A send-back is recorded whatever happens to the session afterwards.

## Target (immutable)

| Field | Type | Notes |
| :--- | :--- | :--- |
| `schema` | int | 1 |
| `owner`, `repo` | string | from the pull request URL |
| `number` | int | pull request number |
| `url` | string | canonical pull request URL |
| `title` | string | pull request title at capture, shown by `list` |
| `author` | string | pull request author login |
| `viewer` | string | authenticated login at capture |
| `baseSha`, `headSha` | string | from the API, verified against the fetched refs |
| `round` | int | 1-based, one higher than any existing round for the pull request; the round a published review names counts only published rounds, so it MAY be lower (FR-042) |
| `previousRound` | int, optional | `round - 1` when any earlier round exists, published or not; lineage only |
| `capturedAt` | time | |
| `clonePath` | string | absolute path of the clone used |
| `baseRef`, `headRef` | string | `refs/loupe/<owner>/<repo>/<number>/<round>/{base,head}` |
| `mergeBaseSha` | string | `git merge-base baseRef headRef`, the commit `pr.diff` compares the head against |
| `diffSha256` | string | hex SHA-256 of `pr.diff`; every load of `pr.diff` compares it and refuses `record` on a mismatch (ruled 2026-09-13) |
| `source` | string, optional | `--source` at capture, `name[@version]`, shown in the published footer and `loupe-meta`; omitted when none, so runs captured before it load unchanged |
| `model` | string, optional | `--model` at capture, the reviewer's model id as the caller names it, shown in the published footer and `loupe-meta`; omitted when none (specs 008 and 023) |

Validation: capture refuses (`same-head`) when the newest existing round has the same `headSha` and no receipt. `previousRound` is lineage; the previous published findings are found by walking rounds downward from `round - 1` to the first with a receipt. Capture refuses (`input`) a `source` outside the `src=` rule in `docs/comment-format.md` or a `model` outside the `model=` rule there, and loading a `target.json` that carries either refuses `record`.

## Draft

```text
draft:    { schema: 1, version, summary, findings[], decisions{}, notes[], replies[] }
```

| Field | Type | Notes |
| :--- | :--- | :--- |
| `schema` | int | 1 |
| `version` | int | starts at 0 on capture; every mutation increments by one; `--expect-version` compares against it inside the lock |
| `summary` | string | Markdown, allowlist-checked, may be empty |
| `findings` | Finding[] | append-only; ids never reused |
| `decisions` | map findingId → Decision | at most one per finding |
| `notes` | Note[] | |
| `replies` | Reply[] | |

## Finding

| Field | Type | Publishable | Notes |
| :--- | :--- | :--- | :--- |
| `id` | string | | `f-001`, sequential within the run, never reused |
| `rev` | int | | starts at 1; increments on any change to a publishable field or to `included` |
| `title` | string | yes | required, non-empty, collapsed to one line at render |
| `body` | string | yes | required, Markdown, allowlist-checked, at most 64 KiB |
| `location` | Location, optional | yes | exactly one of `location` or `general: true` |
| `general` | bool | yes | |
| `label` | string, optional | yes | `issue`, `suggestion`, `question` or any other word of letters, digits, `_`, `.` or `-` (at most 40 characters) kept verbatim; empty means none |
| `blocking` | bool | yes | default false |
| `confidence` | string, optional | yes | `high`, `medium` or `low` when present |
| `severity` | string, optional | yes | `critical`, `major`, `minor` or `trivial` when set or changed (spec 008); a stored value outside that set still loads and renders |
| `verified` | string, optional | yes | `reproduced` or `plausible` when present (spec 008) |
| `impact` | string, optional | yes | Markdown, allowlist-checked like `body` (spec 008) |
| `references` | string[], optional | yes | at most six `http` or `https` URLs, each at most 200 bytes, no whitespace, control or format characters, `<`, `>` or backticks; empty stored as absent (spec 008) |
| `suggestedFix` | string, optional | yes | Markdown, allowlist-checked like `body`, code fenced by the writer; never a GitHub suggestion fence (spec 020) |
| `by` | string | | `agent` or `human`; audit only, does not affect readiness |
| `included` | bool | | true on add; `edit --exclude` sets false (withdraw), `edit --include` sets true (restore); never settable from JSON input |
| `createdAt`, `updatedAt` | time | | |
| `history` | History[] | | one entry per edit: `{ at, by, changed: { field: previousValue } }` |

## Location

| Field | Type | Notes |
| :--- | :--- | :--- |
| `path` | string | must be a file in `pr.diff` |
| `side` | `RIGHT` or `LEFT` | default `RIGHT`; `RIGHT` is the new file, `LEFT` the old |
| `line` | int | must appear on that side in some hunk of that path |
| `startLine` | int, optional | when present, `startLine <= line` and both in the same hunk |

Validation is against the stored diff only. A refusal names up to three valid lines below and three above on that side of that path.

## Decision

| Field | Type | Notes |
| :--- | :--- | :--- |
| `findingId` | string | map key |
| `decision` | `accepted` or `excluded` | |
| `findingRev` | int | the finding's `rev` when decided; the decision is current iff equal to the finding's current `rev` |
| `at` | time | |

Only the review interface writes decisions (`by` is implicitly human). A send-back deletes the finding's decision. A label or blocking edit made in the review interface re-records a current decision at the finding's new `rev` (specs/005-edit-in-review). Reinstating a withdrawn finding there sets `included`, bumps `rev` and records an acceptance at the new `rev` (specs/011-reinstate-withdrawn); like that edit it is out of the command line's reach, because `--by` is self-reported. A stale decision (rev mismatch) is ignored by derivation and overwritten by the next decision.

## Note

| Field | Type | Notes |
| :--- | :--- | :--- |
| `id` | string | `n-001`, sequential |
| `findingId` | string | one finding |
| `body` | string | the human's send-back message |
| `at` | time | |
| `status` | `open`, `resolved`, `dismissed` | only the review interface changes it |
| `closedAt` | time, optional | |

## Reply

| Field | Type | Notes |
| :--- | :--- | :--- |
| `id` | string | `r-001`, sequential |
| `noteId` | string | |
| `body` | string | Markdown |
| `by` | string | `agent` or `human` |
| `at` | time | |

A reply never changes note status or any decision.

## Derived values (never stored)

| Value | Rule |
| :--- | :--- |
| Disposition `accepted` | `included` and a current `accepted` decision |
| Disposition `excluded` | a current `excluded` decision |
| Disposition `withdrawn` | not `included` and no current decision |
| Disposition `pending` | `included` and no current decision |
| Readiness | no finding is `pending` and no note is `open` |
| Counts | accepted, pending, excluded, withdrawn, openNotes |
| Publishable set | findings that are `included` and not human-excluded: disposition `accepted` or `pending` |
| Publishable digest | hex SHA-256 over the summary and, sorted by id, each finding in the publishable set reduced to its id and publishable fields, in a canonical JSON encoding. Decisions do not affect it except exclusion, which removes a finding from the set. At an attended publish, readiness means no finding is pending, so the publishable set equals the published set; under `--unattended` they are equal by definition |
| Published set | the findings publish sends: disposition `accepted` when attended, the whole publishable set under `--unattended` (`specs/007-unattended-publish` FR-013). An `excluded` finding is still `included: true` but is in neither the publishable nor the published set. Corrected on 2026-09-21 by `specs/015-publishable-set`: it read `accepted` alone, which spec 007 made false |

## Attempt

| Field | Type | Notes |
| :--- | :--- | :--- |
| `schema` | int | 1 |
| `state` | `in-flight` or `unknown` | |
| `startedAt`, `updatedAt` | time | |
| `envelope` | Envelope | the exact request that was or may have been sent |
| `confirmed` | `{ version, digest, dispositions: { id: disposition } }` | what the human saw when they pressed `y` |
| `lastError` | string, optional | transport or 5xx detail for the human |

Lifecycle: written `in-flight` under the draft lock before the request; deleted on 2xx (receipt written) or definite 4xx; set to `unknown` on anything else. An unknown attempt is resolved only by reconciliation or by `--retry-unknown`, which passes every gate again and overwrites the attempt with a new `in-flight` one.

## Receipt

| Field | Type | Notes |
| :--- | :--- | :--- |
| `schema` | int | 1 |
| `reviewId` | int | GitHub review id |
| `reviewUrl` | string | `html_url` |
| `action` | `comment`, `approve`, `request-changes` | |
| `postedAt` | time | |
| `envelope` | Envelope | what was sent; source of `show --previous` for the next round |

A receipt is never deleted by loupe. Its existence makes every later `publish` a no-op that prints `reviewUrl`.

## Envelope (publication payload)

| Field | Type | Notes |
| :--- | :--- | :--- |
| `target` | `{ owner, repo, number, headSha, round }` | |
| `viewer` | string | |
| `action` | string | |
| `event` | `COMMENT`, `APPROVE`, `REQUEST_CHANGES` | derived from `action`; always sent |
| `commitId` | string | equals `target.headSha` |
| `draftVersion` | int | |
| `digest` | string | publishable digest; also inside the hidden marker |
| `publicationId` | string | UUID v4, generated per attempt; inside the hidden marker |
| `inline` | `none`, `blocking`, `all` | |
| `body` | string | composed per `docs/comment-format.md`; at most 65,536 characters, as is each inline comment body |
| `comments` | `[{ path, line, side, startLine?, startSide?, body }]` | located findings selected by `inline`; body is the finding rendered without its `<details>` wrapper |
| `findings` | `[{ id, title, body, location, label, blocking }]` | the published findings, kept for `show --previous` |

## State transitions

```text
Run:      (none) --capture--> captured --publishable set all accepted, no open note--> ready --receipt--> published
Finding:  pending --accept--> accepted --edit/withdraw/send-back--> pending
          pending --exclude--> excluded --edit--> pending ; excluded --restore(u)--> pending
          included --edit --exclude--> withdrawn --edit --include--> pending ; withdrawn --reinstate(u)--> accepted
Note:     open --resolve|dismiss (review only)--> resolved|dismissed
          open --accept its finding--> resolved ; open --exclude its finding--> dismissed
Attempt:  (none) --confirm--> in-flight --2xx--> (deleted, receipt) | --4xx--> (deleted) | --other--> unknown
          unknown --reconcile match--> (deleted, receipt) | --retry-unknown + confirm--> in-flight
```

## Invariants

- Finding and note ids are sequential and never reused within a run; a withdrawn finding keeps its id.
- Every mutation runs under the run's exclusive lock, validates fully, then bumps `version` and writes atomically; a batch with one invalid entry writes nothing.
- JSON input never sets `included`, a decision, or a note status; unknown fields are refused.
- Decisions, notes and replies never appear in the composed body or comments.
- The stored diff is the only authority for locations; findings are never remapped after the head moves.
- A run directory, a receipt, or a lock file is never deleted by loupe; the only automatic deletion is a resolved attempt.
