# Feature Specification: Published findings never cite finding ids

**Feature Branch**: `043-no-finding-ids`

**Created**: 2026-10-28

**Status**: Draft

**Input**: Board draft "Draft spec: published findings never cite finding ids", with the owner's decisions on its open questions (2026-10-28): inline code spans are not exempt, run references are left for a follow-up, and linking prior-round ids is deferred.

This spec amends `docs/comment-format.md`, which the constitution makes a contract that changes only by a spec amendment. It also widens the `markdown` row of the error-code table in `specs/001-loupe-v1/contracts/cli.md`, and makes the existing allowlist refusal name the field it refused. It adds no refusal code and changes no exit code.

A finding id such as `f-002` names one finding inside one round's draft. Ids restart at `f-001` in every round (`internal/draft` numbers them `f-%03d` from the draft's own findings), and the published review shows none of them. Spec 027 keeps them only in the hidden findings record. A reader of the review has no way to resolve one.

It already happened. On a client pull request, round 2's agent read round 1 through `loupe show --previous`, where `f-002` was one finding. It titled a round-2 finding "… so f-002 still reproduces" and opened that finding's impact with "f-002's … comes back". In round 2, `f-002` was a different finding. The human accepted it, because the sentence reads fine to anyone who holds loupe's context, and the human deciding the finding holds exactly that context. The pull request's author does not.

`show --previous` hands the agent ids that the current round reuses, so this can collide on every follow-up round that cites the one before it. Nothing refuses it today. The allowlist checks the structure of `body`, `impact`, `suggestedFix`, the summary and the sticky round note, not what they say. `title` is checked only for being non-empty at write time, and is escaped at composition.

## Clarifications

### Session 2026-10-28

- Q: Are inline code spans exempt from the check? → A: No. Fences stay exempt. A code span lets `` `f-002` `` through, which is the same failure in monospace.
- Q: Are run references such as `owner/repo#123@1` refused too? → A: Not in this spec. A follow-up MAY add them.
- Q: Should a prior-round id render as a link instead of being refused? → A: Deferred, not rejected. It is a bigger change than a refusal.
- Q: Is an id of the current round refused, or only one from an earlier round? → A: Refused whatever round it came from. The published review shows no id of any round.
- Q: Is the summary checked on an attended run, which does not publish it? → A: Yes. `loupe summary` refuses a finding id on every run (FR-003).
- Q: Is the sticky round note checked? → A: Yes. It is written only by `loupe publish --unattended --sticky --note` and never stored, so it is checked there, before any review request. This is the one exception to FR-009.
- Q: Should the existing allowlist refusal keep calling `impact` and `suggestedFix` "body"? → A: No. It names the real field in its message and in `details.field` (FR-018).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An id in published text is refused at write time (Priority: P1)

An agent files a finding whose title or impact cites an earlier round's finding by id. loupe refuses it and says which field, which text, which line, and how to fix it: name the earlier finding by its subject. The agent rewords it and files again.

**Why this priority**: this is the incident. A human cannot be relied on to catch it, because the human holds the context that makes the id read as meaningful.

**Independent Test**: replay the incident's shapes through `loupe add`: a title ending "so f-002 still reproduces" and an impact opening "f-002's failure comes back". Each is refused with the field, the match and the line, and the draft is unchanged.

**Acceptance Scenarios**:

