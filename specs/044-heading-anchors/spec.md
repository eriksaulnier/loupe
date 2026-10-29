# Feature Specification: Anchor a finding to a Markdown heading

**Feature Branch**: `044-heading-anchors`

**Created**: 2026-10-28

**Status**: Draft

**Input**: The owner's board draft, "Draft spec: anchor a finding to a Markdown heading": a finding on a spec or doc pull request is often about a whole section, so an agent SHOULD be able to anchor it with `{"path": "plan.md", "heading": "Rollout"}` instead of picking a line range by hand.

The idea comes from sift, another review tool, which anchors a finding to a Markdown heading in a whole file. loupe comments on a diff, and GitHub accepts a line comment only on lines inside the diff's hunks, and a range only inside one hunk (`docs/github-facts.md`, Inline review comments). So a heading anchor here is an input form, not a new kind of location. `loupe add` resolves it to a line or a range that `internal/diff` already accepts, and the draft keeps the heading next to that range.

The board draft says the anchor resolves "against `pr.diff`, the same way line anchors do". That is not enough on its own. With three lines of context, `pr.diff` rarely holds a section's heading or the next heading, and it never shows whether a `#` line sits inside a fence. Section bounds need the file at the captured head, so capture stores a heading outline of each changed Markdown file, and `add` resolves against `pr.diff` and that outline (FR-004).

## Clarifications

### Session 2026-10-28

- Q: Where does `add` get the section bounds, since `pr.diff` alone does not hold them? → A: Capture writes an optional run file holding a heading outline (heading text, level, line) of each changed `.md`, `.markdown` and `.mdx` file at the head. `add` resolves heading anchors against it and stays local to the run.
- Q: What does a heading anchor resolve to when the section's changes fall in more than one hunk? → A: `add` refuses with `location`, and the fix lists each hunk's resolved line range, so the agent files one line-anchored finding per hunk. The published comment does not show the heading, so a first-hunk fallback would publish a fragment that looks like the whole finding. Principle VI prefers a refusal to a recovery path.
- Q: Does the published comment format change? → A: No. `docs/comment-format.md` is untouched. The heading lives in the draft and the review interface only.
- Q: How is a range that starts on a context line confirmed, since `docs/github-facts.md` does not record it on its own? → A: The implementation runs one live probe on `eriksaulnier/loupe-probe#1`, with `--inline all` and a range that starts on a context line, and records the result in `docs/github-facts.md`. The owner approved that one probe.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Anchor a finding to a section (Priority: P1)

A review agent reads a spec pull request and finds that the Rollout section of `plan.md` never says how to roll back. It files `{"title": …, "body": …, "location": {"path": "specs/007-x/plan.md", "heading": "Rollout"}}`. loupe finds the section, picks the lines of it that the diff shows, and stores the finding with that range and the heading. The agent writes no line numbers.

**Why this priority**: this is the feature. Today the agent counts lines in a file it read whole, maps them onto the diff by hand, and gets a `location` refusal or a fragment of the section.

**Independent Test**: capture a pull request that edits one section of a Markdown file in one hunk, add a finding with a heading anchor, and read the stored location with `loupe show --json`. It holds the heading and the hunk's lines that fall inside the section, and publishing it creates the review with no 422.

**Acceptance Scenarios**:

1. **Given** a section whose changes are in one hunk, **When** the agent adds a finding anchored to its heading, **Then** the finding is stored with the heading and a range from the first to the last line of that hunk inside the section.
2. **Given** a section with exactly one line in the diff, **When** the agent anchors to it, **Then** the stored location is that single line.
3. **Given** a heading anchor stored by `add`, **When** the human accepts it and publishes with `--inline all`, **Then** GitHub accepts the inline comment on the stored range.
4. **Given** a finding stored with a heading, **When** the agent edits its `location` to a line form, **Then** the heading is gone and the line form is stored, as for any location change.
5. **Given** the human-facing flags, **When** a human runs `loupe add --path plan.md --heading Rollout …`, **Then** the result is the same as the JSON form.

---

### User Story 2 - A refusal that names what to use (Priority: P1)

The agent misspells a heading, names one whose section the pull request does not touch, names one whose changes fall in two hunks, names one that appears twice in the file, or names a heading in a file that is not Markdown. loupe refuses with `location` and a fix the agent can act on in one retry: the headings it can use, or the line ranges to file instead.

