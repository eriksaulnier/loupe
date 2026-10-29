# Implementation Plan: Published findings never cite finding ids

**Branch**: `043-no-finding-ids` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

## Summary

`internal/markdown` gains a finding-id check beside the allowlist. `add`, `edit` and `summary` run it at write time, after the allowlist, on `title`, `body`, `impact` and the summary, and `publish` runs it on the sticky round note only. It refuses with the existing `markdown` code and `details.rule` `finding-id`. The allowlist refusal starts naming the real field, so `impact` and `suggestedFix` stop reading as "body", and both refusals carry `details.field`. The help, the skill, `docs/comment-format.md` and `contracts/cli.md` state the rule. The rendered review does not change.

## Technical Context

Go, standard library and cobra, as today. No new dependency, no run-file field and no schema bump (FR-017). The change touches `internal/markdown/allowlist.go` and a new `internal/markdown/findingid.go`, `internal/draft/mutate.go`, `internal/publish/envelope.go`, `internal/render/body.go` (one call site), the help in `internal/cli/{add,edit,summary,show,publish}.go`, `plugin/skills/human-review/SKILL.md`, `docs/comment-format.md`, `specs/001-loupe-v1/contracts/cli.md`, and the goldens `publish-help.80.txt` and `publish-help.100.txt`.

## Research

- **Where the allowlist runs.** `markdown.Check(text, kind, fix)` is called from `draft.validateInput` for `body`, `impact` and `suggestedFix`, from `draft.SetSummary`, from `draft.SendBack` and `draft.AddReply`, from `publish.Build` for the opening prose, the note and each publishable finding's `body` and `impact`, and from `render` to decide whether a stored `suggestedFix` needs a fence. Every finding field is checked with `markdown.Body`, which is why the refusal says "body" for `impact` and `suggestedFix`.
- **What `edit` validates.** `draft.Edit` builds the post-edit finding and, when any publishable field changed, runs `validateInput` on all of it. `severity` and `suggestedFix` are blanked when unchanged so a stored value is not rechecked. The id check needs the same treatment for `title`, `body` and `impact`: it reads only fields whose new value differs from the stored one (FR-002). `Recalibrate` and `Reinstate` go through `Edit` and change no checked field, so they never meet the check (US3.4).
- **Batch refusals.** `draft.CheckBatch` collects each entry's first refusal from `validateInput` and keeps each refusal's `details`. Running the id check inside `CheckBatch`, after `validateInput` succeeds for an entry, gives FR-008 with no new plumbing.
- **How the allowlist reads fences.** `walkStructural` already mirrors `scanner.line`'s fence and HTML-block reading for `OpenDetails` and `StructuralLines`. The id check needs every line outside a fence, including indented and non-tag lines, so `walkStructural` becomes a filter over a new `walkLines` that reports every non-blank line outside a fence and whether it is structural. The fence logic stays in one walker plus the scanner, as today. A fence's opening line, info string included, counts as fence: GitHub never shows the info string.
- **Titles.** A title is never allowlist-checked and is one escaped paragraph, so a backtick run there is text, not a fence. The title is matched line by line with no fence reading, only the URL exemption (edge case: "A `title` cannot hold a fence").
- **Publish and the note.** `publish.Build` checks the note against the allowlist after it blanks a whitespace-only note. The id check goes right after that call. `Build` runs before any attempt is recorded or any request is sent, so a refusal there leaves nothing to reconcile (FR-019). The `--note` usage refusal in `internal/cli/publish.go` runs before the run is resolved and stays first.
- **The human's message.** `Build` checks the attended opening with `markdown.Summary`, so its refusal says "summary" today. Adding `details.field` would label the human's message `summary`. It gets its own kind, `Message`, at the summary's depth, so the refusal says `message`. FR-018 asks every published field to be named as itself, and the message is published. It is not id-checked (FR-010).
- **Unpublished bodies.** `loupe reply` bodies and send-back notes keep `markdown.Body`, so their message reads as today. They also gain `details.field` `body`, the name their message already uses, because the field key is set in the one place every allowlist refusal is built. That is a superset of FR-018's "every refusal of a published field", not a change of wording.
- **Goldens.** `testdata/golden/cli` pins `help`, `handoff-help`, `publish-help`, `show`, `list`, `feedback` and `refusal`. `add`, `edit`, `summary` and `show` help are not pinned, so only `publish-help.{80,100}.txt` move (SC-005). The `refusal` golden renders a `not-found` refusal from `show` and is untouched.