1. **Given** an `add` input whose `title` contains `f-002`, **When** the agent runs `loupe add`, **Then** it refuses with `markdown`, `details.rule` is `finding-id`, `details.field` is `title`, `details.match` is `f-002`, `details.line` is 1, and nothing is stored.
2. **Given** an `add` input whose `impact` cites `f-002's` on its second line, **When** the agent runs `loupe add`, **Then** the refusal names `impact` and line 2.
3. **Given** a batch whose entry 1 cites an id in its `body`, **When** the agent runs `loupe add`, **Then** the whole batch is refused, `details.entry` is 1, and `details.entries` lists that entry with its own `finding-id` details, as for any other invalid entry.
4. **Given** a summary that cites `f-014`, **When** the agent runs `loupe summary`, **Then** it refuses with `markdown` naming `summary`, and the stored summary is unchanged.
5. **Given** a `body` that cites `` `f-002` `` in an inline code span, **When** the agent runs `loupe add`, **Then** it refuses, because a code span is not exempt.
6. **Given** a refused finding, **When** the agent rewords it to name "the earlier finding about the retry loop" and runs `loupe add` again, **Then** the finding is stored.
7. **Given** an `edit` that changes a finding's `body` to text citing `f-001`, **When** the agent runs `loupe edit`, **Then** it refuses naming `body`, and the stored finding, its rev and its decision are unchanged.
8. **Given** an unattended sticky publication whose `--note` cites `f-002`, **When** the pipeline runs `loupe publish --unattended --sticky --note`, **Then** it refuses with `markdown` naming `note`, before any review request is sent, and nothing is posted.

---

### User Story 2 - Text that only resembles an id, or sits where ids are allowed, still lands (Priority: P1)

An agent writes a finding about code, a URL, or a fix that happens to contain an id-shaped string, in a place the rule exempts. loupe stores it without comment.

**Why this priority**: a rule that refuses honest text teaches agents to work around it. The exemptions are the rule's precision.

**Independent Test**: file one finding per exemption below. Each is stored.

**Acceptance Scenarios**:

1. **Given** a `body` with `f-002` inside a fenced code block, **When** the agent runs `loupe add`, **Then** the finding is stored.
2. **Given** a `body` with `https://example.com/issues/f-002` as a bare URL, an autolink or a link destination, **When** the agent runs `loupe add`, **Then** the finding is stored.
3. **Given** a `suggestedFix` that contains `f-002` outside a fence, **When** the agent runs `loupe add`, **Then** the finding is stored, because `suggestedFix` is not checked.
4. **Given** text containing `ref-002`, `f-02`, `F-002`, `f-002a` or `f-002_x`, **When** the agent runs `loupe add`, **Then** the finding is stored, because none of them is the id shape.
5. **Given** a `loupe reply` body or a human's send-back note that contains `f-002`, **When** it is written, **Then** it is stored, because neither is published.
6. **Given** a human typing `f-002` in the message at the publish confirmation, **When** they publish, **Then** the message is not checked, because the words are the human's own.

---

### User Story 3 - Stored drafts and unrelated edits are unaffected (Priority: P2)

A draft written before this change holds a finding whose body cites an id. The agent edits that finding's label, the human reinstates or recalibrates it in `loupe review`, and the human publishes. None of these refuses.

**Why this priority**: the check exists to stop a new mistake, not to strand a draft that a human is part way through deciding.

**Independent Test**: seed a draft whose finding's body cites `f-002`. Edit only its label, accept it, and publish against the fake GitHub. Each step succeeds and the published body holds the text as stored.

**Acceptance Scenarios**:

1. **Given** a stored finding whose `body` cites `f-002`, **When** an `edit` changes only its `label`, **Then** the edit lands.
2. **Given** the same finding, **When** an `edit` changes its `title` to clean text and leaves `body` as stored, **Then** only `title` is checked and the edit lands.
3. **Given** the same finding, **When** an `edit` input repeats the stored `body` byte for byte, **Then** `body` counts as unchanged and is not checked.
4. **Given** the same finding, **When** the human reinstates it or changes its label or blocking in `loupe review`, **Then** the change lands.
5. **Given** a draft whose accepted finding or whose summary cites an id, **When** the human publishes it, attended or unattended, **Then** publish does not refuse over the id, and the body is composed as it is today. Only a `--note` passed to that publish is checked.

---

### User Story 4 - The rule is stated where ids are handed out (Priority: P2)

An agent working from `--help` alone, or from the plugin skill, learns before it writes that published text names an earlier finding by its subject, never by id.

