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
| `severity` | arbitrary text | Retained and rendered verbatim, never mapped onto a label |

- **Label and blocking are independent.** A small required mechanical change is `suggestion (blocking)`, not an `issue`. `question (blocking)` is valid: a question whose answer determines whether the change is correct does block.
- **Unknown labels are never refused.** An unrecognized label renders verbatim in the finding's own summary line (`perf-nit:`) and counts in the `Other` chip only when nonblocking. A finding with an unknown label and `blocking` set is placed and counted in `Blocking`, like every other blocking finding, while still counting under its own label in `loupe-meta`. `Other` is a bucket, never a rewrite of the label.
- A label that is absent or empty is treated as no label: the finding counts as `other` and its summary line carries no label word. The same applies to an empty `confidence` or `severity`, which render nothing.
- **Confidence MUST NOT be computed, defaulted or inferred.** It is a value the reviewer reports or omits. The reason for a confidence level belongs in the body.
- **Severity is never mapped onto a label.** It renders as literal code-span content inside the disclosure body and never decides section placement.

## Body composition

- Section order is fixed: `Blocking`, `Issues`, `Suggestions`, `Questions`, `Other`. Empty sections are omitted.
- **Every included finding has exactly one home in the body.** A blocking finding lives in `Blocking` and nowhere else, regardless of its label. `--inline` decides only what additionally anchors to a line; it never changes what the body contains.
- Within `Blocking`, findings sort by label in section order (`issue`, `suggestion`, `question`, then every unknown label as one group) and by finding id within each group. Within a label section and within `Other`, findings sort by id.

````markdown
> [!IMPORTANT]
> **Changes requested** — 1 blocking finding.

`⛔ 1 blocking` `⚪ 1 other`

The retry path can publish twice and the digest is not verified on reconcile.
Tests were not executed in this read-only review.

---

### Blocking

<details>
<summary>🔴 <b>issue (blocking):</b> Retry loop can double-publish a review</summary>