**Why this priority**: an agent that cannot recover from the first refusal falls back to guessing lines, which is the problem this feature removes. Principle V requires every refusal to name the correction.

**Independent Test**: for each refusal below, send the input, read `error.fix` and `error.details`, apply the fix, and get stored findings on the second call.

**Acceptance Scenarios**:

1. **Given** a heading text that no heading in the file matches, **When** the agent adds the finding, **Then** loupe refuses with `location`, and the fix lists, as written, every heading in that file whose section holds a change.
2. **Given** a heading whose section holds no change, **When** the agent adds the finding, **Then** loupe refuses with `location`, the message says the section has no changes in the diff, and the fix lists the headings that do, or names a general finding when none does.
3. **Given** a section whose changes fall in two hunks, **When** the agent anchors to its heading, **Then** loupe refuses with `location`, and the fix lists the section's range in each hunk as a line location, so the agent files one line-anchored finding per hunk.
4. **Given** a heading text that matches two headings in one file, **When** the agent adds the finding, **Then** loupe refuses with `location`, and the fix gives each match's line ranges, so the agent can file a line location instead.
5. **Given** a heading anchor on a path that is not a Markdown file, **When** the agent adds the finding, **Then** loupe refuses with `location`, and the fix names the line form.
6. **Given** a run captured by a loupe from before this feature, **When** the agent adds a heading anchor, **Then** loupe refuses with `location`, and the fix names the line form.
7. **Given** a batch with one bad heading anchor among good findings, **When** the agent adds it, **Then** nothing is stored, and `details.entries` names the bad entry, as for every batch refusal.

---

### User Story 3 - The human sees which section a finding is about (Priority: P2)

The human opens `loupe review`. A finding anchored to a heading shows the section's heading on its location line, next to the range it resolved to, and the diff under it shows that range as it does for any location. The human can see what the agent meant and what GitHub will show, and judge whether the range fits.

**Why this priority**: the human decides each finding (Principle II). A range alone does not say that the agent meant the whole section. The published review shows only the range, so the review interface is the one place the human sees the heading.

**Independent Test**: open review on a draft with a heading-anchored finding and read its detail, then open the whole-file diff from it.

**Acceptance Scenarios**:

1. **Given** a finding with a heading, **When** the human opens its detail, **Then** the location line shows the range and the heading's text.
2. **Given** the same finding, **When** the human opens the whole-file diff, **Then** the finding's mark sits on the range's last line, as for a line location.
3. **Given** a finding without a heading, **When** the human opens its detail, **Then** the detail is unchanged from today.

### Edge Cases

