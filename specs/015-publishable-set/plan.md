# Implementation Plan: One word for the set of findings that gets published

**Branch**: `015-publishable-set` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/015-publishable-set/spec.md`

## Summary

`internal/publish/gates.go` loses its private `included()` helper and calls `draft.PublishableSet` at all three sites. Four user-visible strings pick up the word for the set they actually describe: two gates say `publishable`, the post-confirmation gate and its mirror in the publish help say `published`. `docs/comment-format.md` gains the same split across seven lines. Spec 001's `spec.md`, `plan.md`, `contracts/cli.md` and `tasks.md` are amended to match, its readiness requirement is corrected to the rule the code derives, and its dead-lock-holder edge case is corrected to what `internal/run/lock.go` does. Nothing else moves: no refusal condition, no refusal code, no rendered byte.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. One function is deleted.

**Storage**: Unchanged. The `Included` field keeps its name and its JSON key, and no stored run needs migrating.

**Testing**: The assertion is that nothing moved. `go test ./internal/publish/` pins that every gate still fires on the same input after the helper is deleted. `go test ./internal/render/` runs **without** `-update`, so a moved body golden means the premise is broken. `go test ./internal/cli/` needs `-update` once, for line 20 of the two `publish-help` goldens and nothing else. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.1 Boundaries and `CONTRIBUTING.md:21` — `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md` are contracts, so this specification is the amendment `CONTRIBUTING.md:68` requires the pull request to name. The reconciliation marker, the `loupe-meta` keys and the body composition rules are untouched.

**Scale/Scope**: One deleted function, three repointed call sites, four strings, one comment, one test assertion, seven lines of a contract, and the spec-001 amendments.

## Research

- **The helper already exists.** Decision: delete `internal/publish/gates.go`'s `included()` and call `draft.PublishableSet`. Rationale: FR-001 and Constitution VI. `PublishableSet` at `internal/draft/digest.go:14` applies the same predicate, is already exported, and is already called twice by `internal/publish/envelope.go`. `included()` reaches the same answer through `draft.Dispositions`, which is a map built by calling the same private `disposition` over the same slice in the same order, so the two are the same function written twice. Keeping a second copy under a name that misdescribes it is how the wrong word survived. Alternative rejected: renaming `included()` in place, which leaves the duplicate and buys a word.

- **Two words, not one.** Decision: the gate before the confirmation says `publishable`, the gate after it says `published`. Rationale: FR-002 and FR-003. They test different sets. `internal/publish/gates.go:56` runs before readiness and reasons over `accepted` and `pending`. `internal/publish/publish.go:179` runs after readiness and tests `len(env.Findings) == 0`, which is accepted-only, so it is testing what the body will carry. A blanket "publishable" would erase the distinction the specification exists to draw, and a blanket "published" would be wrong about the first gate, which fires on a draft where nothing has been accepted yet. Alternative rejected: one word for both, which is what `internal/cli/publish.go:31` already does badly, in one sentence, with both words.

- **The help sentence is the place the distinction shows.** Decision: the `empty` row reads "no summary and no publishable findings; and, after you confirm, no message and no published findings". Rationale: FR-004. It is the only sentence in the repository that names both gates in a row, so it is the only place a reader can see that they differ. T120 half-fixed it: it wrote `publishable` into the first clause and left `included` in the second, which reads as a slip rather than a distinction.

- **The contract splits three ways.** Decision: `docs/comment-format.md` says `published` at the three lines describing the body's contents (`:45`, `:119`, `:216`), `publishable` at the four describing a gate's set (`:232`, `:233`, `:256`, `:278`), and keeps ordinary English at `:158` and `:184`. Rationale: FR-005. The `--inline` table rows name the set `--inline` chooses from, which is publishable and located. The Markdown check at publish runs over `envelope.go`'s set, which is the publishable set, and `:278` is the refusal that check raises, so it takes the same word: under `--unattended` readiness is skipped, so a `pending` finding's body reaches that check and can refuse there. The two ordinary-English hits — a diff side "included inside the span" and "bidi overrides included" — are the word doing its everyday job. Note: T125's line numbers are stale, from before the file moved. They were regenerated from a fresh grep and most of the cited lines no longer hold the word.

- **Readiness as written cannot hold.** Decision: FR-019 becomes "no finding pending and no note open". Rationale: FR-006. "Every included finding accepted" is not merely imprecise. An excluded finding stays `included: true` and is never accepted, so a draft with one excluded finding could never be ready, which is not what loupe does. `internal/draft/derive.go:72` already computes the correct rule, and `data-model.md:149` already states it. The specification was the only document that was wrong.

- **A flock cannot go stale.** Decision: rewrite spec 001's dead-lock-holder edge case and correct `plan.md:87`'s "stale-lock refusal text". Rationale: FR-008. The edge case says the refusal names a command to remove the lock. The kernel releases a flock when its holder dies, so the dead-holder scenario cannot arise, and `internal/run/lock.go:98-103` never names a removal command: it names the holder's pid and command and says to wait for it or stop it. `plan.md:136` already says all of this correctly, so the edge case is amended to it rather than to a new claim. `plan.md:87`'s file-listing comment carries the same wrong idea in three words.

- **The dead link.** Decision: `specs/001-loupe-v1/tasks.md:428` stops pointing at `github.com/eriksaulnier/loupe/issues/22` and points at this specification's directory. Rationale: FR-011. `gh api repos/eriksaulnier/loupe/issues/22` returns 404 and `gh issue list --state all` returns nothing: the repository has no issues at all. The link survived the republish from `loupe-archive` and is a dead link on a public repository. The work it tracked is this specification, so the directory is the honest target.

- **What the word correctly names.** Decision: `internal/draft/mutate.go:234` and `:273`, `internal/draft/mutate_decide_test.go:42`, `internal/cli/summary.go:28`, `internal/draft/digest.go:14`'s own doc comment, and the seven `contracts/cli.md` hits that are the flag or a count of it are untouched. Rationale: FR-007 and the first Edge Case. `IncludedCount` counts `f.Included` and is a genuinely different number from either set. The rule is to name the set you mean, not to replace a string.

- **What stays out.** Decision: `internal/publish/envelope.go`'s local `included`, and the hits in `specs/005`, `specs/013` and `specs/001-loupe-v1/research.md`. Rationale: the Edge Cases. The local variable holds a different set in each branch of the one function that computes both, has no prose to fix, and renaming it is a code change this specification would have to justify on its own. The earlier specifications are the record of what was decided at the time; spec 001's `spec.md`, `plan.md` and `contracts/cli.md` are amended only because that directory's own open tasks name them.

## Constitution Check

- **I. A tool for agents.** PASS. No command, flag, input, refusal code or `--json` envelope changes. Four refusal and help strings read correctly for the agent that reaches them.
- **II. Nothing posts unread under a human's name.** PASS. Every gate fires on the same condition. Readiness, the per-finding decisions and the confirmation are untouched.
- **III. Local files, no service.** PASS. Nothing written to disk changes, and no stored run needs migrating.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. Refusal codes, exit codes and result envelopes are unchanged; only the human-readable `message` of three refusals moves. `docs/comment-format.md` is amended from this specification, not redesigned.
- **VI. Simplicity over ceremony.** PASS, and this is the principle driving FR-001. A duplicate function is deleted, nothing is added, and no new abstraction appears.
- **VII. Verified means ran.** PASS. The verification is `go test ./internal/publish/` unchanged, `go test ./internal/render/` without `-update`, one deliberate golden regeneration read as a diff, and `mise run check`. The claim that the words read better to a human is named unverifiable rather than made.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/015-publishable-set/
├── spec.md
├── plan.md
└── tasks.md

internal/publish/gates.go                  # delete included(); three call sites; two refusal strings; the Touched comment
internal/publish/gates_test.go              # the empty-gate comment, which said the flag and meant the set
internal/publish/publish.go                # the post-confirmation empty refusal
internal/cli/publish.go                    # the empty row of the long help
internal/render/body.go                    # the Findings field comment
internal/tui/confirm_test.go               # the blocking string assertion
docs/comment-format.md                     # seven lines; two left alone
specs/001-loupe-v1/spec.md                 # FR-019, FR-025, FR-026, FR-043, US2 AS4, US3 and its scenarios, Q3, two edge cases
specs/001-loupe-v1/plan.md                 # the approve-with-blocking bullet and the lock.go file comment
specs/001-loupe-v1/contracts/cli.md        # the empty refusal row and the confirmation paragraph
specs/001-loupe-v1/tasks.md                # T123 to T127 checked off; the dead issue link
testdata/golden/cli/publish-help.*.txt     # regenerated once, line 20 only
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 014. The research is above, and the data model this change is about is already recorded in `specs/001-loupe-v1/data-model.md:151-153`.

## Complexity Tracking

No Constitution Check violations. `docs/comment-format.md` is a contract and this specification is the amendment the constitution requires, which is the mechanism rather than a departure from it. No Complexity Tracking entry is needed.