**Why this priority**: Principle I requires every workflow to be completable from `--help`. A refusal the help never mentions costs a round trip each time.

**Independent Test**: read `loupe show --help`, `loupe add --help`, `loupe edit --help`, `loupe summary --help`, `loupe publish --help` and the skill. Each states the rule.

**Acceptance Scenarios**:

1. **Given** `loupe show --help`, **When** the agent reads the `--previous` paragraph, **Then** it says each id there belongs to its own round, the current round reuses it, and published text names an earlier finding by its subject.
2. **Given** `loupe add --help`, `loupe edit --help` or `loupe summary --help`, **When** the agent reads it, **Then** it says which fields refuse a finding id, that fences and URLs are exempt, and that code spans are not.
3. **Given** `plugin/skills/human-review/SKILL.md`, **When** the agent reads steps 2, 3 and 5, **Then** step 2 says the `earlier` ids belong to their round, and steps 3 and 5 state the rule for findings and for the summary.
4. **Given** `loupe publish --help`, **When** the agent reads the `--note` paragraph, **Then** it says the note refuses a finding id as the summary does.

---

### User Story 5 - An allowlist refusal names the field it refused (Priority: P3)

An agent files a finding whose `impact` holds raw HTML. loupe refuses it as it does today, and the refusal says `impact`, not `body`. The agent fixes the right field the first time.

**Why this priority**: the allowlist already refuses correctly, so nothing reaches GitHub through this gap. It only costs a round trip. This spec adds `details.field` to its own refusal, and the allowlist refusal should match it.

**Independent Test**: file one finding with `<br>` in `impact` and one with `<br>` in `suggestedFix`. Each refusal names its field in the message and in `details.field`.

**Acceptance Scenarios**:

1. **Given** an `add` input whose `impact` holds `<br>`, **When** the agent runs `loupe add`, **Then** it refuses with `markdown`, the message names `impact`, and `details.field` is `impact`, beside the existing `rule` and `line`.
2. **Given** an `edit` that changes `suggestedFix` to text with `<br>`, **When** the agent runs `loupe edit`, **Then** the refusal names `suggestedFix` in the message and in `details.field`.
3. **Given** a stored draft whose publishable finding's `impact` fails the allowlist, **When** the human publishes, **Then** the publish refusal names `impact` too.
4. **Given** a `body` or summary that fails the allowlist, **When** it is refused, **Then** the message reads as it does today and `details.field` is `body` or `summary`.

### Edge Cases