> [`internal/publish/publish.go:88`](https://github.com/o/r/pull/7/files#diff-8f3c…R88)\
> **Confidence:** high

Reproduced against the recorded fixture. The catch re-enters the loop after a request
that may already have succeeded, so a 502 produces two reviews.

**Suggested fix**

```
Return the original write error.
```

</details>

---

### Other

<details>
<summary><b>perf-nit:</b> Redundant sort on every read</summary>

> [`internal/draft/store.go:10–14`](https://github.com/o/r/pull/7/files#diff-5610…R10-R14)\
> severity `minor`

…

</details>

---

loupe · round 2 · reviewed `d23632e`

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
````

### Verdict

Derived from `--action` at publish time, never authored, so the text cannot disagree with the GitHub event.

| Action | Callout |
| :--- | :--- |
| `request-changes` | `> [!IMPORTANT]` **Changes requested** |
| `approve` | `> [!NOTE]` **Approved** |
| `comment` | `> [!NOTE]` **Comment**, or `> [!IMPORTANT]` when blocking findings are included |

The blocking count MUST be appended when non-zero, as ` — N blocking finding(s).` It also appears once in the leading blocking chip and in `loupe-meta`; label chips MUST NOT count blocking findings again.

`WARNING`, `CAUTION` and `TIP` are deliberately unused. Alert titles are GitHub's and cannot be customized.

- "Warning" labels a routine request as a hazard.
- `CAUTION` is GitHub's register for negative outcomes and a review verdict is an ordinary outcome.
- `TIP` announces an approval as advice.

`IMPORTANT` says the review needs action and `NOTE` carries a verdict without retitling it.

### Chips

- One inline code span per non-zero count, zeros omitted, pluralized except `other` and `blocking`.
- Each is led by a dot: ⛔ blocking, 🔴 issue, 🟡 suggestion, 🔵 question, ⚪ other.
- Counts are derived from the final included findings at publish time, never from the summary prose.
- **One chip per section that exists below, in the order the sections appear.** Blocking leads the row and is counted only there: a finding sits in exactly one section, so a blocking issue is `⛔ 1 blocking`, never also `🔴 1 issue`.
- A reader who scans the row and finds no chip for a kind will find no heading for it either. The verdict callout states the blocking count again; that is the one repetition kept on purpose.
- No external badge images. They are a network dependency and route through GitHub's camo proxy.

### The summary line

`<summary>` carries the title, and a dot and label word only where they distinguish one row from another:

```
<summary>🔴 <b>issue (blocking):</b> Retry loop can double-publish a review</summary>
<summary>Redundant sort on every read</summary>
```

| Where | Dot | Label word | Why |
| :--- | :--- | :--- | :--- |
| `Blocking` | yes | yes | Labels mix here, so both tell the rows apart |
| `Issues` · `Suggestions` · `Questions` | no | no | The heading names the label; every row would repeat it |
| `Other` | no | yes | Constant dot, but the heading does not name the actual label |
| Inline comment | yes | yes | No heading to lean on |

- The label is followed by ` (blocking)` when the finding blocks. That happens only in `Blocking` and on the inline surface, because a blocking finding is never placed in a label section. Inside `Blocking` it is redundant against the heading and kept anyway so the inline surface, which has no heading, keeps the signal.
- `<b>` is the only tag loupe emits inside a `<summary>`. The title is HTML-escaped before interpolation.
- A finding with no dot, label word or blocking decoration in its context renders the escaped title alone. An unlabeled inline finding keeps the `⚪` dot. A finding that blocks but carries no label renders `⚪ <b>(blocking):</b>` before its title.

### The meta block

A blockquote at the top of the disclosure body, present only when at least one part exists.

- The location stands alone on the first line; confidence and severity follow on the second, joined with ` · `. The two lines are separated by a backslash hard break.
- A finding with no location has only the second line; one with only a location has only the first.

| Part | Rendering | Example |
| :--- | :--- | :--- |
| Location | Markdown code span, side included inside the span, alone on its line; in the review body it is the text of a link to the PR's files view | `` [`src/a.go:88`](…/pull/7/files#diff-<sha256 of the path>R88) `` |
| Confidence | the bold label `**Confidence:**` then the escaped level word | `**Confidence:** high` |
| Severity | Markdown code span | `` severity `minor` `` |

- **Location is rendered in full**, path and range, because with `--inline none` this is the only place it survives. `src/a.go:88` for a single line, `src/a.go:10–14` for a range. **The side is shown only when it is `LEFT`**, appended as ` (LEFT)`.
- **In the review body the location links to the PR's files view**, not to a review thread: GitHub assigns no inline-comment URL until the review is submitted. The anchor is `#diff-` followed by the SHA-256 hex of the path, then `L` or `R` for the side and the line, with `R21-R24` for a range. This is observed behavior, see github-facts.md.
- **In an inline comment the location stays a plain code span**: the comment already sits on the line.
- **A code span MUST be delimited by a backtick run longer than any run inside its content**, padded with a space when the content starts or ends with a backtick. This is the CommonMark rule and it applies to every generated code span, including the footer's.
- **Code-span contents MUST NOT be HTML-escaped.** A code span is already inert, and GitHub shows character references inside one literally. Escaping is for text interpolated outside a span.
- **Every generated inline field MUST be collapsed to a single line before interpolation**, with runs of whitespace becoming one space. A newline in a title breaks the summary line out of its tag; a newline in `severity` can open a fence that swallows the rest of the review.
- Confidence renders only when the reviewer supplied it. Severity never appears in the summary line or decides section placement.

### Suggested fix

- Rendered after the body under a bold `**Suggested fix**` line, inside a fence whose length is `max(3, longest backtick run in the content + 1)`, so its contents are inert.
- It is prose or code the human reads, never a GitHub `suggestion` fence: an applicable suggestion MUST contain the exact replacement lines for the anchored range, and a suggested fix holds prose. Wrapping prose in a suggestion fence produces a broken apply button.

### Dividers

- A `---` divider MUST be preceded by a blank line.
- GitHub renders a `---` that immediately follows `</details>` as literal text.

### Footer

```text
loupe · round N · reviewed `SHA`
```

- `N` is the run's round and `SHA` is the abbreviated captured head commit, a generated code span subject to the backtick rule.
- Nothing else: no run reference, no local paths, no agent name, none of which a PR reader can resolve. Machine-readable provenance belongs in `loupe-meta`.

### Markers

Two HTML comments, both shipped in the payload and both visible in raw Markdown forever. Neither is private.

```text
<!-- loupe digest=<sha256 of the publishable draft> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
```

- The first is the reconciliation marker: an unknown publication is resolved by finding a review whose body contains this exact comment. Its format MUST NOT change between versions that may need to reconcile each other's attempts.
- **`loupe-meta`'s per-label counts are a census, not the chips row.** They count every included finding, blocking ones included, so a review whose only issue blocks records `blocking=1 issues=1` while the visible chips show no issue. The chips are an index of the headings a reader can scroll to; the marker is an inventory of what the review held.
- New keys MAY be added to `loupe-meta`; existing keys MUST keep their meaning.

## Inline modes

`loupe publish --action <action> --inline none|blocking|all`, default `blocking`.

| Mode | `comments[]` contains |
| :--- | :--- |
| `none` | nothing |
| `blocking` | every included, located, blocking finding |
| `all` | every included, located finding |

- The body is always complete regardless of mode.
- General findings, those with no location, are never inline.
- Excluded and withdrawn findings appear in neither the body nor `comments[]`.
- An inline comment's body is the finding rendered as it appears inside its disclosure (summary line as a bold first line, then the meta block with a plain code-span location, then the body and suggested fix), without the `<details>` wrapper.

## The supported Markdown boundary

The renderer wraps each finding in a generated `<details>`. Authored content that closes it early restructures the published review. Two fields carry raw Markdown: a finding `body` and the review `summary`. Every other interpolated field is made structurally incapable of unbalancing the wrapper:

| Field | Why it cannot escape |
| :--- | :--- |
| `title`, `label`, `confidence` | Collapsed to one line, then HTML-escaped before interpolation |
| Location, footer fields | Collapsed to one line and emitted inside a code span whose backtick run is longer than any run in the content |
| `severity` | Collapsed to one line and emitted inside a code span |
| `suggestedFix` | Emitted inside a fence longer than any backtick run in the content |
| `body` | Validated by the allowlist below; correction `loupe edit <id> --from -` |
| `summary` | Validated by the allowlist below; correction `loupe summary --from -` |

### Allowlist

A body or summary is accepted when all of the following hold. The check runs at write time in `add`, `edit` and `summary`, and again at publish for every included finding and the summary. Excluded and withdrawn findings are not checked.

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
| An included finding body that fails the allowlist | `loupe edit <id> --from -` |
| A review summary that fails the allowlist, with or without findings | `loupe summary --from -` |
| A composed body, or an inline comment body, over 65,536 characters | exclude a finding in `loupe review` or shorten bodies with `loupe edit <id> --from -` |
