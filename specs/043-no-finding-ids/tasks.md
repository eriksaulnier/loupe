# Tasks: Published findings never cite finding ids

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task follows failing test, then implementation, then a passing `mise run check`. Tests use `internal/markdown`'s table tests, `internal/draft`'s mutate tests, `internal/publish`'s `Build` and `Run` tests with the fake GitHub, and `internal/cli`'s command harness.

## Phase 1: Foundational

- [X] T001 Add `walkLines` in `internal/markdown/allowlist.go`, which calls its callback for every non-blank line outside a fence with whether the line is structural, and rebuild `walkStructural` as a filter over it. The existing `OpenDetails`, `MapSummaryLines` and `StructuralLines` tests in `internal/markdown/allowlist_test.go` MUST pass unchanged.
- [X] T002 Add kinds `Impact`, `SuggestedFix` and `Message` to `markdown.Kind` in `internal/markdown/allowlist.go`, at the body's, the body's and the summary's depth, named `impact`, `suggestedFix` and `message`. `scanner.refuse` adds `details.field` set to the kind's name. Test first in `internal/markdown/allowlist_test.go`: each kind's refusal message starts with its name, `details.field` equals it, and `rule` and `line` are unchanged.

## Phase 2: User Story 5 - An allowlist refusal names the field it refused (P3, done first because US1 shares its plumbing)

**Independent test**: `<br>` in `impact` and in `suggestedFix` is refused naming that field in the message and `details.field`, at `add`, `edit` and publish.

- [X] T003 [US5] Test first in `internal/draft/mutate_add_test.go` and `internal/draft/mutate_edit_test.go`: `<br>` in `impact` refuses at `Add` naming `impact`, `<br>` in `suggestedFix` refuses at `Edit` naming `suggestedFix`, and a `body` refusal keeps today's message with `details.field` `body`. Then pass `markdown.Impact` and `markdown.SuggestedFix` in `validateInput` in `internal/draft/mutate.go`.
- [X] T004 [US5] Test first in `internal/publish/envelope_test.go`: a stored publishable finding whose `impact` fails the allowlist refuses naming `impact`, and an attended message that fails it refuses naming `message`. Then pass `markdown.Impact` and `markdown.Message` in `Build` in `internal/publish/envelope.go`, and `markdown.SuggestedFix` in `internal/render/body.go`.
- [X] T005 [US5] Test first in `internal/cli`: `loupe add --json` with `<br>` in `impact` returns `error.details.field` `impact` and a message naming `impact`.

## Phase 3: User Story 1 - An id in published text is refused at write time (P1)

**Independent test**: the incident's title and impact shapes, replayed through `loupe add`, are refused with field, match and line, and nothing is stored.

- [X] T006 [US1] Add `FindingIDs(text string, kind Kind, retry string) error` and `TitleFindingIDs(title, retry string) error` in a new `internal/markdown/findingid.go`, with table tests in `internal/markdown/findingid_test.go` written first. The shape is "a lowercase `f-` followed by three or more ASCII digits, not preceded and not followed by an ASCII letter, an ASCII digit or `_`" (FR-004). A URL is "a run of text that starts with `http://` or `https://` and ends at whitespace or `>`" and is exempt, as is every line inside a fence read by `walkLines`. Code spans, link text, emphasis, quotes, HTML block lines and indented lines are not exempt (FR-005). The first match by line, then column, is refused with code `markdown`, `details` `{rule: "finding-id", field, match, line}`, a message naming field, match and line, and the fix from the plan's Design. Lines split at `\n`, `\r\n` and a lone `\r`. `TitleFindingIDs` reads no fences. Cases MUST include every shape in US2.4, fences, bare URLs, autolinks, link destinations, code spans, `f-002's`, an id on line 2, a `\r`-separated line, an indented line and a title opening with three backticks.
- [X] T007 [US1] Add `checkFindingIDs(title, body, impact, retry string) error` in `internal/draft/mutate.go`, which checks `title`, `body`, `impact` in that order. Call it from `CheckBatch` for each entry whose `validateInput` passed, with retry "run loupe add again". Test first in `internal/draft/mutate_add_test.go`: the incident title "… so f-002 still reproduces" refuses with `field` `title`, `match` `f-002`, `line` 1; an impact with `f-002's` on line 2 refuses naming `impact`, line 2; a batch whose entry 1 cites an id in `body` refuses whole with `details.entry` 1 and the entry's own `finding-id` details; a code span `` `f-002` `` refuses; a value failing both the allowlist and the id check is refused by the allowlist; the reworded finding is stored; `--by human` is not exempt.
- [X] T008 [US1] In `Edit` in `internal/draft/mutate.go`, after `validateInput`, call `checkFindingIDs` with only the fields whose value differs from the stored one and "" for the rest, and retry "run loupe edit <id> --from - again" with the finding's id. Test first in `internal/draft/mutate_edit_test.go`: an edit setting `body` to text citing `f-001` refuses naming `body`, and the stored finding, its rev and its decision are unchanged.
- [X] T009 [US1] In `SetSummary` in `internal/draft/mutate.go`, after the allowlist, call `markdown.FindingIDs(summary, markdown.Summary, "run loupe summary --from - again")`. Test first: a summary citing `f-014` refuses naming `summary`, and the stored summary is unchanged.
- [X] T010 [US1] In `Build` in `internal/publish/envelope.go`, after the note's allowlist check, call `markdown.FindingIDs(note, markdown.Note, "reword --note and run loupe publish again")`. Test first in `internal/publish/sticky_test.go` with the fake GitHub: an unattended sticky publish whose `--note` cites `f-002` refuses with `markdown`, `details.field` `note`, sends no create or edit review request and records no attempt; a whitespace-only note is not checked.
- [X] T011 [US1] Add `internal/cli` tests that `loupe add --json`, `loupe edit --json` and `loupe summary --json` with an id return exit 1, `error.code` `markdown`, `error.details.rule` `finding-id`, the field and the match, and that the draft version does not move.