- A value that fails both the allowlist and this check is refused by the allowlist. The allowlist decides where fences are, and this check reads fences as the allowlist reads them.
- A field with several ids reports the first, by line and then by column. A finding with ids in several fields reports the first field in the order `title`, `body`, `impact`. Each batch entry still reports its first defect only.
- An id of the current round is refused too, whatever round it came from. The published review shows no id of any round, so a sibling finding is named by its subject as well.
- A `--note` that is empty or only whitespace is no note, as today, and is not checked.
- A `--note` passed without `--unattended --sticky` is still refused with `usage` first, before the run is resolved.
- On a `--note` refusal, nothing is sent and no attempt is left to reconcile. The pipeline rewords the note and runs `loupe publish` again.
- A line indented four or more columns is not exempt. The allowlist does not read it as a fence either, because CommonMark may read it as paragraph text.
- An unclosed fence never reaches this check, because the allowlist refuses it with `fence` first.
- A `title` cannot hold a fence. A finding about a codebase whose own identifiers look like `f-123` names that identifier in a fence in the `body` or `impact`, and words the title around it.
- An `edit` that clears `impact` with `null` leaves nothing to check in it.
- `--by human` does not exempt `add`, `edit` or `summary`. The rule is about the reader of the review, not about who wrote the text.
- The titles of earlier findings in "Still open from earlier rounds" are copies stored by `loupe assess`. They are not checked again, and they render as stored.
- Lines are counted as the allowlist counts them: one-based, ending at `\n`, `\r\n` or a lone `\r`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe add` MUST refuse a finding whose `title`, `body` or `impact` contains a finding id outside the exemptions of FR-005.
- **FR-002**: `loupe edit` MUST apply the same check to each of `title`, `body` and `impact` whose new value differs from its stored value. It MUST NOT check a field whose value after the edit equals the stored value, whether the input omits the field or repeats it. This is the rule `edit` already applies to `severity` and `suggestedFix`.
- **FR-003**: `loupe summary` MUST refuse a summary that contains a finding id outside the exemptions of FR-005. It MUST check every summary it writes, on every run.
- **FR-004**: A finding id MUST be matched by its shape, not against the run's live ids: a lowercase `f-` followed by three or more ASCII digits, not preceded and not followed by an ASCII letter, an ASCII digit or `_`. A prior round's id is the dangerous case, and it is not an id of this run.
- **FR-005**: The check MUST exempt the content of a fence, read as the allowlist reads fences, and a URL. A URL is a run of text that starts with `http://` or `https://` and ends at whitespace or `>`. The check MUST NOT exempt inline code spans, link text, emphasis, quotes, HTML block lines or indented lines.
- **FR-006**: A refusal MUST use the existing code `markdown` and exit 1. Its message MUST name the field, the matched text and its line. Its `details` MUST carry `rule` set to `finding-id`, `field` (`title`, `body`, `impact` or `summary`), `match` and `line`. Its `fix` MUST say to name the earlier finding by its subject, or to put a codebase's own identifier in a fence, and MUST name the command to retry: `loupe add`, `loupe edit <id> --from -` or `loupe summary --from -`.
- **FR-007**: When a value fails the allowlist, the allowlist's refusal MUST be returned and this check MUST NOT run on that value.
- **FR-008**: In a batch, this check MUST take part in the existing batch refusal: each invalid entry reports its first defect, and the batch is stored entirely or not at all.
- **FR-009**: `loupe publish` MUST NOT run this check on findings, on the summary or on the human's message, attended or unattended. A draft stored before this change MUST publish as it would today. The one exception is the round note of FR-019, which is written only at publish.
- **FR-010**: The check MUST NOT apply to `suggestedFix`, `label`, `references`, `loupe reply` bodies, send-back notes, the human's message at the publish confirmation, or stored assessments.
- **FR-011**: The check MUST apply whatever `--by` says and however the run was captured.
- **FR-012**: The rendered review MUST NOT change. For every draft that passes the check, the composed body, inline comments, findings record and digest MUST be what they are today.
- **FR-013**: `docs/comment-format.md` MUST state the rule: in The supported Markdown boundary, the `title` row and a new subsection for finding ids that names the checked fields, the shape, the exemptions and the `finding-id` code, and in Refusals, a row for it. It MUST say the check runs where each field is written: at `add`, `edit` and `summary` for the findings and the summary, and at publish for the round note only. It MUST also say that an allowlist refusal of a published field names that field.
- **FR-014**: The `markdown` row of the error-code table in `specs/001-loupe-v1/contracts/cli.md` MUST name the `finding-id` code, the `title` field and the round note, and MUST say that every `markdown` refusal of a published field carries `details.field`. The `add`, `edit`, `summary` and `publish` sections MUST state the rule in one line each.
- **FR-015**: The help of `loupe add`, `loupe edit` and `loupe summary`, and the `--note` paragraph of `loupe publish --help`, MUST state the rule and its exemptions. The `--previous` paragraph of `loupe show --help` MUST say the ids it shows belong to their round and MUST NOT appear in published text.
- **FR-016**: `plugin/skills/human-review/SKILL.md` MUST state the rule in step 2, where the `earlier` list is handed to the review, in step 3 for findings, and in step 5 for the summary.
- **FR-017**: The change MUST NOT add a runtime dependency, a run-file field or a schema bump.
- **FR-018**: Every allowlist refusal of a published field MUST name the field it refused, in its message and in `details.field`: `body`, `impact`, `suggestedFix`, `summary` or `note`, at write time and at publish. A finding's `impact` and `suggestedFix` MUST NOT be called "body". The `rule`, `line`, code, fix and exit code stay as they are.
- **FR-019**: `loupe publish --unattended --sticky --note` MUST refuse a note that contains a finding id outside the exemptions of FR-005. The check MUST run where publish already checks the note against the allowlist, after that check, and before any attempt is recorded or any review request is sent. The refusal MUST follow FR-006 with `field` set to `note`, and its `fix` MUST say to reword `--note` and run `loupe publish` again.

