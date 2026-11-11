# Implementation Plan: Approve a pull request whose head moved after capture

**Branch**: `056-approve-moved-head` | **Date**: 2026-11-10 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/056-approve-moved-head/spec.md`

## Summary

`checkHead` in `internal/publish/gates.go` stops refusing approve at a moved head. An attended approve then takes the path that comment and request-changes already take: compare the captured commit with the live head, then build a `HeadMoved` for the confirmation, and send at the captured commit. `github.ComparedFile` keeps the per-file additions and deletions that GitHub's compare response already carries, and `HeadMoved` sums them. `headMovedLines` in `internal/tui/confirm.go` takes the action. For every action it prints one size line. For approve it adds two lines: what the approval covers, and that branch protection may not count it. The contract, the 001 spec and `validation.md` change to match.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: None added.

**Storage**: None. `HeadMoved` lives only during publish and is never written to disk, so no schema changes.

**Testing**: `internal/publish` gate tests against `fakegh`, `internal/tui` confirmation view tests on a fixture preview, and one `internal/integration` test end to end. `fakegh` serves `additions` and `deletions` from compare.

**Target Platform**: Linux and macOS, as today.

**Project Type**: CLI.

**Constraints**:

- No new GitHub request (FR-009). The attended path already compares when the head moved (`gates.go:202`).
- The confirmation for an unmoved head stays byte for byte the same (SC-005).
- Commit text stays escaped through `render.ForDisplay` (FR-010).

**Scale/Scope**: About 40 lines of production code across four files, the test changes, and edits to the contract, the 001 spec and `validation.md`.

## Constitution Check

The check is against constitution 4.0.0.

- **II**: The approval still goes out in one interactive command that sends one request, after the human reads a confirmation that names what the approval covers. The human still accepts each finding individually, and still writes the opening prose. An unattended run still sends only COMMENT.
- **III**: No new network access. The size comes from the compare call that the attended confirmation already makes.
- **V**: No `--json` field, refusal code or exit code changes. The `head-moved` refusal keeps its code and its other causes, and only its help text changes.
- **VI**: No new type, flag or dependency. `HeadMoved` gains three ints, and `ComparedFile` gains two.
- **VII**: Tests use `fakegh` and the fixture preview. A real APPROVE at an older commit is unverified (FR-011).

## Research

### GitHub's compare response

`GET /repos/{owner}/{repo}/compare/{base}...{head}` returns `files[]` with `filename`, `previous_filename`, `additions`, `deletions` and `changes` per file. loupe reads it today and keeps only the two names (`internal/github/client.go:562`). The file list stops at 300 (`compareFileCap`), and loupe already records that as `FilesTruncated`.

### Decisions

- **Where the refusal goes.** Decision: delete the `if action == "approve"` block at `gates.go:193`. Then `checkHead` no longer needs `action`, so drop that parameter. Rationale: approve becomes the same case as the other attended actions. Alternative: keep the block for the unattended path. Rejected, because the unattended action is already forced to comment in `publish.Run` (`publish.go:98`) before the gates run.
- **Size totals.** Decision: `HeadMoved` gains `Files`, `Additions` and `Deletions`. `Files` is `len(cmp.Files)`, and the other two are sums over `cmp.Files`. Rationale: the confirmation needs the totals and not the per-file rows. The unattended path builds `HeadMoved` from the commit list with no files, and no one sees it there, so its zeros do no harm.
- **Size line text.** Decision: `Since capture: 3 files changed, +42 -7.` One file reads `1 file changed`. With `FilesTruncated` set: `Since capture: at least 300 files changed, +X -Y (GitHub lists at most 300 changed files, so these counts can be low).` The line goes right after the `Head moved …` line, so that the size appears before the explanation of pinning.
- **Approve lines.** Decision: after `A comment on a line they changed shows beside the new code.`, an approve adds:
  - `Approving also covers these N commits, which this review did not read.`
  - `A branch that dismisses stale approvals or requires approval of the latest push may not count an approval pinned to 1111111.`

  Rationale: the first line is FR-006, and the second is FR-007. Both come after the pinning lines that they depend on.
- **Passing the action.** Decision: `headMovedLines(moved, action string)`. Both callers already hold `c.title.action` (`confirm.go:397`). Alternative: put the action in `publish.Preview`. Rejected, because the confirmation title already carries it.
- **Header tag.** Decision: no change. `head moved +N` already shows in the summary line for every action.

## Files

| File | Change |
| :--- | :--- |
| `internal/github/client.go` | `ComparedFile` gains `Additions`, `Deletions`. `wireComparison` decodes `additions` and `deletions`. `Compare` copies them |
| `internal/github/client_test.go` | `TestCompare` asserts the two fields |
| `internal/testutil/fakegh/fakegh.go` | `getComparison` writes `additions` and `deletions` |
| `internal/testutil/fakegh/fakegh_test.go` | `TestCompare` asserts them |
| `internal/publish/gates.go` | Delete the approve refusal, drop `action` from `checkHead`, and give `HeadMoved` the three totals filled in `headMoved` |
| `internal/publish/publish_test.go` | `TestRunRefusesApproveAtMovedHead` becomes `TestRunPublishesApproveAtMovedHead`, which expects one APPROVE at the captured `commit_id`, a preview with `HeadMoved` set, and a compare call. `TestRunRefusesApproveWhenBaseTookCapturedCommitIn` becomes a publish test the same way. One new test pins the totals and `FilesTruncated` |
| `internal/tui/confirm.go` | `headMovedLines` takes the action, then adds the size line and the two approve lines |
| `internal/tui/preview_test.go`, `confirm_test.go` | The `movedPreview` fixture gains sizes. `movedHeadLines` gains the size line. A new approve view test pins the two lines, and the comment test asserts that they are absent |
| `internal/integration/*_test.go` | One test: a forward push, then publish with approve, sends one APPROVE with `commit_id` = the captured head |
| `internal/cli/publish.go` | The `head-moved` help line no longer says "or approve at a moved head" |
| `testdata/golden/cli/` | Regenerate `publish-help` with `-update`, then read the diff |
| `cmd/loupe-demo/seed.go` | Give the #44 comparison per-file sizes so the demo shows the size line |
| `specs/001-loupe-v1/contracts/cli.md` | Remove the approve case from the `head-moved` row and its "for approve" fix. The publish section says that an approve at a moved head shows the head-moved block |
| `specs/001-loupe-v1/spec.md` | Add an amendment note to FR-025 and FR-043, as earlier specs did |
| `specs/001-loupe-v1/validation.md` | Row 25 replaces `TestRunRefusesApproveAtMovedHead` with the new test names and the approve case. The Unverified list adds GitHub's handling of an APPROVE at an older `commit_id`, under no protection, under "dismiss stale approvals" and under "require approval of the most recent reviewable push" |

## Verification

- `go test ./internal/publish/ ./internal/tui/ ./internal/github/ ./internal/testutil/... ./internal/integration/` after each task.
- `go test ./internal/cli/ -update`, then `git diff testdata/golden`. Only `publish-help` changes.
- `mise run check`, plus patch coverage per `CONTRIBUTING.md`.
- By eye: `mise run demo -- publish 'acme/widgets#44' --action approve`, captured through tmux. The confirmation shows the size line and both approve lines, with no refusal.
- Unverified: a live APPROVE at an older commit on GitHub (FR-011).

## Complexity Tracking

No constitution violations. The departure from `contracts/cli.md` is the amendment that FR-011 names.