## Phase 4: User Story 2 - Text that only resembles an id, or sits where ids are allowed, still lands (P1)

**Independent test**: one finding per exemption is stored.

- [X] T012 [US2] Test in `internal/draft/mutate_add_test.go` that each US2 case stores: an id in a fence, in a bare URL, an autolink and a link destination, in `suggestedFix` outside a fence, and `ref-002`, `f-02`, `F-002`, `f-002a`, `f-002_x`. Test that `SendBack` and `AddReply` accept a body citing `f-002`, and in `internal/publish/envelope_test.go` that an attended message citing `f-002` builds.

## Phase 5: User Story 3 - Stored drafts and unrelated edits are unaffected (P2)

**Independent test**: a stored finding whose body cites `f-002` takes a label edit, a title edit, a byte-for-byte `body` repeat, a reinstate and a recalibrate, and publishes.

- [X] T013 [US3] Test in `internal/draft/mutate_edit_test.go` on a stored finding whose `body` cites `f-002`: an edit of only `label` lands, an edit of `title` to clean text lands, an edit that repeats `body` byte for byte and changes `label` lands, so the repeated `body` reaches validation and is not checked, `Recalibrate` lands, and `Reinstate` of a withdrawn twin lands.
- [X] T014 [US3] Test in `internal/publish/envelope_test.go` that a draft whose accepted finding and summary cite an id builds attended and unattended, with the same body it composed before this change (FR-009, FR-012).

## Phase 6: User Story 4 - The rule is stated where ids are handed out (P2)

**Independent test**: each command's `--help` and the skill state the rule.

- [X] T015 [US4] Add the rule to `addHelp`, `editHelp` and `summaryHelp` in `internal/cli/add.go`, `internal/cli/edit.go` and `internal/cli/summary.go`: which fields refuse a finding id, that fences and URLs are exempt, and that code spans are not. Add to the `--previous` paragraph of `showHelp` in `internal/cli/show.go` that each id there belongs to its round, the current round reuses it, and published text names an earlier finding by its subject. Add to the `--note` paragraph of `publishHelp` in `internal/cli/publish.go` that the note refuses a finding id as the summary does. Test first in `internal/cli/help_test.go` that each help holds the rule. Regenerate goldens with `go test ./internal/cli/ -update` and confirm only `testdata/golden/cli/publish-help.80.txt` and `publish-help.100.txt` changed.
- [X] T016 [US4] State the rule in `plugin/skills/human-review/SKILL.md` step 2 (the `earlier` ids belong to their round), step 3 (findings) and step 5 (the summary). Run `internal/cli/plugin_test.go`.

## Phase 7: Contracts

- [X] T017 Amend `docs/comment-format.md`: the `title` row, a `### Finding ids` subsection under The supported Markdown boundary naming the checked fields, the shape, the exemptions, the `finding-id` code and where the check runs (`add`, `edit`, `summary` for findings and the summary, publish for the round note only), the allowlist text saying a refusal names its field, and a Refusals row.
- [X] T018 Amend `specs/001-loupe-v1/contracts/cli.md`: the `markdown` error row names the `finding-id` code, the `title` field and the round note, and says every `markdown` refusal of a published field carries `details.field`; the `add`, `edit`, `summary` and `publish` sections each gain one line stating the rule.

## Phase 8: Polish

- [ ] T019 Run `mise run check` after the last edit and confirm `git diff --stat main -- testdata/golden` lists only the two `publish-help` goldens.

## Dependencies

T001 and T002 first. US5 (T003–T005) before US1, because both name fields through the same kinds. T006 before T007–T011. US2 and US3 tests after T007–T010. T015–T018 after the code lands. T019 last.