### Key Entities

- **Finding id**: the id loupe gives a finding in one round's draft, `f-001` onward. It is unique only within that round. The published review carries it only in the hidden findings record.
- **Checked field**: `title`, `body` and `impact` of a finding, the draft's summary, and the round note of an unattended sticky round. These are the authored texts a reader of the review sees that an agent writes, through `add`, `edit` and `summary`, or through `publish --note`.
- **Exempt span**: the content of a fence, or a URL. An id inside one is not refused.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The incident's title and impact shapes, replayed through `loupe add`, are refused with the right field, match and line, and nothing is stored.
- **SC-002**: Every exemption case in User Story 2 is stored with no refusal.
- **SC-003**: An edit that changes no checked field never refuses for an id, tested on a stored finding whose `body` cites one.
- **SC-004**: A draft holding an id-citing finding and summary publishes, attended and unattended, against the fake GitHub with no refusal. The same unattended sticky publish with an id-citing `--note` is refused and sends no request.
- **SC-005**: The only goldens under `testdata/golden` that change are `publish-help.80.txt` and `publish-help.100.txt`, for the `--note` paragraph. Every existing render test passes unchanged.
- **SC-006**: An agent reading only `--help` finds the rule in `add`, `edit`, `summary`, `show --previous` and `publish --note`.
- **SC-007**: An allowlist refusal of `impact` or `suggestedFix` names that field in its message and in `details.field`, at write time and at publish.

## Assumptions

- Code spans are not exempt (owner, 2026-10-28). Exempting them lets `` `f-002` `` through, which is the same failure in monospace. The cost is a finding about a codebase whose own identifiers look like `f-123`: it puts them in a fence and words the title around them.
- Only lowercase ids match. loupe writes ids in lowercase and `show` prints them that way. `F-002` is left alone.
- The summary is checked on every run, although an attended publication does not post it (owner, 2026-10-28). An unattended publication posts it as its opening prose. loupe could tell the two paths apart by the viewer that capture records, but the allowlist already checks the summary at write time on every run, one rule is simpler to teach, and the cost of a refusal is one reword.
- The refusal reuses `markdown`. It is already the write-time refusal for these fields' content, and its fix already names the same commands. A new code would add a contract row for no new correction. The contract's `markdown` row widens to cover the `title` field and the `finding-id` code.
- `details.field` is new for a `markdown` refusal. Today the allowlist's own refusal calls a finding's `impact` and `suggestedFix` "body" and carries only `rule` and `line`. FR-018 fixes that, so both checks name fields the same way (owner, 2026-10-28).
- The round note is the one checked field that loupe never stores. Publish is the only place it is written, so publish is where it is checked (owner, 2026-10-28).
- The incident was observed once, on a client pull request. This spec names no client, repository or pull request from it.

## Out of Scope

- Run references such as `owner/repo#123@1`. They are internal too, but no incident is known. A follow-up MAY add them.
- Rendering a prior-round id as a link to its published thread. It would keep the reference and make it resolvable, and it is a bigger change than a refusal, so it is deferred, not rejected.
- Note ids (`n-001`), reply ids (`r-001`) and assessment refs (`e-1`). None has been seen in published text.
- Naming the field in allowlist refusals of `loupe reply` bodies and send-back notes. Those are not published, and their refusals keep today's wording.
- Checking stored findings or the summary at publish, or rechecking stored drafts.
