# Feature Specification: Severity at the top level

**Feature Branch**: `014-severity-badges`

**Created**: 2026-09-18

**Status**: Draft

**Input**: Owner, 2026-09-18: severity should be visible and load-bearing on the two surfaces a reader scans before opening anything — the review list a human decides from, and the collapsed review a pull-request reader skims.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract, and completes the arc `specs/008-finding-fields` opened and `specs/012-severity-meaning` finished for the vocabulary.

- **Spec 008** made `severity` an enum so "a reader compares it across findings", and put it on the meta line inside the disclosure.
- **Spec 012** gave the four words meanings, and changed no rendered byte.
- Neither made severity reachable without opening a finding, and neither made anything sort by it. A rank that nothing ranks by is a field, not a scale.
- The four accepted values do not change. No input valid today becomes invalid, and no stored run needs migrating.
- Stored order does not change. `internal/draft` keeps findings in arrival order, and the publishable digest already sorts by finding id, so no digest moves because of this.
- Constitution 2.0.1 is unchanged. No principle is added, removed or redefined.

### Why this is worth a contract change

`severity` today sits on the meta line inside a collapsed `<details>`, and `docs/comment-format.md` states outright that it never reaches the `<summary>`. That rule was right when severity had no defined meaning: a word that meant nothing agreed-on had no business in the one line a reader gets for free. Spec 012 removed that objection and left the rule standing.

The cost is paid on every review. A reader scanning ten collapsed rows has the label, the blocking flag and the title, and nothing that says which of the four `issue` rows is the data-loss one. They open all ten, or they open none. Inside the `Blocking` section it is worse than neutral: findings sort by label group, so a `critical` question sits below a `minor` issue because `issue` sorts before `question`. The section whose whole job is "read these first" is ordered by something that is not urgency.

The terminal half has the same shape and needs no amendment: `loupe review`'s list carries id, blocking, note, title, label and location, and severity appears only after the human has committed to a finding. It belongs in this specification anyway, because the two surfaces MUST agree on one ordering rule. A human who decides findings in one order and then publishes a review that presents them in another has been given two different answers to "what matters most here".

### One thing said once

Two words on the summary line were saying what something else already said.

`**Severity:** major` on the meta line repeated the word the summary line now leads with, two lines above it. And ` (blocking)` repeated the `### ⛔ Blocking` heading in the body and the `⛔` dot inline — the contract already called the first of those redundant and kept it "so the inline surface, which has no heading, keeps the signal", which does not survive contact with the inline surface leading with `⛔`.

### One severity, in one place

Putting the word on the summary line makes the meta line's `**Severity:** major` a repeat: every context that renders a meta block renders a summary line directly above it, so for a rated finding the same word lands twice within two lines. `docs/comment-format.md` already has the rule for this shape — an inline comment carries no location line, "the comment already sits on the line" — and this applies it to severity. The cost is that the word is no longer labeled anywhere in the review; a reader meets it as the first word of a bold prefix. That reader is the one who opened a collapsed finding, and they read the word on the line above either way, so the label was buying nothing they did not already have.

### What this deliberately costs

`docs/comment-format.md` currently promises that findings within `Blocking` sort by label group, then by id. After this change severity outranks the label group, so that property is gone: two `issue` findings in `Blocking` MAY be separated by a `suggestion` between them. That is the point of the change rather than a side effect of it. The label group survives as the tie-break, so labels still cluster among findings of equal severity.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pull-request reader scans the collapsed review (Priority: P1)

Someone who did not run loupe opens the review on their pull request. Every finding is collapsed. They read the `<summary>` lines top to bottom and learn, without opening anything, which findings are the bad ones — and the order they are already reading in is that order.

**Why this priority**: It is the surface with the most readers and the least context. It is also the only one that requires a contract amendment, so it is what this specification exists to authorize.

**Independent Test**: Render a review holding one finding of each severity plus one unrated, and read the composed body. Each rated finding's summary line leads with its severity word; the sections are ordered `critical`, `major`, `minor`, `trivial`, then unrated.

**Acceptance Scenarios**:

1. **Given** a finding with `severity: major`, the label `issue` and `blocking` set, **When** the body is composed, **Then** its summary line reads `<b>major · issue:</b> ` before the title, and says `(blocking)` nowhere.
2. **Given** a finding with `severity: trivial` and the label `perf-nit` in `Other`, **When** the body is composed, **Then** its summary line reads `<b>trivial · perf-nit:</b> ` before the title.
3. **Given** a finding with `severity: minor` and no label word in its context, **When** the body is composed, **Then** its summary line reads `<b>minor:</b> ` before the title.
4. **Given** findings of mixed severity in `Blocking`, **When** the body is composed, **Then** they are ordered by severity first and by label group only among equal severities.
5. **Given** a located blocking finding and `--inline blocking`, **When** its inline comment is composed, **Then** the severity word leads the bold prefix there too, after the `⛔` dot.
6. **Given** a finding that carries no severity, **When** the body is composed, **Then** its summary line is byte for byte what it was before this change.

---

### User Story 2 - A human decides findings in the review list (Priority: P1)

The human opens `loupe review`. The list shows a severity column beside each finding, colored by how bad it is, and the worst findings are at the top. They walk down the list in the order the review will present it.

**Why this priority**: Per-finding sign-off is the product (Principle II). The list is where the human spends the decision, and spending it in arrival order means the `trivial` naming nit gets the same first attention as the data-loss bug.

**Independent Test**: Open the review interface against a draft of mixed severities at each of the three window widths, under color and under `NO_COLOR`. The column is present and ordered; under `NO_COLOR` it degrades to a plain word in a column.

**Acceptance Scenarios**:

1. **Given** a draft of mixed severities, **When** the review list is drawn, **Then** findings appear in the order the published review puts them in: every blocking finding first, then each label section, and within each of those every rated finding before every unrated one, in `critical`, `major`, `minor`, `trivial` order, ties broken by finding id.
1a. **Given** a nonblocking `critical` finding and a blocking `minor` one, **When** the list is drawn and the review composed, **Then** both put the blocking finding first.
2. **Given** the review list, **When** a row is drawn for a rated finding, **Then** a severity column between the id and the title carries the word.
3. **Given** a row for an unrated finding, **When** it is drawn, **Then** the severity column is blank rather than filled with a stand-in word.
4. **Given** a window too narrow for every column, **When** the list is drawn, **Then** the label drops first, then the location shortens to a filename, and the severity column drops last of the three.
5. **Given** the detail view, **When** the human walks findings with the next and previous keys, **Then** they move in the order the list showed, and the "N of M" position matches.
6. **Given** a finding open in the detail view, **When** its chips are drawn, **Then** the severity chip is colored by how bad it is rather than dimmed like an incidental field.
7. **Given** the line-by-line review mode, **When** findings are presented, **Then** they are in the same order the full-screen list uses.
8. **Given** `loupe show`, **When** findings are printed, **Then** they are in that same order and the severity cell is colored.
9. **Given** a finding with an enum severity, **When** its disclosure or inline comment is composed, **Then** the word appears on the summary line and nowhere else in that finding.

---

### Edge Cases

- **An unrated finding.** It sorts after every rated one **within its section**, in finding-id order, and carries no summary-line severity word, no column text and no chip. Severity is reviewer-reported and MUST NOT be inferred or defaulted, so treating an absent severity as `trivial` would invent a value the reviewer withheld. It does not sort after a rated finding in a later section: section placement outranks severity everywhere (FR-001).
- **A stored severity outside the enum.** Runs captured before 008 hold free text. It counts as unrated for ordering, is omitted from the summary line, and still renders on the meta line inside a code span exactly as today. It cannot lead a summary line, because a summary line interpolates its prefix outside a code span and only the enum words are known to be inert there.
- **Every finding unrated.** The claim is about the published review, and only about the findings that do not block. A nonblocking unrated finding renders byte for byte what it rendered before, summary line and meta block together, which is what keeps an older review comparable to a newer one (SC-003). A blocking one loses ` (blocking)` from its summary line, by FR-028. Section placement and the label groups already decided the body's order before this change, so an all-unrated body is byte-identical apart from that.
- **Every finding unrated, in the terminal.** Not byte for byte, and deliberately. The list still draws the severity column, blank in every row, because a column that came and went with the draft's contents would move every other column with it. The order still follows the published review's — `issue` before `suggestion` whatever the severities — where before this change the terminal walked the stored arrival order. Both are what FR-001 and FR-017 ask for; neither is covered by SC-003, which is about the review.
- **Severity and blocking disagree.** Unchanged and still allowed. Section placement is decided by `blocking` alone; severity orders findings within a section and never moves one between sections.
- **A window too narrow for the severity column.** The column drops, after the label has gone and the location has given up its width. It outranks both, but a title the reader cannot read costs more than any of them.
- **`NO_COLOR` or a non-UTF-8 locale.** The badge is a word plus a color, and no glyph. Under `NO_COLOR` the word stands alone in its column and the ordering still carries the rank; under ASCII nothing is lost, because there is no glyph to degrade.

