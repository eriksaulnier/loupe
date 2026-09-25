# The published comment format

loupe publishes one GitHub review per round, or with `--sticky` keeps one review per pull request and edits it each round (see Sticky reviews). Where this document and `specs/001-loupe-v1/spec.md` disagree, the spec wins and the conflict is a bug here.

- This is the contract for what that review contains: the finding vocabulary, how the body is composed, and what authored Markdown is accepted.
- It was iterated against real GitHub rendering; the observations behind its rules are in [github-facts.md](github-facts.md).

## Vocabulary

Findings carry a reduced [Conventional Comments](https://conventionalcomments.org) vocabulary. The label says what kind of remark it is; an independent flag says whether it blocks.

| Field | Values | Meaning |
| :--- | :--- | :--- |
| `label` | `issue`, `suggestion`, `question` | The kind of remark. Any other value is accepted and preserved verbatim |
| `blocking` | boolean, absent means false | Whether the PR should not merge until this is addressed |
| `confidence` | `high`, `medium`, `low` | Reviewer-reported. Optional |
| `severity` | `critical`, `major`, `minor`, `trivial` | How bad the consequence is, per the table below. Reviewer-reported. Optional. Never mapped onto a label |
| `verified` | `reproduced`, `plausible` | Whether the reviewer ran the failure or reasoned to it. Optional |
| `impact` | Markdown | What goes wrong and under what input. Optional, allowlist-checked like `body` |
| `references` | list of `http` or `https` URLs | What the reviewer looked at. At most six, never fetched. Optional |

- **Label and blocking are independent.** A small required mechanical change is a blocking `suggestion`, not an `issue`. A blocking `question` is valid: a question whose answer determines whether the change is correct does block.
- **Unknown labels are never refused.** An unrecognized label renders verbatim in the finding's own summary line (`<b>perf-nit</b>`), and a nonblocking one leads its row with the `⚪` dot and counts in the `⚪ other` chip. A finding with an unknown label and `blocking` set is placed in `Must fix` and counted in the `⛔` chip, like every other blocking finding, while still counting in `loupe-meta`'s census under `other`, the group of every unknown label. `other` is a bucket, never a rewrite of the label.
- A label that is absent or empty is treated as no label: the finding counts as `other` and its summary line carries no label word. The same applies to an empty `confidence`, `severity` or `verified`, which render nothing.
- **Confidence, severity and verified MUST NOT be computed, defaulted or inferred.** Each is a value the reviewer reports or omits. The reason for a confidence level belongs in the body.
- **Severity is never mapped onto a label.** It follows the dot on the summary line and orders findings within a section, and never decides which section one is placed in.
- **Verified is not confidence.** Confidence is how sure the reviewer is; verified is whether it ran or observed the failure (`reproduced`) or reasoned to it (`plausible`).
- **A stored severity outside the enum still renders.** Runs captured before the enum hold free text. It cannot reach the summary line, which interpolates the severity word raw, so it renders on the meta line inside a code span, where it stays inert Markdown. Only a new or changed value is refused.

| `severity` | What goes wrong if it ships |
| :--- | :--- |
| `critical` | Data loss, a security hole, an outage |
| `major` | A real defect on a normal path |
| `minor` | An edge case, or a cost paid later |
| `trivial` | Cosmetic. Naming, style, a preference |

**The rows are ordered, and a finding takes the highest row it satisfies.** They name different kinds of consequence, so more than one MAY fit; the order decides. Data loss confined to an edge case is `critical`, not `minor`, and an outage on a normal path is `critical`, not `major`.

**Severity says how bad the consequence is; `blocking` says whether merge waits.** The two correlate and MAY diverge: a `critical` finding in code the release does not reach need not block, and a `trivial` one MAY block when the human says so.

## Body composition

- The body holds at most two finding sections, in fixed order: `### Must fix`, then `### Worth a look`. An empty section is omitted. Neither heading carries a dot: every row leads with its own.
- **Every published finding has exactly one home in the body.** A blocking finding lives in `Must fix` and nowhere else, regardless of its label. Every other finding lives in `Worth a look`. `--inline` decides only what additionally anchors to a line; it never changes what the body contains.
- **Both sections sort by severity first**: `critical`, `major`, `minor`, `trivial`, then every finding whose severity is absent, empty or free text captured before the enum. An unrated finding sorts last rather than as `trivial`, because severity is reviewer-reported and placing it among the rated ones would infer the value the reviewer withheld.
- Severity is followed by label group (`issue`, `suggestion`, `question`, then every other or empty label as one group), then by finding id. Severity outranks the label group, so two findings with the same label MAY be separated by a third between them; each section exists to be read worst first.
- Two sections, not one per label, because real reviews hold zero to ten findings and usually one. `blocking` is the one axis that changes what the author must do before merge, and each row carries its own label (`specs/020-review-format`).

````markdown
`⛔ 1 blocking` `⚪ 1 other`

The retry path can publish twice and the digest is not verified on reconcile.
Worth fixing before this merges; the rest reads fine to me.

---

### Must fix

<details>
<summary>⛔ <b>issue</b> <picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major-dark.svg"><img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major.svg" alt="MAJOR" height="16" align="absmiddle"></picture>: Retry loop can double-publish a review</summary>

> [`internal/publish/publish.go:88`](https://github.com/o/r/pull/7/files#diff-8f3c…R88)\
> **Confidence:** high\
> **Severity:** major\
> **Verified:** reproduced

**Impact:** A 502 on the first send leaves two reviews on the pull request, and the receipt records only one.

Reproduced against the recorded fixture. The catch re-enters the loop after a request
that may already have succeeded, so a 502 produces two reviews.

**Suggested fix:** Return the original write error.

**References:** [github.com/o/r/issues/12](<https://github.com/o/r/issues/12>)

</details>

---

### Worth a look

<details>
<summary>⚪ <b>perf-nit</b> <picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/minor-dark.svg"><img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/minor.svg" alt="MINOR" height="16" align="absmiddle"></picture>: Redundant sort on every read</summary>

> [`internal/draft/store.go:10–14`](https://github.com/o/r/pull/7/files#diff-5610…R10-R14)\
> **Severity:** minor

…

</details>

---

reviewed [`d23632e`](https://github.com/o/r/commit/d23632e5b0a1c9f4e7d2b8a6c3f1e0d9b7a5c4e2)

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-findings v=1 sha256=<sha256> <data> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 excluded=1 withdrawn=1 reinstated=0 regraded=1 -->
````

### Opening

The chips row leads the body and the opening prose follows it. With no findings to count, the row is one `🟢 no findings` pill. The chips lead because `⛔ N blocking` is the one fact an author most needs, and a long unattended summary would otherwise push it down. The body MUST NOT open with a callout. GitHub's review header already shows the event (approved, changes requested, commented), and the leading `⛔` chip already states the blocking count, so a callout would only repeat one or the other.

**Who writes the opening prose depends on who published.** An attended review carries a message the human typed at the publish confirmation, reading the body as they wrote it; the draft's summary is not published on that path and orients the human while they sort findings instead. An unattended review carries the draft's summary, because no human is there to type anything, and its footer already ends in ` · unattended` so a reader knows the prose was not read by a person before it appeared.

The prose is optional in both modes. When there is none the body opens on the chips row, and nothing is rendered in its place. A review with neither prose nor a finding is refused rather than posted.

### Chips

- One inline code span per non-zero count, zeros omitted, pluralized except `other` and `blocking`.
- **A review with no findings still opens on the row**, as the one chip `` `🟢 no findings` ``, so a reader sees at a glance that nothing was found. A body published before this chip opens on its prose instead, and sticky read-back accepts both.
- Each is led by a dot: ⛔ blocking, 🟡 issue, 🟣 suggestion, 🔵 question, ⚪ other, and 🟢 alone when there is nothing to count.
- Counts are derived from the final published findings at publish time, never from the summary prose.
- **One chip per row dot below, in the order ⛔, 🟡, 🟣, 🔵, ⚪.** The chips row is a key to the dots that lead the rows. Blocking leads the row and is counted only there: a blocking row leads with `⛔` whatever its label, so a blocking issue is `⛔ 1 blocking`, never also `🟡 1 issue`. Every other chip counts the nonblocking findings of its label group.
- A reader who scans the row and finds no chip for a dot will find no row with that dot either.
- **The chips row is text.** It is one code span per count, and no chip is an image.
- **The severity pill is the one image loupe emits.** See The summary line for how it is hosted and drawn.

### The summary line

One row rule serves the body's `<summary>` and an inline comment's first line. The two differ only by the inline escaping described below:

```
<summary>⛔ <b>issue</b> <picture>…MAJOR…</picture>: Retry loop can double-publish a review</summary>
<summary>🟣 <b>suggestion</b>: Digest is not verified on reconcile</summary>
<summary>⚪ Redundant sort on every read</summary>
```

| Part | Rendering | Absent when |
| :--- | :--- | :--- |
| Dot | `⛔` for a blocking finding, whatever its label. Otherwise the label group's dot: 🟡 issue, 🟣 suggestion, 🔵 question, ⚪ every other or empty label | Never |
| Label | `<b>` around the label, verbatim apart from escaping, known or unknown | The label is absent or empty |
| Severity | The pill below | Severity is absent, empty or free text captured before the enum |
| Colon | `:` directly after the last of label and severity | Both are absent |
| Title | The rules below | Never |

- **The row has no location.** The meta block's first line is the full location, linked to the files view, and a click on the row opens onto it.
- **The row says `blocking` nowhere.** The `⛔` dot says it on every row, body and inline, and the `Must fix` heading says it for the section. The label stays either way: the dot says that a finding blocks, not what kind of remark it is.
- **Only the four enum words reach the row.** A stored severity that is not one of them gets no pill and appears only on the meta line, where its code span makes it inert. An enum word appears in both places: as the pill on the row, and as text on the meta line (see The meta block).
- `<b>`, `<code>` and the pill's `<picture>`, `<source>` and `<img>` are the only tags loupe emits inside a `<summary>`. GitHub parses no Markdown inside a `<summary>`, so loupe applies two CommonMark rules to the title itself. Each backtick code span becomes `<code>`. A backslash before ASCII punctuation outside a span shows the punctuation alone, so an escaped backtick opens no span. The rest of the title is plain text: it is HTML-escaped, and emphasis and links show literally.
- **The label is HTML-escaped inside its `<b>`**, since a tag in a `<summary>` is raw HTML. An inline comment's first line is a Markdown paragraph, so there the title and the label are also backslash-escaped, and they render as the same text in both places.

#### The severity pill

The severity word is drawn as a colored pill, an SVG image. This is the one exception to text-only output, and it holds to these rules, each from an observation in github-facts.md:

```
<picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major-dark.svg"><img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major.svg" alt="MAJOR" height="16" align="absmiddle"></picture>
```

- **It MUST be a `<picture>`, never a bare `<img>`.** GitHub wraps a bare `<img>` in a link to the image, which takes a click on the `<summary>` away from the disclosure. The dark `<source>` switches the pill with the reader's theme.
- **`alt` MUST be the uppercase word.** A reader whose client does not load the image, such as an email client, reads the same word the pill shows.
- **`height="16"` and `align="absmiddle"`, with the pill drawn in the top 14 of the image's 16 pixels and its text at 10.** `absmiddle` centers the image, so the 2 transparent pixels below raise the pill by 1, which centers it on the text, and the row stays the height of a text row. With no padding the pill sat 1 to 2 pixels low; with 3 or 4 it sat high. Other alignments sat 2 to 3 pixels off. The image also carries 3 transparent pixels to the right of the pill, so the colon after it does not touch it; a space in the Markdown instead would leave `MAJOR :` wherever the image does not load.
- **The files live at `assets/review/v1/` in `eriksaulnier/loupe` and are linked through `main`.** Every review published hotlinks them forever, so a file under `v1` MUST NOT change once it is on `main`; a redesign adds `v2`. A test pins each file's hash. Until a version reaches `main`, its pills show as their alt text.
- **Terminal surfaces show the word, not the HTML.** The publish confirmation shows the body as raw Markdown, and it replaces each pill with its word there, as it replaces the findings record with a note (see Markers), in the `<summary>` lines outside fences and on each inline comment's first line only, so authored text keeps its bytes. The payload view shows the exact bytes.
- **This is a network dependency.** Every reader's client loads the pills through GitHub's camo proxy. When it cannot, the alt word stands in.

### The meta block

A blockquote at the top of the disclosure body, present only when at least one part exists.

- One part per line, each line separated from the next by a backslash hard break: the location first, then confidence, severity and verified, in that order. An absent part leaves no line.
- A finding with no location starts at its first present part; one with only a location has that line alone.

| Part | Rendering | Example |
| :--- | :--- | :--- |
| Location | Review body only. A Markdown code span, side included inside the span, as the text of a link to the PR's files view | `` [`src/a.go:88`](…/pull/7/files#diff-<sha256 of the path>R88) `` |
| Confidence | the bold label `**Confidence:**` then the escaped level word | `**Confidence:** high` |
| Severity | the bold label `**Severity:**` then the enum word as text, or a value outside the enum in a code span | `**Severity:** major`, `` **Severity:** `P2` `` |
| Verified | the bold label `**Verified:**` then the escaped word | `**Verified:** reproduced` |

- **Location is rendered in full**, path and range, because with `--inline none` this is the only place it survives. `src/a.go:88` for a single line, `src/a.go:10–14` for a range. **The side is shown only when it is `LEFT`**, appended as ` (LEFT)`.
- **In the review body the location links to the PR's files view**, not to a review thread: GitHub assigns no inline-comment URL until the review is submitted. The anchor is `#diff-` followed by the SHA-256 hex of the path, then `L` or `R` for the side and the line, with `R21-R24` for a range. This is observed behavior, see github-facts.md.
- **An inline comment carries no location line**: the comment already sits on the line, so its meta block starts at confidence.
- **A code span MUST be delimited by a backtick run longer than any run inside its content**, padded with a space when the content starts or ends with a backtick. This is the CommonMark rule and it applies to every generated code span, including the footer's.
- **Code-span contents MUST NOT be HTML-escaped.** A code span is already inert, and GitHub shows character references inside one literally. Escaping is for text interpolated outside a span.
- **Every generated inline field MUST be collapsed to a single line before interpolation**, with runs of whitespace becoming one space. A newline in a title breaks the summary line out of its tag; a newline in `severity` would end the meta block's blockquote and leave the rest of the line as body text.
- Confidence, severity and verified render only when the reviewer supplied them. None of the three decides section placement.
- **An enum severity is repeated here as text.** The row carries it only as a pill image, and anything that reads the review as plain text drops images: a terminal Markdown renderer such as `gh pr view` shows the row with no severity at all (github-facts.md). So the meta line, which such a reader shows right under the row, carries the word.

### Field labels

Impact, the suggested fix and the references each render under a bold label. **A value of one line follows its label on the same line**, as `**Impact:** …`. A longer value goes below a bold label on a line of its own, as `**Impact**`, because a list, a quote or a fence can only open at the start of a line. A one-line value that starts like one, such as `- Return the error`, shows that punctuation as text after the label. A lone carriage return ends a line as `\n` does, in this rule and in the allowlist.

### Impact

- Rendered first after the meta block, before the body, as Markdown with trailing newlines trimmed. A reader deciding whether to act wants the consequence before the reasoning.
- It passes the same allowlist as `body` and is rechecked at composition, so it cannot unbalance the disclosure.

### Suggested fix

- Rendered after the body, as Markdown with trailing newlines trimmed. Code in it MUST be fenced by whoever writes it; bare code renders as prose.
- It passes the same allowlist as `body` at write time, when `add` writes it or an `edit` changes it; an edit that leaves it unchanged does not recheck it. A stored fix that fails the allowlist, written before this rule, is instead rendered inside a fence whose length is `max(3, longest backtick run in the content + 1)` under a `**Suggested fix**` line, so it stays inert and still publishes.
- It is prose or code the human reads, never a GitHub `suggestion` fence: an applicable suggestion MUST contain the exact replacement lines for the anchored range, and a suggested fix holds prose. Wrapping prose in a suggestion fence produces a broken apply button.

### References

- Rendered last in the disclosure, in input order. One reference follows its label as `**References:** [text](<url>)`. Several go below a `**References**` line, one `- [text](<url>)` bullet each.
- The link text is the host and path, without scheme, query or fragment and with trailing slashes trimmed. When that is over 50 characters and the path has more than one segment, it is shortened to `host/…/last-segment`. In the text, `&` is written as `&amp;`, and each of ``\ ` * _ [ ] ~ $`` is backslash-escaped, `$` because GitHub renders math between two of them. GitHub's emoji shortcodes, such as `:tada:` in a path, still render as emoji; escaping cannot stop that.
- The destination is inside `<…>`, so parentheses in the URL cannot end the link. CommonMark still decodes backslash escapes and character references there, so `\` is doubled and `&` is written as `&amp;`, and the link goes to the URL as given.
- Each entry MUST be an `http` or `https` URL with a host, at most 200 bytes, with no whitespace, control or format character (bidi overrides included), `<`, `>` or backtick, and there are at most six; `loupe add` refuses anything else and composition checks again. Those characters are what would end or break the link. loupe never fetches a reference.

### Dividers

- A `---` divider MUST be preceded by a blank line.
- GitHub renders a `---` that immediately follows `</details>` as literal text.

### Footer

```text
reviewed `SHA`
reviewed `SHA` · via `NAME VERSION`
reviewed `SHA` · via `NAME VERSION` · unattended
reviewed `SHA` · unattended
reviewed `SHA` · `MODEL`
reviewed `SHA` · `MODEL` · unattended
reviewed `SHA` · via `NAME VERSION` · `MODEL`
reviewed `SHA` · via `NAME VERSION` · `MODEL` · unattended
```

- The footer answers only what a reader who never installed loupe can act on: which commit, who reviewed it, which model produced the findings, and whether anybody read it before it posted (`specs/009-review-footer`, `specs/023-model-footer`).
- `SHA` is the abbreviated captured head commit, a generated code span subject to the backtick rule, linked to that commit on GitHub: ``[`SHA`](https://github.com/OWNER/REPO/commit/FULLSHA)``. The forms above show it bare for reading; the link is always there when the pull request's repository is known.
- **A sticky round with an earlier round** adds `` · [changes since round N](https://github.com/OWNER/REPO/compare/PREV...FULLSHA)`` right after the SHA, where `N` and `PREV` are the newest earlier round's number and commit, in full when that round's footer links it. They are taken before the length limit drops any round, so the link stays when the round it names was dropped. An edit sends no notification and keeps the review's first timestamp, so this link is how a reader sees what changed between rounds. A plain review, and a sticky review's first round, have no such segment. Read-back accepts a footer with or without these links, since earlier releases wrote the SHA bare.
- The ` · via ` suffix MUST appear only when capture recorded a source, and is otherwise absent, leaving the first form byte for byte. `NAME VERSION` is that source with its `@` rendered as a space, or `NAME` alone when it has no version, in a generated code span.
- The `` · `MODEL` `` segment MUST appear only when capture recorded a model, and is otherwise absent, leaving the forms above it byte for byte. It follows ` · via ` when there is one, and otherwise the SHA. `MODEL` is the id exactly as capture recorded it, in a generated code span, with no display name and no label such as `by`: on an attended review the human is the author, and a model id names itself (`specs/023-model-footer`).
- **` · unattended` MUST come last**, after ` · via ` and the model when there are any, only on a review `loupe publish --unattended` sent (`specs/007-unattended-publish/spec.md` FR-016, amended by `specs/009-review-footer`). A review published without `--unattended` is unchanged byte for byte.
- Nothing else: no tool name, no round number, no run reference, no local paths, and no agent name beyond the source and the model capture was given, none of which a PR reader can resolve. Machine-readable provenance belongs in `loupe-meta`.

### Markers

Three HTML comments, all shipped in the payload and all visible in raw Markdown forever. None is private.

```text
<!-- loupe digest=<sha256 of the publishable draft> publication=<uuid> -->
<!-- loupe-findings v=1 sha256=<checksum> <data> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 excluded=1 withdrawn=1 reinstated=0 regraded=1 -->
```

- The first is the reconciliation marker: an unknown publication is resolved by finding a review whose body contains this exact comment. Its format MUST NOT change between versions that may need to reconcile each other's attempts.
- **The second is the findings record** (`specs/027-previous-from-github`). It lets the next round's capture read this round back from GitHub, so `loupe show --previous` works where no local receipt exists, such as a CI run that starts from an empty data root. It sits between the reconciliation marker and `loupe-meta`, on every review, attended or unattended, sticky or not. A body carries at most one.
  - `<data>` is standard base64 of raw DEFLATE of compact JSON: an array of `{"id", "title", "body", "location", "label", "blocking"}`, one per published finding in id order, the same fields and encoding as the receipt envelope's findings. `location` is `null` for a general finding, and otherwise `{"path", "side", "line"}` plus `"startLine"` for a range. A review with no findings carries `[]`. Base64 has no `-` or `>`, so no finding can close the comment.
  - `<checksum>` is the SHA-256 hex of the body with CRLF and lone CR read as LF and the record line removed, then a newline, then `<data>`. An edit on GitHub to any other byte of the body, or to the data, therefore stops the record from being read, and the next round reports that rather than list findings the review no longer shows.
  - It holds only findings the body shows, copied from the same findings the body is composed from. It never holds the draft's summary.
  - When the body would pass 65,536 characters with the record, collapsed earlier rounds are dropped first as for any sticky review. If the body is still too long, the line is `<!-- loupe-findings v=1 omitted=length -->`, so the next round can say why it has no previous round.
  - A reader that finds no record, an omission, more than one record, a `v=` other than `1`, a checksum mismatch or data that does not decode reports that there is no previous round and why. It never falls back to an older review.
  - **Terminal surfaces show a note, not the data.** The publish confirmation shows the body as raw Markdown, and it replaces the record there with `<!-- loupe-findings: a copy of the N findings above -->` (`the 1 finding above`, `no findings`), as it shows each pill as its word. The payload view shows the exact bytes.
- **`loupe-meta`'s per-label counts are a census, not the chips row.** They count every published finding, blocking ones included, so a review whose only issue blocks records `blocking=1 issues=1` while the visible chips show no issue. The chips are a key to the row dots a reader sees; the marker is an inventory of what the review held.
- **`excluded=`, `withdrawn=`, `reinstated=` and `regraded=` follow `other=` in that order on every review, attended or unattended, and count the whole draft, not only what it published** (`specs/021-human-gate-counts`). They are always present, so `excluded=0` means none and a missing key means a loupe older than them. `excluded=` counts the findings a person rejected: those with a current exclude decision, and those withdrawn where a human made the latest change to `included`. `withdrawn=` counts the findings the skill cut itself: withdrawn with no current exclude decision, where the agent made the latest change to `included` or nobody did. The census plus `excluded=` plus `withdrawn=` is every finding the round filed. `reinstated=` counts the findings a human put back after a withdrawal, and `regraded=` the findings whose label, blocking or severity a human changed. Those two are flags, not a partition: a finding counts at most once in each, and either MAY count a finding that also publishes or that `excluded=` or `withdrawn=` also counts.
- **The gate counts are self-reported and per round.** They read the draft's history, which records who made each change but not through which command, so `loupe edit --by human` and `loupe edit --exclude --by human` count exactly as the review screen does. `loupe add --by human` puts findings the human wrote into the filed total. `regraded=` counts an edit even when a later edit reverts it, and the review screen changes label and blocking but never severity, so a severity regrade came from `loupe edit`. The counts sit outside the digest, so `--retry-unknown` after a change outside the publishable set, such as excluding a withdrawn finding, MAY post different counts under the same digest. A later round is its own draft, and its counts never include an earlier round's.
- **`unattended=1` follows `round=` only on a review `loupe publish --unattended` sent**, directly before `src=` when both are present (`specs/007-unattended-publish/spec.md` FR-016). It is also how `loupe publish --unattended` counts a pull request's earlier rounds for `N` (FR-015), alongside the review's author ending `[bot]`.
- **`src=` follows `round=`, or `unattended=1` when present, only when capture recorded a source**, as given (`src=gadfly-review-pr@2.2.0`). It is never escaped, so capture refuses a source that does not match `^[a-z0-9][a-z0-9._-]*(@[0-9][0-9A-Za-z.+-]*)?$`, is over 64 characters, or contains `--`, and every command refuses a `target.json` carrying one. None of those can close the comment.
- **`model=` follows `src=`, or sits where `src=` would, only when capture recorded a model** with `loupe capture --model`, as given (`model=anthropic/claude-sonnet-5`). It is never escaped, so capture refuses a model that does not match `^[a-z0-9][a-z0-9._/:-]*$`, is over 64 characters, or contains `--`, and every command refuses a `target.json` carrying one. The footer shows the same id (see Footer), and this key is unchanged by it.
- **`sticky=K` is the last key, only on a sticky review**, where `K` counts every round published into it, dropped ones included (`specs/025-sticky-review`). A review without the key is not sticky. See Sticky reviews.
- New keys MAY be added to `loupe-meta`; existing keys MUST keep their meaning. **`round=` is the one recorded exception:** it carries `N`, this review's position among the pull request's publications (FR-042), where it once carried the run's round. Outside the shared-`N` case below, a pull request with no abandoned round reads identically under both, and `round=` still rises with each review a pull request publishes, so a reader that takes the highest as live picks the newest review. Since `specs/009-review-footer` the marker is the only place `N` appears, so the three rules that follow are a key for tooling and never something a reader of the review is asked to reconcile.
- **`N` counts publications, not captures.** It is 1 plus the number of the pull request's *other* rounds holding a receipt or an attempt, so a round captured and abandoned unpublished does not advance it, and it MAY be lower than the run's round. An attempt counts because its review may already be on GitHub, so no other round reuses its number while the attempt stands, at the cost of a skipped number when an unknown attempt never landed. Other rounds count rather than earlier ones, so an older round that publishes after a newer one still gets the higher number.
- **An unattended review counts differently.** `loupe publish --unattended` has no local round state to count, so its `N` is 1 plus the pull request's reviews by a `[bot]` author carrying a `loupe-meta` marker (`specs/007-unattended-publish/spec.md` FR-015). On a pull request reviewed both ways the two counters are disjoint: a human's review and a bot's MAY carry the same `N`, and a bot's `N` MAY be lower than the human `N` before it. The author and the ` · unattended` segment are what tell them apart.
- **Two reviews MAY share an `N`**, and every way needs the head force-pushed back to an older round's commit: `--retry-unknown` on that round after a newer round published, since the retry counts the newer receipt that already counted its attempt; two rounds at that commit publishing at once, since `N` is fixed before confirmation and each publish locks only its own run; or `--retry-unknown` on that round failing with a definite rejection, which deletes the attempt a newer round already counted, so the next round to publish reuses the newer round's number. This is accepted rather than refused.

## Inline modes

`loupe publish --action <action> --inline none|blocking|all`, default `none` (specs/026-inline-default-none).

| Mode | `comments[]` contains |
| :--- | :--- |
| `none` | nothing |
| `blocking` | every published, located, blocking finding |
| `all` | every published, located finding |

- The body is always complete regardless of mode.
- A sticky review is always `none`, since an inline comment belongs to the review that created it and could not follow the body as it is edited.
- General findings, those with no location, are never inline.
- Excluded and withdrawn findings appear in neither the body nor `comments[]`.
- An inline comment's body is the finding rendered as it appears inside its disclosure (the summary line as the first line, then the meta block without its location line, then the impact, body, suggested fix and references), without the `<details>` wrapper.

## Sticky reviews

`loupe publish --sticky` keeps one loupe review per pull request current (`specs/025-sticky-review`). The first sticky round creates a `COMMENT` review with no inline comments. Each later round replaces that review's body in place, and nothing else about it. The body shows the newest round on top and each earlier round collapsed below it.

````markdown
`🔵 1 question`

The blocking issue is fixed; one question left.

---

### Worth a look

<details>…</details>

---

reviewed `bbbbbbb` · via `gadfly-review-pr 2.2.0`

---

<!-- loupe-earlier -->

### Earlier rounds

<!-- loupe-round -->

<details>
<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>⛔ 1 blocking</code></summary>

> ### Must fix
>
> <details>…</details>
>
> reviewed `aaaaaaa` · via `gadfly-review-pr 2.1.0`

<!-- loupe digest=<sha256> publication=<uuid> -->

</details>

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-findings v=1 sha256=<sha256> <data> -->
<!-- loupe-meta v=1 round=2 src=gadfly-review-pr@2.2.0 inline=none blocking=0 issues=0 suggestions=0 questions=1 other=0 excluded=0 withdrawn=0 reinstated=0 regraded=0 sticky=2 -->
````

- **The newest round is composed exactly as an ordinary review with `--inline none`**: chips, opening prose, `Must fix`, `Worth a look`, then a divider and its footer. The hidden markers end the body as on any review, and describe the newest round.
- **`### Earlier rounds` follows the newest round's footer, after a divider**, only when the review holds an earlier round or dropped one. It holds one `<details>` per earlier round, newest first.
- **An earlier round's `<summary>` reads `Round N · reviewed <code>SHA</code> · CHIPS`.** `N` is the round's place in the sticky review, 1 for the first sticky round, never loupe's `round=`, which can start above 1. `SHA` is its footer's commit. `CHIPS` is its chips row as pills, each chip in its own `<code>` separated by a space so it matches the scoreboard the round opened on, or a `<code>🟢 no findings</code>` pill when the round published none. A `<summary>` is raw HTML, so the pills are `<code>`, not backticks. Its section holds the rest of the round as it read when it was on top, as one blockquote (`specs/030-quoted-rounds`): its prose and sections, with loupe's `### Must fix` and `### Worth a look` headings as they read on top, then its footer line exactly as it read. Every line of the quote is prefixed with `> `, and a blank line with `>` alone, so GitHub draws one bar down the opened round and the next summary sits clearly outside it. A finding's location line, itself a quote, reads `> > ` and nests inside the round's quote. The footer stays so the history shows which source and model reviewed each round. The round's reconciliation marker follows the quote, outside it, after a blank line. The quote holds no divider loupe wrote: not the one before each section, not the one before the footer, and none after it, since inside the quote a rule reads as a boundary between rounds. A divider the author wrote in the prose stays. Its chips row is dropped, since the summary carries it. Its `loupe-meta` and its findings record are dropped too, so a body carries one of each, and both describe the round on top. The reconciliation marker stays, so an interrupted publish of that round still reconciles after a later round edited over it.
- **A loupe released before the findings record cannot continue a sticky series that carries one.** It finds the record where it expects the reconciliation marker and refuses with `sticky`, whose fix, publishing once without `--sticky`, still works. Every loupe from spec 027 on reads bodies with and without a record.
- **Earlier bodies still read back.** A summary whose chips are plain text joined by ` · ` or that says `no findings` in plain text, as v0.11.0 wrote them, and an unquoted collapsed round with `###` section headings or no divider after its footer, as earlier builds of this layout wrote it, all read back and are carried as they are.
- **A body in the v0.12.0 layout** (each collapsed round unquoted, with `####` section headings and a divider before its sections, before its footer and after it) still reads back, and the next round quotes the round it showed. The rounds v0.12.0 collapsed are carried as they are, apart from their `Round N`, so a review MAY mix both layouts until those rounds are dropped or the series ends. A collapsed round is read as quoted when its content ends on its footer inside the quote, so an unquoted round whose prose opens on a quote is not taken for a quoted one.
- **A body in the v0.11.0 layout** (the newest round's footer after the earlier rounds, and collapsed rounds without a footer) still reads back, and the next round writes this layout. The round it showed keeps its footer. The rounds v0.11.0 collapsed have no footer to recover and are carried as they are. One such body is refused with `sticky`: every earlier round dropped, and the shown round's prose ending in a divider and a line shaped like a footer, since nothing then tells it from a footer added on GitHub.
- **A body published before this format** (its collapsed rounds keep their footer and a chips line inside) still reads back. Its collapsed rounds are carried as they are, and only their `Round N` is renumbered by place. When the round it shows has no findings and its prose ends in a divider and a line shaped like a footer, it MAY be refused with `sticky` instead, since it then reads as a footer added on GitHub. Only pre-release probe reviews carry this format.
- **Two hidden comment lines delimit the rounds**: `<!-- loupe-earlier -->` opens the section, and `<!-- loupe-round -->` precedes each round. loupe reads a sticky body back by these lines and by the generated tail (reconciliation marker, findings record when present, `loupe-meta`) and by the newest round's divider and footer, and only on lines outside a fence. The allowlist refuses an HTML comment outside a fence in every authored field, so authored text can quote a delimiter but never move a round boundary. A body that has lost that structure, such as one reworded on GitHub, is refused with `sticky` rather than guessed at. That includes text added to the footer or before the first collapsed round, a newest collapsed round whose footer was removed, quoted or not, a findings record anywhere but the tail, which the next body would otherwise carry as a second record, a collapsed round that is not one `<details>` under loupe's `<summary>Round N · …</summary>` line, a round that is not exactly one `<details>` whose tags inside pair up and nest, or whose fence or HTML comment is left open, and collapsed rounds plus dropped ones that do not add up to `K − 1`, since the next edit would otherwise lose a round without saying so.
- **When the earlier rounds would take the body past 65,536 characters, the oldest are dropped first**. GitHub was observed to accept an edit of up to 262,144 UTF-8 bytes, and 65,536 characters never exceed that, so the bound is conservative by design, and the section opens with `The oldest round was dropped to fit GitHub's length limit.` or `The N oldest rounds were dropped to fit GitHub's length limit.` A dropped round's reconciliation marker goes with it. When the newest round alone is over the limit, publish refuses with `limit` as for any review.
- **Two publications that edit the same review at once race.** Each reads the same body and the later edit wins, so the earlier one's round is lost from the history. loupe sends no precondition with the edit and does not lock across machines. The attended recheck after `y` narrows the window but cannot close it, and a pipeline's concurrency group is the guard.
- **An edit is silent.** GitHub sends no notification for a body edit, and a round posts nothing to announce itself. Whether GitHub notifies is unverified (`docs/github-facts.md`).
- **The review keeps the first round's commit and state.** An edit cannot change `commit_id` or the event, which is why a sticky review is `COMMENT` only: a `REQUEST_CHANGES` would outlive the round that asked for it. The footer names the newest round's commit.
- **A plain loupe review ends a sticky series.** When the publisher's newest loupe review carries no `sticky=`, the next sticky round creates a new sticky review. That is also how a sticky review loupe cannot read back is left behind: publish once without `--sticky`.
- **The review edited is the publisher's newest sticky review**: the viewer's own, or unattended, the newest `[bot]` review whose `src=` name, the part before any `@`, matches the source the round's capture recorded. An installation token cannot read its own login, so the source is what tells one App's review from another's, and an unattended `--sticky` round with no source is refused. Each unattended pipeline on a repository MUST use its own source name, since two sharing one pick or mix each other's reviews. An attended round never edits another person's review, and reviews without `sticky=` are left as they are. Two Apps that record the same source name MAY still pick each other's review, and GitHub is expected to refuse that edit, which is unverified.
- **Several publishers on one pull request each keep their own sticky review.** A collapsed round's `Round N` counts only that review's rounds. An unattended `round=` still counts every App's loupe reviews, as the bullet on unattended numbering above says.
- **Numbering.** `round=` is the newest round's `N`. Attended, `N` is counted from local receipts as for any review. Unattended, a sticky bot review counts as the `K` rounds its `sticky=K` records, and any other bot loupe review as one.
- **A collapsed round nests its findings' `<details>` one level deeper** than the allowlist's bound assumes, 17 levels at most. GitHub was observed to render 17 levels (`docs/github-facts.md`). The quote adds no `<details>` level. GitHub was observed to render `<details>` and a nested quote inside a quote.
- **A collapsed round is carried as it is, even when a line of its quote lost its `>` on GitHub.** That line renders outside the quote. A finding's `</details>` line that lost it also closes the round's collapse early, so the rest of that round renders below it; loupe carries that too. Only a hidden marker could tell that edit from an earlier layout's prose that ends in a quote and a line shaped like a footer, so loupe does not guess.
- **The quote costs two characters a line.** The length limit keeps its rule, so a long history MAY keep one round fewer than before.

## The supported Markdown boundary

The renderer wraps each finding in a generated `<details>`. Authored content that closes it early restructures the published review. Four fields carry raw Markdown: a finding `body`, `impact` and `suggestedFix`, and the review `summary`. Every other interpolated field is made structurally incapable of unbalancing the wrapper:

| Field | Why it cannot escape |
| :--- | :--- |
| `title` | Collapsed to one line. Backslash escapes are applied, each backtick code span becomes `<code>` around its HTML-escaped content, and the text between spans is HTML-escaped, so `<code>` is the only tag the title adds |
| `label` | Collapsed to one line, then HTML-escaped inside a `<b>` tag; inline, ASCII punctuation is also backslash-escaped |
| `confidence`, `verified` | Collapsed to one line, then HTML-escaped before interpolation |
| `severity` | Collapsed to one line; an enum word selects a fixed pill on the summary line and is written as text on the meta line, and anything else is emitted inside a code span on the meta line |
| Location, footer fields | Collapsed to one line and emitted inside a code span whose backtick run is longer than any run in the content |
| `references` | Refused at input, and again at composition, unless each is a URL free of whitespace, control and format characters, `<`, `>` and backticks; then emitted as a link with escaped text and a `<…>` destination |
| `body`, `impact` | Validated by the allowlist below; correction `loupe edit <id> --from -` |
| `suggestedFix` | Validated by the allowlist below at write time; correction `loupe edit <id> --from -`. A stored fix that fails it is emitted inside a fence longer than any backtick run in the content |
| `summary` | Validated by the allowlist below; correction `loupe summary --from -`, or a reword at the confirmation for a human's message |

### Allowlist

A body, impact, suggested fix or summary is accepted when all of the following hold. The check runs at write time in `add`, `edit` and `summary`, and again at publish for the body and impact of every publishable finding and for whichever opening prose is being published: the human's message when attended, the draft's summary when not. Excluded and withdrawn findings are not checked.

| Rule | Code |
| :--- | :--- |
| At most 64 KiB of UTF-8 | `limit` |
| Every fence opened with three or more backticks or tildes, indented at most three spaces, is closed by a fence of the same character and at least the same length; fence content is literal and exempt from the rules below | `fence` |
| Outside fences and code spans, the only raw HTML is `<details>`, `<details open>`, `<summary>`, `</summary>` and `</details>`, each alone on its line (trailing whitespace and up to three spaces of indentation allowed) | `html` |
| No HTML comments, declarations, CDATA or processing instructions outside fences | `html` |
| Every `<details>` is followed, after optional blank lines, by `<summary>` on the next content line, and `</summary>` closes it on the same line or a later one before any other content | `html` |
| `<details>` nesting is balanced and at most 15 levels deep in a body, 16 in a summary | `depth` |

- A refusal names the code, the one-based line of the first violation (line 1 for `limit`), the reason and the correction command.
- Tags inside code spans, fences, backslash escapes or entities are text, not HTML.
- A code span that does not close on its own line hides nothing on the later lines of its paragraph, and a line indented four or more columns is never a fence or a tag line, because CommonMark may read either differently from a line scanner.
- Ordinary Markdown (prose, headings, emphasis, links, autolinks, entities, tables, task lists, lists, quotes, hard breaks) is accepted without inspection.
- This is deliberately a line scanner, not a parser. It refuses more than GitHub would (for example `<br>`, `<img>`, `<sub>` outside a summary) in exchange for being small and predictable.
- GitHub matches `<details>` anywhere in a line and an unterminated `<!--` swallows the rest of the review, which is why comments are refused outright and disclosure tags must stand alone.

## Refusals

| Refusal | Correction |
| :--- | :--- |
| A publishable finding body that fails the allowlist | `loupe edit <id> --from -` |
| A review summary that fails the allowlist, with or without findings | `loupe summary --from -` |
| A composed body, or an inline comment body, over 65,536 characters | exclude a finding in `loupe review` or shorten bodies with `loupe edit <id> --from -` |