## Design

- **Kinds.** `markdown.Kind` gains `Impact`, `SuggestedFix` and `Message`. `Impact` and `SuggestedFix` have the body's depth; `Message` has the summary's. Each kind names its field: `body`, `impact`, `suggestedFix`, `summary`, `note`, `message`. `scanner.refuse` adds `details.field`. Callers pass the kind of the field they check: `validateInput` and `publish.Build` use `Impact` for `impact`, `validateInput` and `render` use `SuggestedFix`, and `Build` uses `Message` for the attended opening.
- **The id check.** `markdown.FindingIDs(text string, kind Kind, retry string) error` reads `text` with `walkLines`, skips fence lines, blanks each URL (`http://` or `https://` up to whitespace or `>`), and scans for the first `f-` followed by three or more ASCII digits with no ASCII letter, digit or `_` on either side. `markdown.TitleFindingIDs(title, retry string) error` does the same with no fence reading. Lines split as `Check` splits them. The first match by line, then column, is refused.
- **The refusal.** Code `markdown`. Message: `<field> cites finding id <match> at line <n>; a reader of the review cannot resolve an id, since every round numbers its findings from f-001`. Details: `rule` `finding-id`, `field`, `match`, `line`. Fix: `name the earlier finding by its subject, or put a codebase's own identifier in a fence, then <retry>`. For a title the fence advice reads "put a codebase's own identifier in a fence in the body or impact and word the title around it". `retry` is "run loupe add again", "run loupe edit <id> --from - again" with the real id, "run loupe summary --from - again", or "reword --note and run loupe publish again".
- **Where it runs.** `draft.checkFindingIDs(title, body, impact, retry)` checks title, body, impact in that order and is a no-op on an empty string. `CheckBatch` calls it for each entry whose `validateInput` passed, with "run loupe add again". `Edit` calls it after `validateInput`, passing only fields whose value changed and "" for the rest, with the finding's own id in `retry`. `SetSummary` calls `FindingIDs(summary, Summary, …)` after the allowlist. `publish.Build` calls `FindingIDs(note, Note, …)` after the note's allowlist check. Nothing else calls it (FR-009, FR-010).
- **Order.** Every allowlist check of an entry runs before any id check, so a value failing both is refused by the allowlist (FR-007), and ids are reported in field order `title`, `body`, `impact`.
- **Help and skill.** `add`, `edit` and `summary` help each gain one sentence naming the checked fields, the shape, and that fences and URLs are exempt and code spans are not. `show --help`'s `--previous` paragraph says each id there belongs to its round, the current round reuses it, and published text names an earlier finding by its subject. `publish --help`'s `--note` paragraph says the note refuses a finding id as the summary does. The skill states the rule in steps 2, 3 and 5.
- **Contracts.** `docs/comment-format.md`: the `title` row, a new `### Finding ids` subsection under The supported Markdown boundary, the allowlist text naming the field, and a Refusals row. `contracts/cli.md`: the `markdown` error row, and one line each in `add`, `edit`, `summary` and `publish`.

## Constitution Check

- **I.** Every command that refuses states the rule in its own `--help`, and `show --previous` says it where the ids are handed out. The skill adds no code path.
- **II.** Unchanged. Publish keeps every gate, and the note check refuses before any request.
- **III, IV.** Unchanged. No new file, no network, no Git change.
- **V.** The refusal names its reason and its corrective command. The contract is amended by this spec. No run file changes, so no schema bump.
- **VI.** One new function pair in an existing package, reusing the allowlist's walker and refusal code. No dependency.
- **VII.** Tests use the fake GitHub and local Git repositories through the existing `internal/cli` and `internal/draft` harnesses.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/comment-format.md` and `contracts/cli.md` gain the `finding-id` code, the `title` field, the round note and `details.field` | This spec is the amendment (FR-013, FR-014). |
| The attended message's allowlist refusal says `message`, not `summary` | FR-018 names each published field as itself; the spec does not list the message, and calling it `summary` in `details.field` would mislabel it. |
| `loupe reply` bodies and send-back notes gain `details.field` `body` | The key is set where every allowlist refusal is built; their message text does not change. |