- The section runs from its heading to the line before the next heading of the same or a higher level, or to the end of the file. A deeper heading inside it does not end it.
- Lines before the first heading belong to no section.
- A `#` line inside a fenced code block is not a heading. A `#` line inside YAML front matter at the top of the file is not a heading.
- A heading inside a block quote or a list item is not a heading for anchors, and does not end a section.
- A setext heading (a line underlined with `===` or `---`) is out of scope. It is not a heading for anchors, and it does not end a section.
- A section whose only diff lines are context, with no added or deleted line inside it, holds no change and refuses as in User Story 2.
- A section that one hunk changes and a second hunk touches only with context holds changes in one hunk, and resolves to that hunk.
- A deletion belongs to the section that holds the new-side line right before it. A deletion before a file's first new-side line belongs to no section.
- The heading line itself is in a hunk when the pull request changed it or a line near it. Then the range starts on the heading.
- A pull request that renames a heading: the anchor matches the new text, since the outline is read from the head.
- A file deleted by the pull request, or a binary file, has no head text and no outline. A heading anchor on it refuses with `location`, and the fix names a `LEFT` line location or a general finding.
- A pull request that changes no Markdown file still gets an outline file, with no files in it, so a missing outline means only a run from before this feature.
- A heading anchor with `"side": "LEFT"`, or with `line` or `startLine`, refuses with `input`.
- An empty `heading` refuses with `input`.
- An agent that writes the heading with its `#` marks, `"## Rollout"`, matches nothing and gets the list of headings as written.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A finding's input `location` MUST accept `{"path", "heading"}` as an alternative to `{"path", "line", "side", "startLine"}`, on `loupe add` and `loupe edit`. The flag equivalent MUST be `--heading <text>` with `--path`.
- **FR-002**: Every input valid today MUST stay valid and MUST store what it stores today. A heading anchor MUST NOT carry `line` or `startLine`, and MUST NOT carry a `side` other than `RIGHT`. Such input MUST refuse with `input`.
- **FR-003**: A heading anchor MUST apply only to a path ending in `.md`, `.markdown` or `.mdx`, in any letter case. Any other path MUST refuse with `location`, and the fix MUST name the line form.
- **FR-004**: `loupe capture` MUST write an optional run file holding a heading outline of each `.md`, `.markdown` and `.mdx` file the diff changes, read at the captured head: each heading's text, level and line. It MUST write the file on every capture, with no files in it when the diff changes no Markdown file. It MUST NOT outline a file deleted by the pull request or a binary file. A failure to read a changed Markdown file at the head MUST fail the capture as any other git failure does, since capture fetched that head itself. Reading the head MUST stay within Principle IV: no working file, index, branch or ref changes.
- **FR-005**: `add` and `edit` MUST resolve a heading anchor against `pr.diff` and the outline only. They MUST NOT read the clone, so that a run carried to another job resolves a heading anchor as it does a line location.
- **FR-006**: Only ATX headings at the top level of the document MUST count: up to three spaces of indent, one to six `#`, then a space, a tab or the end of the line. Lines inside a fenced code block, an indented code block, YAML front matter at the top of the file, a block quote or a list item MUST NOT count.
- **FR-007**: A heading's text MUST be the source between its opening `#` run and its optional closing `#` run, with leading and trailing whitespace trimmed and each run of inner whitespace read as one space. Inline Markdown MUST stay as written, so `` `loupe add` flags `` matches the heading that reads that way in the file. A match MUST be exact and case-sensitive after that normalization, applied to both sides.
- **FR-008**: A section MUST hold a change in a hunk when that hunk adds a line inside it, or deletes a line whose preceding new-side line is inside it.
- **FR-009**: A heading anchor whose section holds changes in exactly one hunk MUST resolve, at `add` or `edit`, to the new-side lines of that hunk that fall inside the section: `startLine` the first, `line` the last, and a single line when they are the same. The resolved location MUST pass the same check a line location passes today.
- **FR-010**: A heading anchor whose section holds changes in more than one hunk MUST refuse with `location`. The message MUST say how many hunks change the section. The fix MUST list the section's resolved range in each such hunk, in file order, as line locations, and say to file one line-anchored finding per hunk. `details.ranges` MUST list the same ranges as `{startLine, line}`.
- **FR-011**: A heading text that matches no heading in the file MUST refuse with `location`. The fix MUST read `use one of: ` followed by every heading of that file whose section holds a change, each as written, in file order. `details.headings` MUST list the same headings as `{heading, level, line}`. When no section of the file holds a change, the fix MUST name a line location or a general finding instead.
- **FR-012**: A heading whose section holds no change MUST refuse with `location`. The message MUST say that the section has no changes in the diff. The fix and `details.headings` MUST be those of FR-011.
- **FR-013**: A heading text that matches more than one heading in the file MUST refuse with `location`. The message MUST give each match's line. The fix MUST give, as line locations, the resolved ranges of each match whose section holds a change, one per hunk that changes it. `details.matches` MUST list each match as `{level, line, ranges}`, where `ranges` lists `{startLine, line}` and is empty for a match with no change.
- **FR-014**: A run with no outline file, because a loupe from before this feature captured it, MUST refuse a heading anchor with `location`. The message MUST say the run was captured before heading anchors, and the fix MUST name the line form. It MUST NOT refuse with `record`, since the run is not damaged.
- **FR-015**: The draft MUST store the heading's text, as written, in the finding's location beside the resolved `path`, `side`, `line` and `startLine`. Any edit that sets `location` MUST replace the heading with the new location's, or remove it for a line form. `null` and `--clear-location` MUST remove it with the location.
- **FR-016**: Every reader of a location outside the draft and the review interface, including publish, the inline comment, the review body, the findings record and the receipt, MUST use the resolved range and MUST NOT carry the heading. The published comment format in `docs/comment-format.md` MUST NOT change, and the findings record's `v=` MUST NOT change.
- **FR-017**: The review interface MUST show a finding's heading on its detail's location line, after the range. The findings list and the hunk view under the detail MUST stay as they are. The whole-file diff MUST mark the range's last line, as for a line location. The review interface MUST NOT read the head file or show lines outside the diff.
- **FR-018**: `loupe add --help` and `loupe edit --help` MUST describe the heading form and its refusals, so the workflow stays completable from `--help` alone (Principle I). The `human-review` skill MUST describe the heading form beside the line form.
- **FR-019**: Heading parsing MUST NOT add a runtime dependency. goldmark is already a direct dependency (`internal/markdown`, spec 025), and the plan MUST either use it or justify a line scanner.
- **FR-020**: `contracts/cli.md` MUST gain the heading form under `add` and `edit`, the `--heading` flag, the outline in what `capture` stores, and the widened meaning of `location` in the error table: a path or line not in the stored diff, a range that spans hunks, or a heading that is missing, repeated, holds no change or holds changes in more than one hunk.
- **FR-021**: `docs/versioning.md` MUST list the outline file at `schema` 1, strict like the other run files, and the draft's schema bump for the stored heading. Both MUST follow its rules, and the schema test MUST pin both field sets.
- **FR-022**: The implementation MUST include one live probe on `eriksaulnier/loupe-probe#1`: a review published with `--inline all` whose inline comment's range starts on a context line. It MUST record the result in `docs/github-facts.md` under Inline review comments, with the date, the review id and GitHub's response. The owner approved this one probe on 2026-10-28. No other live run is approved. A tasks list for this spec MUST carry the probe as its own task, after the publish path works against the fake.

