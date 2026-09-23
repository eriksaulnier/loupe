# Feature Specification: Two sections and readable rows

**Feature Branch**: `020-review-format`

**Created**: 2026-09-22

**Status**: Draft

**Input**: Owner, 2026-09-22: the published review is built for many findings, but real reviews hold zero to ten and usually one. PR #24's review opens with `⛔ 1 blocking` directly above a `### ⛔ Blocking` heading that holds that one finding. The collapsed rows should be the scan surface: two sections at most, one styled row per finding. Revised the same day in review of the spike: the row reads `dot label severity: title`, severity is a colored pill, and the location stays in the meta block.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries). It reverses parts of `specs/001-loupe-v1` and `specs/014-severity-badges`, listed in the plan's Complexity Tracking.

- **Spec 001** set five label sections, each heading led by its chip's dot, and rows in the label sections with no dot.
- **Spec 014** put the severity word on the summary line, joined to the label with ` · ` in one bold prefix, and made every terminal surface follow the body's section order. It also promised that an unrated, nonblocking finding renders byte for byte what it did before.
- `specs/015-publishable-set` changed no rendered byte and is not touched.
- The finding vocabulary does not change, and no stored run needs migrating. One input valid today becomes invalid: a new or edited `suggestedFix` that the allowlist refuses (FR-024).
- The `loupe-meta` marker, the reconciliation marker, the footer and the meta block do not change. Inside the disclosure, impact moves ahead of the body, and the suggested fix and references are reformatted (User Story 4).
- `suggestedFix` becomes Markdown under the body allowlist. A new or edited fix with raw HTML the allowlist refuses is now refused, and code in a fix MUST be fenced by whoever writes it. A stored fix that fails the allowlist still publishes, fenced as before.
- Constitution 2.0.1 is unchanged.

### Why this is worth a contract change

Five sections pay off when a review holds many findings of every label. A review with one finding pays for a chip, a divider, a heading and a divider before the reader reaches the row. The label is also the weakest sorting axis on the page: blocking decides whether merge waits, and severity says how bad the consequence is. The label says only what kind of remark it is.

The row itself reads as one run of text. `<b>major · issue:</b> Retry loop can double-publish a review` puts two enum words and a title in one bold-then-plain string, and the reader must find the colon to know where the title starts. The location, which says where to look, is inside the collapsed disclosure.

### What this deliberately costs

- **Byte-for-byte comparability with older reviews ends.** Every summary line changes, rated or not. Spec 014's SC-003 no longer holds. A reader comparing an old review to a new one sees a different row format, and this is accepted.
- **The heading no longer names the label.** A nonblocking row now carries its label in bold, after its label dot. The chips row still counts per label.
- **The `Must fix` heading carries no `⛔`.** Every row under it already leads with one, and the dot is a key into the chips row on every row, so a blocking row has the same mark in the body as inline. Owner, 2026-09-22: one `⛔` per row, none on the heading, which also matches `Worth a look`.

### A severity pill, after three spike rounds

The spike ran on 2026-09-22 against `eriksaulnier/loupe-format-spike`. It is recorded in `docs/github-facts.md`. The bar was a mark that is legible, centered on the text, no taller than a text row, and that leaves the `<summary>` click alone.

