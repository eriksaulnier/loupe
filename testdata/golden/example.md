`⛔ 1 blocking` `⚪ 1 other`

The retry path can publish twice and the digest is not verified on reconcile.
Worth fixing before this merges; the rest reads fine to me.

---

### Must fix

<details>
<summary>⛔ <b>issue</b> <picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major-dark.svg"><img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major.svg" alt="MAJOR" height="16" align="absmiddle"></picture>: Retry loop can double-publish a review</summary>

> [`internal/publish/publish.go:88`](https://github.com/o/r/pull/7/files#diff-ce7057dc498c6d64ab8893ac812c640bfd801aa1e0dff85792a9ee9e6a294aebR88)\
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

> [`internal/draft/store.go:10–14`](https://github.com/o/r/pull/7/files#diff-cd72d9c08f06a69424f41d70a7dc4f036dc2fd30f90c0466fa71d58f41b2a65bR10-R14)\
> **Severity:** minor

…

</details>

---

reviewed `d23632e`

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