## Requirements *(mandatory)*

### The ordering rule

- **FR-001**: One ordering rule MUST apply on every surface that presents a draft's findings to a reader, and it MUST be the sequence the published review puts them in: the body's section first (`Blocking`, then `Issues`, `Suggestions`, `Questions`, `Other`), then severity in `critical`, `major`, `minor`, `trivial` order with every unrated finding after them, then the label group that breaks ties inside `Blocking`, then the finding id. Severity MUST NOT outrank the section: the review places a blocking finding above every nonblocking one whatever its severity, so a terminal order that led with severity would walk the human through a different review from the one their name goes on.
- **FR-001a**: The section order and the label groups MUST have one definition, shared by the renderer that emits the sections and by the derivation every terminal surface reads.
- **FR-001b**: The recap of an already-published round is outside FR-001. It reads a stored receipt whose findings carry no severity, so ordering it would mean changing a stored schema, which Principle III puts out of scope. It keeps the order it has today, and this specification does not claim otherwise.
- **FR-002**: An absent severity, an empty severity and a stored severity outside the enum MUST all sort as unrated. None MAY be mapped onto an enum word for ordering.
- **FR-003**: Unrated findings MUST sort among themselves by finding id, which is the order every surface uses today.
- **FR-004**: The stored order of findings MUST NOT change, and the publishable digest MUST NOT change because of this feature.
- **FR-004a**: Editing a finding's label or blocking flag in review MAY move its row, because both decide its section. The cursor MUST stay on the finding that was open, not on the position it held.
- **FR-005**: The four words and their order MUST have exactly one definition in the codebase, shared by every surface that validates, sorts or colors them.

### The published review

- **FR-006**: A rated finding's summary line MUST lead its bold prefix with the severity word, separated from what follows by ` · `. Where the context carries no label word, the severity word is the whole prefix.
- **FR-007**: The severity word MUST be a word, never a colored dot. The contract already reserves dots for labels and records that a label dot under the `⛔` heading reads as a severity.
- **FR-008**: A finding that carries neither a severity nor the blocking flag MUST render byte for byte what it rendered before this change, summary line and meta block together, in every context. Blocking rows are excepted by FR-028 and only there.
- **FR-009**: Within `Blocking`, findings MUST sort by severity, then by label group, then by finding id. The label-grouping property the contract states MUST be amended to record that severity now outranks it.
- **FR-010**: Within a label section and within `Other`, findings MUST sort by severity, then by finding id.
- **FR-011**: An inline comment's summary line MUST carry the severity word on the same rule as the body, after its dot.
- **FR-012**: The chips row MUST NOT change. It is an index of the headings below it, and every chip maps to a heading a reader can scroll to; severity counts would stack a second taxonomy on it and break that property.
- **FR-013**: Section placement MUST NOT change. `blocking` alone decides `Blocking`, and the label alone decides the rest.
- **FR-014**: The meta block MUST NOT repeat a severity the summary line already carries. Every context that renders a meta block renders a summary line directly above it, so an enum word would otherwise appear twice, two lines apart. A value stored before the enum cannot reach the summary line, so it keeps its `**Severity:**` line and its code span. Confidence and verified are untouched, and a meta block left with no parts is omitted as it is today.
- **FR-015**: `docs/comment-format.md` MUST be amended where it states that severity never appears in the summary line, where it states the `Blocking` sort, and in its embedded example, which is pinned against the rendered golden.
- **FR-016**: No external badge image MAY be introduced. The contract's existing ban stands: a badge is a color and a word in the terminal, and a word on GitHub.
- **FR-028**: The summary line MUST NOT carry ` (blocking)`. Blocking is already said by the `⛔ N blocking` chip, by the `### ⛔ Blocking` heading in the body and by the `⛔` dot inline, and a blocking finding is never placed in a label section, so no context is left where the word is the only carrier. The label word MUST stay: the dot says that a finding blocks, not what kind of remark it is. This amends the answer recorded in `specs/001-loupe-v1/spec.md`'s clarification log.