- **Round one (PR #1) failed the bar.** At 18px the row grew about 4px and the pills sat high. `align="middle"` sat low. The 18px drawing scaled to 14px kept the row height, but its text shrank to about 8px. A plain `<img>` is wrapped by GitHub in a link to itself, which takes the click on a `<summary>`. `<picture>` is not wrapped.
- **Round two (PR #2) passed it.** A pill drawn at 14px with 10px text, inside `<picture>`, with `align="absmiddle"`, was legible and kept rows 37.4px apart, the same as text. It sat 1 to 2px low.
- **Round three (PR #3) centered it.** The same pill in the top 14px of a 16px image, `height="16"`, sits centered, with rows still about 37px apart.
- The owner chose that pill for severity, hosted at a versioned path on `main`. The contract's ban on badge images becomes a set of rules for this one image.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pull-request reader scans a short review (Priority: P1)

Someone who did not run loupe opens the review on their pull request. It holds one blocking finding and two others. They see the chips row, a `Must fix` section with one row and a `Worth a look` section with two. Each row shows, before anything is opened, the finding's mark, how bad it is, what kind of remark it is, its title and the file and line.

**Why this priority**: It is the surface with the most readers and the least context, and it is what the contract amendment authorizes.

**Independent Test**: Compose a body from a mixed draft and read it. Compose bodies with only blocking and only nonblocking findings, and check that the empty section is absent.

**Acceptance Scenarios**:

1. **Given** a blocking finding with `severity: major`, label `issue`, title `Retry loop can double-publish` and location `internal/publish/publish.go:88` on the right side, **When** the body is composed, **Then** its summary line is `⛔ <b>issue</b> <pill MAJOR>: Retry loop can double-publish`, where `<pill MAJOR>` is the `<picture>` of FR-010.
2. **Given** blocking and nonblocking findings, **When** the body is composed, **Then** it has `### Must fix` holding every blocking finding, followed by `### Worth a look` holding every other published finding, and no other finding section.
3. **Given** only nonblocking findings, **When** the body is composed, **Then** it has no `Must fix` section. **Given** only blocking findings, **Then** it has no `Worth a look` section.
4. **Given** a nonblocking `suggestion`, **When** its row is composed, **Then** it leads with `🟣`. An `issue` leads with `🟡`, a `question` with `🔵`, and an unknown or empty label with `⚪`.
5. **Given** a finding whose severity is absent, empty or free text, **When** its row is composed, **Then** the row has no pill, and free text still renders on the meta line as it does today.
6. **Given** a finding with the label `perf-nit`, **When** its row is composed, **Then** the row carries `<b>perf-nit</b>`. **Given** an empty label, **Then** the row has no label. **Given** neither a label nor a rated severity, **Then** the row is the dot and the title, with no colon.
7. **Given** a located finding, **When** its row is composed, **Then** the row carries no location.
8. **Given** any finding, **When** its disclosure is composed, **Then** the meta block still holds the full path linked to the files view, unchanged.

---

### User Story 2 - Inline comments use the same row (Priority: P1)

A reader looking at a line in the files view sees an inline comment that leads with the same row as the body.

**Why this priority**: One row rule for both places is what makes the row learnable.

**Independent Test**: Publish with `--inline all` against the fake GitHub and read each comment body's first line.

**Acceptance Scenarios**:

1. **Given** a located blocking finding and `--inline blocking`, **When** its inline comment is composed, **Then** its first line is the body row, `⛔ <b>issue</b> <pill MAJOR>: Retry loop can double-publish`.
2. **Given** a nonblocking unlabeled unrated finding and `--inline all`, **When** its inline comment is composed, **Then** its first line is `⚪ ` followed by the title.

---

### User Story 3 - The human decides findings in the review's order (Priority: P2)

The human opens `loupe review`. The list walks findings in the order the published review will present them: every blocking finding first, then everything else, each group by severity, then label group, then id.

**Why this priority**: Spec 014 FR-001 requires one order on every surface. The body's order changes, so the terminal order changes with it. It adds no new behavior.

**Independent Test**: Open `loupe review` and `loupe show` against a draft that holds a nonblocking `critical` question and a nonblocking `minor` issue. The question comes first. Before this change the issue came first, because the `Issues` section came before `Questions`.

**Acceptance Scenarios**:

1. **Given** nonblocking findings of mixed labels and severities, **When** the review list, the line-by-line mode or `loupe show` presents them, **Then** they are in severity order, and label group breaks only ties.
2. **Given** a blocking `minor` finding and a nonblocking `critical` one, **When** any surface presents them, **Then** the blocking finding is first.

---

### User Story 4 - A reader opens a finding (Priority: P2)

A reader expands a finding. They read what goes wrong first, then the reasoning, then the fix as ordinary prose, then a short, readable link to each reference.

**Why this priority**: The fix was always in a code fence, so a one-sentence fix read as monospace with a copy button, and references read as raw wrapped URLs. Owner, 2026-09-22: do this, but keep the meta block's one-part-per-line layout.

**Independent Test**: Compose a disclosure with one-line and multi-line impact, fix and references, and a stored fix that fails the allowlist.

**Acceptance Scenarios**:

1. **Given** a finding with impact, body, suggested fix and references, **When** its disclosure is composed, **Then** the order is meta block, impact, body, suggested fix, references.
2. **Given** a one-line impact `Two reviews.`, **When** it is composed, **Then** it reads `**Impact:** Two reviews.` on one line. **Given** an impact of several lines, **Then** `**Impact**` stands on its own line with the value below.
3. **Given** a suggested fix `Return the original error.`, **When** it is composed, **Then** it reads `**Suggested fix:** Return the original error.` in prose, with no fence.
4. **Given** a stored suggested fix holding `<br>`, **When** it is composed, **Then** it is fenced under `**Suggested fix**` as before, and publish does not refuse it.
5. **Given** `loupe add` or `loupe edit` with a suggested fix holding `<br>`, **When** it runs, **Then** it is refused with the allowlist's `html` rule and the correction `loupe edit <id> --from -`.
6. **Given** one reference `https://github.com/o/r/issues/12`, **When** it is composed, **Then** it reads `**References:** [github.com/o/r/issues/12](<https://github.com/o/r/issues/12>)`. **Given** several, **Then** each is a bullet under `**References**`.
7. **Given** a reference whose host and path are over 50 characters, **When** it is composed, **Then** its text is `host/…/last-segment`.

---

### Edge Cases

- **A review with one finding.** It has the chips row, one section heading and one row. The heading is kept so that a reader always knows which of the two sections they are in.
- **A blocking finding with an unknown label.** It sits in `Must fix`, leads with `⛔` and carries its label verbatim in bold. It counts in `blocking` in the chips row and under `other` in `loupe-meta`, as today.
- **A label or title holding `<`, `&` or a backtick.** The label is HTML-escaped inside its `<b>`, since GitHub parses no Markdown inside a `<summary>`. Title rules are unchanged.
- **An inline comment's first line is a Markdown paragraph.** Backslash escapes decode there, so every generated field in the row MUST survive that. The title already does.
- **A pill that does not load.** Email clients and blocked images show the `alt` word, which is the same uppercase word.
- **The pills before this merges.** The URLs point at `main`, where the files do not exist yet, so reviews published from this branch show the alt word.
- **Free-text severity from an older run.** No word on the row. It stays on the meta line in a code span.
- **A stored severity whose case differs.** Only the four lowercase enum words are rated today, and this changes nothing about that. The pill's alt text is the rated word in uppercase.

## Requirements *(mandatory)*

### Sections

- **FR-001**: The body MUST hold at most two finding sections, in this order: `### Must fix`, holding every published blocking finding, then `### Worth a look`, holding every other published finding. A section with no findings MUST be omitted.
- **FR-002**: Both sections MUST sort by severity (`critical`, `major`, `minor`, `trivial`, then unrated), then by label group (`issue`, `suggestion`, `question`, then every other or empty label as one group), then by finding id.
- **FR-003**: Every surface that presents a draft's findings (the review list, detail navigation, line-by-line mode and `loupe show`) MUST follow the order of FR-001 and FR-002. This keeps spec 014 FR-001 and changes its section list. The section rule and the label groups MUST keep one shared definition.
- **FR-004**: Dividers, the footer and both markers MUST keep their current rules. The opening changes only by FR-027.

### Chips

- **FR-005**: The chips row MUST keep its current appearance and bytes: `⛔ N blocking` first when any finding blocks, then one chip per label group counting the nonblocking findings, zeros omitted.
- **FR-006**: The contract rule MUST change from one chip per section to one chip per row dot. Each chip's dot is the dot that leads the rows it counts.
- **FR-007**: `loupe-meta`'s keys and meanings MUST NOT change.

### The row

- **FR-008**: Every finding's summary line, in the body and inline, MUST be the same row, apart from the inline escaping of FR-016, built in this order: the dot and a space, then the label and the severity pill with one space between them, then `: ` and the title. A row with neither label nor rated severity MUST be the dot, a space and the title.
- **FR-009**: The dot MUST be `⛔` for a blocking finding and the label group's dot otherwise: `🟡` issue, `🟣` suggestion, `🔵` question, `⚪` every other or empty label.
- **FR-010**: A rated severity MUST render as `<picture>` holding a `prefers-color-scheme: dark` `<source>` and an `<img>` with `alt` set to the uppercase word, `height="16"` and `align="absmiddle"`, both pointing at `https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/<word>[-dark].svg`. An absent, empty or free-text severity MUST give no pill. Free text MUST stay on the meta line, as today.
- **FR-010a**: The files under `assets/review/v1/` MUST NOT change once on `main`. A test MUST pin each file's hash and check that every enum word has a light and a dark file. A redesign MUST add a new version directory.
- **FR-010b**: Every terminal surface that shows the raw body or an inline comment before publishing MUST show each pill in a generated row as its uppercase word, and MUST leave authored text as written. The payload view MUST keep the exact bytes.
- **FR-011**: The label MUST be `<b>` around the label, verbatim apart from HTML escaping and line collapsing, for known and unknown labels alike. An empty label MUST give nothing.
- **FR-012**: The title MUST keep its current rendering rules.
- **FR-013**: The row MUST NOT carry the location. The meta block's first line carries it.
- **FR-014**: The row MUST NOT use ` · ` separators or the word `blocking`.
- **FR-015**: `<b>`, the title's `<code>`, and the pill's `<picture>`, `<source>` and `<img>` MUST be the only tags loupe emits in a summary line.
- **FR-016**: Every generated field in the row MUST be collapsed to one line, and MUST be escaped so that it renders as the same text in a `<summary>` and in an inline comment's Markdown paragraph.

### The opening

- **FR-027**: The opening prose, when present, MUST lead the body, and the chips row MUST follow it in the same block, directly above the first divider. With no prose the body MUST open on the chips row, byte for byte as before.
- **FR-028**: The draft's summary MUST NOT be added to an attended review, collapsed or otherwise. Constitution Principle II requires the opening prose to be the human's, and `specs/013-human-message` cut the summary as stale after exclusions and redundant with the findings. Owner, 2026-09-22, asked about a collapsed summary; it is out of scope here and would need its own specification and a constitution amendment.

### Unchanged

- **FR-017**: The meta block, including the full linked location, and the finding body MUST NOT change.

- **FR-018**: Section placement MUST be decided by `blocking` alone.
- **FR-019**: The severity pill MUST be the only image loupe emits in a composed body or inline comment. Authored Markdown MAY still hold images, as the allowlist permits. `docs/comment-format.md` MUST state its rules and their reasons, and `docs/github-facts.md` MUST record every spike round as observations.

### Inside the disclosure

- **FR-022**: The disclosure MUST render the meta block, then impact, then the body, then the suggested fix, then references.
- **FR-023**: Impact, suggested fix and references MUST each follow a bold label. A one-line value MUST sit on the label's line after `:`. A longer value MUST go below a bold label on its own line.
- **FR-024**: A suggested fix MUST be Markdown, checked against the body allowlist by `loupe add` and by a `loupe edit` that changes it. An edit that leaves a stored fix unchanged MUST NOT be refused for it, as with a legacy severity. A stored fix that fails the allowlist MUST render in a fence, as before, and MUST NOT make publish refuse.
- **FR-025**: Each reference MUST be a link whose text is its host and path, without scheme, query or fragment, shortened to `host/…/last-segment` when over 50 characters and the path has more than one segment. The destination MUST be in `<…>`. Escaping MUST keep both the text and the destination exactly as the URL gives them.
- **FR-026**: The help text for `--suggested-fix`, the agent skill, `contracts/cli.md` and the data model MUST say that the fix is Markdown and that code in it is fenced.

### The contract

- **FR-020**: `docs/comment-format.md` MUST be amended in Vocabulary (where it names the `Blocking` and `Other` sections), Body composition, Chips, The summary line (its table becomes one row rule), the severity pill, the field sections (Impact, Suggested fix, References), the supported Markdown boundary, and the example body, which is pinned against the rendered golden.
- **FR-021**: `specs/001-loupe-v1/spec.md`'s clarification log MUST gain a line that records the two sections and the row dots, and points here.

## Success Criteria *(mandatory)*

- **SC-001**: In a review with one finding, only the chips row, the opening prose, one divider and one heading stand between the top of the body and that finding's row.
- **SC-002**: A reader learns each finding's blocking state, kind, severity and title from its collapsed row, without opening it, and its file and line from the first line when it is opened.
- **SC-003**: The row rule is one rule. The body row and the inline first line of the same finding are the same bytes, apart from the inline escaping of the title and label.
- **SC-004**: On every surface, the human and the reader meet findings in the same order.
- **SC-005**: The chips row, the markers, the footer, the meta block and the finding body are byte-identical to what the same draft produced before this change.
- **SC-006**: All automated repository checks pass after the final edit.

## Assumptions

- Two sections is the right cut because `blocking` is the one axis that changes what the author must do before merge. Severity already orders findings within each section.
- The chips row is kept as a key for the dots, not an index of headings. Its bytes do not change, so tooling that reads it is unaffected.
- The location is not needed on the row. It is the first line a reader sees on opening a finding, and leaving it off keeps the row short.
- The row renders on GitHub the way the spike's variant N did, review 5286087452 on `eriksaulnier/loupe-format-spike#2`, checked by eye in light theme, and in both themes by `mise run review-screenshot` on 2026-09-23. Mobile and email rendering were not checked.
