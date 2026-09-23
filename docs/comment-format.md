# The published comment format

loupe publishes one GitHub review per round. Where this document and `specs/001-loupe-v1/spec.md` disagree, the spec wins and the conflict is a bug here.

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
- **Unknown labels are never refused.** An unrecognized label renders verbatim in the finding's own summary line (`<b>perf-nit</b>`), and a nonblocking one leads its row with the `⚪` dot and counts in the `⚪ other` chip. A finding with an unknown label and `blocking` set is placed in `Must fix` and counted in the `⛔` chip, like every other blocking finding, while still counting under its own label in `loupe-meta`. `other` is a bucket, never a rewrite of the label.
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
The retry path can publish twice and the digest is not verified on reconcile.
Worth fixing before this merges; the rest reads fine to me.

`⛔ 1 blocking` `⚪ 1 other`

---

### Must fix

<details>
<summary>⛔ <b>issue</b> <picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major-dark.svg"><img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major.svg" alt="MAJOR" height="16" align="absmiddle"></picture>: Retry loop can double-publish a review</summary>

> [`internal/publish/publish.go:88`](https://github.com/o/r/pull/7/files#diff-8f3c…R88)\
> **Confidence:** high\
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

> [`internal/draft/store.go:10–14`](https://github.com/o/r/pull/7/files#diff-5610…R10-R14)

…

</details>

---

reviewed `d23632e`

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
````

### Opening

The opening prose leads the body and the chips row follows it, directly above the divider and the sections whose row dots it keys. With no prose the body opens on the chips row; with no findings it is the prose alone. The prose leads because on an attended review it is the person's own words, and a notification or preview then opens on a sentence rather than a row of code spans (`specs/020-review-format`). The body MUST NOT open with a callout. GitHub's review header already shows the event (approved, changes requested, commented), and the `⛔` chip already states the blocking count, so a callout would only repeat one or the other.

**Who writes the opening prose depends on who published.** An attended review carries a message the human typed at the publish confirmation, reading the body as they wrote it; the draft's summary is not published on that path and orients the human while they sort findings instead. An unattended review carries the draft's summary, because no human is there to type anything, and its footer already ends in ` · unattended` so a reader knows the prose was not read by a person before it appeared.

The prose is optional in both modes. When there is none the body opens on the chips row, and nothing is rendered in its place. A review with neither prose nor a finding is refused rather than posted.

### Chips

- One inline code span per non-zero count, zeros omitted, pluralized except `other` and `blocking`.
- Each is led by a dot: ⛔ blocking, 🟡 issue, 🟣 suggestion, 🔵 question, ⚪ other.
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
- **Only the four enum words reach the row.** A stored severity that is not one of them stays on the meta line instead, where its code span makes it inert. Exactly one of the two places carries a finding's severity, never both.
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
- **Terminal surfaces show the word, not the HTML.** The publish confirmation shows the body as raw Markdown, and it replaces each pill with its word there, in the `<summary>` lines outside fences and on each inline comment's first line only, so authored text keeps its bytes. The payload view shows the exact bytes.
- **This is a network dependency.** Every reader's client loads the pills through GitHub's camo proxy. When it cannot, the alt word stands in.

### The meta block

A blockquote at the top of the disclosure body, present only when at least one part exists.

- One part per line, each line separated from the next by a backslash hard break: the location first, then confidence, severity and verified, in that order. An absent part leaves no line, and **an enum severity leaves none either**, because the summary line directly above already carries it.
- A finding with no location starts at its first present part; one with only a location has that line alone.

| Part | Rendering | Example |
| :--- | :--- | :--- |
| Location | Review body only. A Markdown code span, side included inside the span, as the text of a link to the PR's files view | `` [`src/a.go:88`](…/pull/7/files#diff-<sha256 of the path>R88) `` |
| Confidence | the bold label `**Confidence:**` then the escaped level word | `**Confidence:** high` |
| Severity | **only a value outside the enum**, as the bold label `**Severity:**` then a code span. An enum word is on the summary line and is not repeated here | `` **Severity:** `P2` `` |
| Verified | the bold label `**Verified:**` then the escaped word | `**Verified:** reproduced` |

- **Location is rendered in full**, path and range, because with `--inline none` this is the only place it survives. `src/a.go:88` for a single line, `src/a.go:10–14` for a range. **The side is shown only when it is `LEFT`**, appended as ` (LEFT)`.
- **In the review body the location links to the PR's files view**, not to a review thread: GitHub assigns no inline-comment URL until the review is submitted. The anchor is `#diff-` followed by the SHA-256 hex of the path, then `L` or `R` for the side and the line, with `R21-R24` for a range. This is observed behavior, see github-facts.md.
- **An inline comment carries no location line**: the comment already sits on the line, so its meta block starts at confidence.
- **A code span MUST be delimited by a backtick run longer than any run inside its content**, padded with a space when the content starts or ends with a backtick. This is the CommonMark rule and it applies to every generated code span, including the footer's.
- **Code-span contents MUST NOT be HTML-escaped.** A code span is already inert, and GitHub shows character references inside one literally. Escaping is for text interpolated outside a span.
- **Every generated inline field MUST be collapsed to a single line before interpolation**, with runs of whitespace becoming one space. A newline in a title breaks the summary line out of its tag; a newline in `severity` would end the meta block's blockquote and leave the rest of the line as body text.
- Confidence, severity and verified render only when the reviewer supplied them. **Severity is carried by the summary line, not by this block**, on the same rule that keeps an inline comment's location out of it: the line above already says it. None of the three decides section placement.

### Field labels

Impact, the suggested fix and the references each render under a bold label. **A value of one line follows its label on the same line**, as `**Impact:** …`. A longer value goes below a bold label on a line of its own, as `**Impact**`, because a list, a quote or a fence can only open at the start of a line. A one-line value that starts like one, such as `- Return the error`, shows that punctuation as text after the label. A lone carriage return ends a line as `\n` does, in this rule and in the allowlist.

### Impact

- Rendered first after the meta block, before the body, as Markdown with trailing newlines trimmed. A reader deciding whether to act wants the consequence before the reasoning.
- It passes the same allowlist as `body` and is rechecked at composition, so it cannot unbalance the disclosure.

### Suggested fix

- Rendered after the body, as Markdown with trailing newlines trimmed. Code in it MUST be fenced by whoever writes it; bare code renders as prose.
- It passes the same allowlist as `body` at write time. A stored fix that fails the allowlist, written before this rule, is instead rendered inside a fence whose length is `max(3, longest backtick run in the content + 1)` under a `**Suggested fix**` line, so it stays inert and still publishes.
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
```

- The footer answers only what a reader who never installed loupe can act on: which commit, who reviewed it, and whether anybody read it before it posted (`specs/009-review-footer`).
- `SHA` is the abbreviated captured head commit, a generated code span subject to the backtick rule.
- The ` · via ` suffix MUST appear only when capture recorded a source, and is otherwise absent, leaving the first form byte for byte. `NAME VERSION` is that source with its `@` rendered as a space, or `NAME` alone when it has no version, in a generated code span.
- **` · unattended` MUST come last**, after ` · via ` when there is one, only on a review `loupe publish --unattended` sent (`specs/007-unattended-publish/spec.md` FR-016, amended by `specs/009-review-footer`). A review published without `--unattended` is unchanged byte for byte.
- Nothing else: no tool name, no round number, no run reference, no local paths, and no agent name beyond the source capture was given, none of which a PR reader can resolve. Machine-readable provenance belongs in `loupe-meta`.

### Markers

Two HTML comments, both shipped in the payload and both visible in raw Markdown forever. Neither is private.

```text
<!-- loupe digest=<sha256 of the publishable draft> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
```

- The first is the reconciliation marker: an unknown publication is resolved by finding a review whose body contains this exact comment. Its format MUST NOT change between versions that may need to reconcile each other's attempts.
- **`loupe-meta`'s per-label counts are a census, not the chips row.** They count every published finding, blocking ones included, so a review whose only issue blocks records `blocking=1 issues=1` while the visible chips show no issue. The chips are a key to the row dots a reader sees; the marker is an inventory of what the review held.
- **`unattended=1` follows `round=` only on a review `loupe publish --unattended` sent**, directly before `src=` when both are present (`specs/007-unattended-publish/spec.md` FR-016). It is also how `loupe publish --unattended` counts a pull request's earlier rounds for `N` (FR-015), alongside the review's author ending `[bot]`.
- **`src=` follows `round=`, or `unattended=1` when present, only when capture recorded a source**, as given (`src=gadfly-review-pr@2.2.0`). It is never escaped, so capture refuses a source that does not match `^[a-z0-9][a-z0-9._-]*(@[0-9][0-9A-Za-z.+-]*)?$`, is over 64 characters, or contains `--`, and every command refuses a `target.json` carrying one. None of those can close the comment.
- **`model=` follows `src=`, or sits where `src=` would, only when capture recorded a model** with `loupe capture --model`, as given (`model=anthropic/claude-sonnet-5`). It is never escaped, so capture refuses a model that does not match `^[a-z0-9][a-z0-9._/:-]*$`, is over 64 characters, or contains `--`, and every command refuses a `target.json` carrying one. The model is provenance for tooling and never appears in the footer.
- New keys MAY be added to `loupe-meta`; existing keys MUST keep their meaning. **`round=` is the one recorded exception:** it carries `N`, this review's position among the pull request's publications (FR-042), where it once carried the run's round. Outside the shared-`N` case below, a pull request with no abandoned round reads identically under both, and `round=` still rises with each review a pull request publishes, so a reader that takes the highest as live picks the newest review. Since `specs/009-review-footer` the marker is the only place `N` appears, so the three rules that follow are a key for tooling and never something a reader of the review is asked to reconcile.
- **`N` counts publications, not captures.** It is 1 plus the number of the pull request's *other* rounds holding a receipt or an attempt, so a round captured and abandoned unpublished does not advance it, and it MAY be lower than the run's round. An attempt counts because its review may already be on GitHub, so no other round reuses its number while the attempt stands, at the cost of a skipped number when an unknown attempt never landed. Other rounds count rather than earlier ones, so an older round that publishes after a newer one still gets the higher number.
- **An unattended review counts differently.** `loupe publish --unattended` has no local round state to count, so its `N` is 1 plus the pull request's reviews by a `[bot]` author carrying a `loupe-meta` marker (`specs/007-unattended-publish/spec.md` FR-015). On a pull request reviewed both ways the two counters are disjoint: a human's review and a bot's MAY carry the same `N`, and a bot's `N` MAY be lower than the human `N` before it. The author and the ` · unattended` segment are what tell them apart.
- **Two reviews MAY share an `N`**, and every way needs the head force-pushed back to an older round's commit: `--retry-unknown` on that round after a newer round published, since the retry counts the newer receipt that already counted its attempt; two rounds at that commit publishing at once, since `N` is fixed before confirmation and each publish locks only its own run; or `--retry-unknown` on that round failing with a definite rejection, which deletes the attempt a newer round already counted, so the next round to publish reuses the newer round's number. This is accepted rather than refused.

## Inline modes

`loupe publish --action <action> --inline none|blocking|all`, default `blocking`.

| Mode | `comments[]` contains |
| :--- | :--- |
| `none` | nothing |
| `blocking` | every published, located, blocking finding |
| `all` | every published, located finding |

- The body is always complete regardless of mode.
- General findings, those with no location, are never inline.
- Excluded and withdrawn findings appear in neither the body nor `comments[]`.
- An inline comment's body is the finding rendered as it appears inside its disclosure (the summary line as the first line, then the meta block without its location line, then the impact, body, suggested fix and references), without the `<details>` wrapper.

## The supported Markdown boundary

The renderer wraps each finding in a generated `<details>`. Authored content that closes it early restructures the published review. Four fields carry raw Markdown: a finding `body`, `impact` and `suggestedFix`, and the review `summary`. Every other interpolated field is made structurally incapable of unbalancing the wrapper:

| Field | Why it cannot escape |
| :--- | :--- |
| `title` | Collapsed to one line. Backslash escapes are applied, each backtick code span becomes `<code>` around its HTML-escaped content, and the text between spans is HTML-escaped, so `<code>` is the only tag the title adds |
| `label` | Collapsed to one line, then HTML-escaped inside a `<b>` tag; inline, ASCII punctuation is also backslash-escaped |
| `confidence`, `verified` | Collapsed to one line, then HTML-escaped before interpolation |
| `severity` | Collapsed to one line; an enum word selects a fixed pill on the summary line, and anything else is emitted inside a code span on the meta line |
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