### Key Entities

- **Heading anchor**: the input form `{path, heading}`. It never reaches GitHub. `add` and `edit` turn it into a range.
- **Heading outline**: the run file capture writes. For each changed Markdown file at the head it holds each heading's text, level and line, which is enough to find any section's bounds.
- **Section**: a heading and the lines under it, up to the next heading of the same or a higher level. Its bounds come from the outline.
- **Resolved location**: the range one hunk shows of a section. It is an ordinary location, and every reader outside the draft and the review interface uses it as one.
- **Stored heading**: the heading's text in the draft's location, kept so that the human sees which section the agent meant.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent anchors a finding to any section of a captured Markdown file whose changes fall in one hunk in one `loupe add` call, with no line number in its input.
- **SC-002**: Every heading-anchored finding that `add` stores passes the existing location check, so none can make GitHub refuse the review with 422.
- **SC-003**: For each refusal in User Story 2, applying the fix it returns stores the findings on the next call.
- **SC-004**: Every existing golden under `testdata/golden/cli` and every published-format test passes unchanged.
- **SC-005**: The human sees the heading of every heading-anchored finding in its detail, in every color tier.
- **SC-006**: `docs/github-facts.md` records the live probe's result for a range that starts on a context line.

## Scope

In scope:

- The `{path, heading}` input form and the `--heading` flag on `add` and `edit`.
- A new optional run file written by `capture`: the heading outline of each changed Markdown file at the head.
- The stored heading in the draft, with its schema bump.
- The `location` refusals of FR-010 to FR-014.
- The heading on the review interface's detail.
- The `--help` text, the `human-review` skill, `contracts/cli.md` and `docs/versioning.md`.
- One live probe, recorded in `docs/github-facts.md`.

Out of scope:

- Any change to the published comment format or the findings record.
- Setext headings, headings in block quotes or lists, and HTML headings.
- Heading anchors on the old side of a file.
- A file-level location, and a fallback to one for a section with no change.
- A form that tells duplicate headings apart by level or parent.
- Showing lines of a section that are outside the diff.

## Assumptions

- Heading anchors cover the new file only. A finding about a section a pull request removed uses a `LEFT` line location.
- Duplicate headings refuse instead of taking a parent heading or a level. No heading repeats within any of the 126 Markdown files directly under `specs/*/` and `docs/` on 2026-10-28, and a disambiguating form can follow when a pull request shows a need (Principle VI).
- A section that holds no change refuses instead of becoming a file-level comment. loupe has no file-level location shape, and the review API's behavior for one inside `comments[]` is not recorded in `docs/github-facts.md`. A general finding already covers a comment about a file as a whole.
- The range clips to the diff because GitHub accepts nothing else. The published review shows only that range, so the agent's title SHOULD name the section when the finding is about all of it.
- GitHub accepts a range that starts or ends on a context line, since such a line is in the diff on the new side. The recorded fact covers lines in the diff and ranges inside one hunk, not a range on context lines on its own. loupe's line ranges already rely on it, and FR-022 confirms it.
- Long heading lists go in the fix, as the paths of a file missing from the diff already do. The headings whose sections hold a change are few, which keeps the fix short.
- The outline is small next to `pr.diff`, since it holds one entry per heading of each changed Markdown file.