### The terminal

- **FR-017**: The review list MUST carry a severity column, between the finding id and the title, wide enough for the longest word.
- **FR-018**: The column MUST be colored by severity, most severe to least, in four distinguishable colors. The existing semantic roles do not supply four: two of them share one yellow, so a ramp built from them alone collapses `major` and `minor` onto the same color. One palette entry MAY be added to separate them, and the role it defines MUST be named by what it means rather than by its color.
- **FR-019**: An unrated finding's severity column MUST be blank.
- **FR-020**: As the window narrows, the severity column MUST be the last of the three to give way. The two steps before it are unchanged: the label column drops first, then the location shortens to a filename. Severity goes only when the title cannot otherwise reach its floor.
- **FR-021**: The severity badge MUST NOT introduce a glyph. The word and its color are the badge, in every tier.
- **FR-022**: Finding-to-finding navigation in the detail view, and the "N of M" position it shows, MUST follow the list's order.
- **FR-023**: The severity chip in the detail view and in line-by-line mode MUST be colored by the same ramp. A legacy free-text severity keeps the dim it has today.
- **FR-024**: Line-by-line review mode MUST present findings in the list's order.
- **FR-025**: `loupe show` MUST print findings in the list's order, and MUST color the severity cell rather than leaving it in the dim meta line.
- **FR-026**: The run-level output of `loupe list` MUST NOT change. It presents runs, not findings.

### Verification

- **FR-027**: How GitHub actually renders the severity word inside a `<summary>`, and whether the four words in a column make reviewers pick the same word for the same defect, MUST be recorded as unverified in `specs/001-loupe-v1/validation.md`, not claimed.

### Key Entities

- **`severity`**: unchanged as data — an optional, reviewer-reported word from a fixed four. What changes is its reach: it now decides order everywhere and appears on both top-level surfaces.
- **The severity rank**: the four words as an ordered list, plus the position an unrated value takes after them. One definition, shared by validation, composition and every terminal surface.
- **`blocking`**: unchanged. An independent axis that decides section placement and carries its own chip everywhere.

## Success Criteria *(mandatory)*

- **SC-001**: A reader of a collapsed review learns which findings are the bad ones without opening any of them.
- **SC-002**: On every surface, the first finding a reader meets is the most severe one in the first section that has any — which is `Blocking` whenever anything blocks. Severity ranks findings within a section; it never moves one between sections.
- **SC-003**: A draft whose findings carry no severity and no blocking flag produces a review byte-identical to what it produced before this change.
- **SC-004**: The four words and their order have one definition. Reordering them is a one-line change; adding one is two, and the suite MUST name the second line rather than let a word ship with no color.
- **SC-005**: The severity badge survives `NO_COLOR` and the ASCII tier with its rank intact, carried by the word and the ordering.
- **SC-006**: A human decides findings in the same order the published review presents them.
- **SC-007**: All automated repository checks pass after the final edit.

## Assumptions

- Severity is the only rank. `impact` is free prose about what goes wrong and under what input, not a second scale; `confidence` and `verified` answer different questions and already have chips; `blocking` is an independent axis with a chip on every surface. The badge encodes severity and nothing else.
- Unrated sorting last is not a judgment that unrated findings matter least. It is the only placement that does not invent a value the reviewer withheld.
- Four words need four colors. A ramp whose middle two look alike puts the burden back on reading the word, which is what the color was there to save.
- The reordering within `Blocking` is worth the label-grouping property it costs. A reader who wants findings grouped by label has the label sections; a reader in `Blocking` wants the worst thing first.
- Whether the words in a column actually change how reviewers pick needs reviews landing on real pull requests over time. It is not claimed here.
