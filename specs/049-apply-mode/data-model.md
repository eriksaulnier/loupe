# Data model: Apply mode

Every record follows `docs/versioning.md`. Each change below is additive, so no migration step is needed.

## Local run directory

```text
<data root>/local/
├── .lock                  # serializes the next counter
├── next                   # the next n, a decimal integer; never decreases
└── <n>/
    ├── target.json        # target schema 3, kind "local"
    ├── pr.diff            # the captured diff (empty for --no-files)
    ├── outline.json       # Markdown headings of captured files (schema 1, unchanged)
    ├── draft.json         # draft schema 5
    ├── handback.json      # send-back notes (schema 1, unchanged)
    └── return.json        # written once by the return action; seals the run
```

PR runs keep the `runs/<owner>/<repo>/<n>/<round>/` layout unchanged.

## target.json, schema 3

**PR target**: today's schema-2 fields, unchanged. `schema` is 3, and `kind`, `applier` and `local` are absent. A missing `kind` reads as `pr`.

**Local target**: written through `LocalTargetFile`. It holds exactly these fields:

| Field | Type | Rule |
| :--- | :--- | :--- |
| `schema` | int | 3 |
| `kind` | string | `"local"` |
| `title` | string, optional | `--title`: one line, at most 120 characters, no control character, the Markdown allowlist under a new kind `markdown.RunTitle` |
| `applier` | string, optional | `--applier`: one line, at most 64 characters, the same rules under a new kind `markdown.Applier` |
| `source`, `model` | string, optional | As on PR runs, with their existing validators. Provenance only |
| `capturedAt` | time | |
| `diffSha256` | string | The SHA-256 of `pr.diff`, as on PR runs |
| `local.n` | int | The run number |
| `local.prefix` | string | `owner/repo`, a sanitized directory name, or `""` outside Git. Display only |
| `local.repository` | string, optional | `owner/repo` when `origin` names a GitHub repository |
| `local.capture` | string | `changed`, `files` or `no-files` |
| `local.root` | string | The absolute worktree root, or the working directory outside Git |
| `local.branch` | string, optional | Empty on a detached `HEAD` |
| `local.baseRef` | string, optional | `--base` as given |
| `local.mergeBase` | string, optional | The commit the diff is against: the merge base, or `HEAD` with no `--base` |
| `local.files` | array | `{path, abs, new?}`. `path` is the display path used in the diff and in anchors (research R3). `abs` is the absolute path. `new` (`--changed` only) is true when `HEAD` does not hold the file |
| `local.skipped` | array | `{path, reason}`, where reason is `binary`, `empty`, `deleted`, `submodule` or `too-large` |

`LoadTarget` accepts schemas 1 to 3. A target with `kind: local` at schema 2 or below refuses with `record`.

## draft.json, schema 5

Schema 5 applies to every run.

| Field | Change |
| :--- | :--- |
| `findings[].proposal` | New, optional, `omitempty`. Markdown, at most 64 KiB, the allowlist and the finding-id rule (new kind `markdown.Proposal`). It is accepted on local runs only, and a change to it bumps `rev` and clears the decision |
| `decisions[].note` | New, optional, `omitempty`, human-only. On `accepted`, it holds the accept note under the send-back note rules, with a new kind `markdown.AcceptNote`. On `excluded`, it holds the exclude reason under `draft.ValidateExcludeReason`: one line, at most 200 characters, with a new kind `markdown.ExcludeReason`. It clears with its decision |

JSON input that carries `decisions` stays refused. Stored findings on local runs keep `blocking: false` and `side: "RIGHT"`, but `show`, `feedback` and `wait` omit both from local-run output.

## return.json, schema 1 (new record)

| Field | Type |
| :--- | :--- |
| `schema` | 1 |
| `returnedAt` | time |
| `draftVersion` | int, the draft version that the return sealed |

`draft.Seal` writes the record under the draft lock. After it exists, `draft.Mutate` refuses every change with `returned`. `RecordHandBack` stays allowed.

## Derived states

Local run states:

| State | Condition |
| :--- | :--- |
| `captured` | No `return.json`, and the run is not `ready` |
| `ready` | No `return.json`, `ReadinessOf` is ready (no pending finding, no open note), and the draft holds at least one finding |
| `returned` | `return.json` exists |

Feedback groups. Each finding's place comes from `draft.GateOf`, and withdrawn findings count as decided:

| Group | Finding |
| :--- | :--- |
| `apply` | Accepted, with no decision note, and `HumanEdited` is false |
| `applyWithChanges` | Accepted, and either it has a decision note (`suggestion`) or `HumanEdited` is true |
| `excluded` | Excluded by the human, or withdrawn by the human. `reason` is present when the human gave one |
| `withdrawn` | Withdrawn by the agent, with `withdrawReason` as today |

A question is a finding with no `proposal`.
