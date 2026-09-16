`⛔ 1 blocking` `⚪ 1 other`

The retry path can publish twice and the digest is not verified on reconcile.
Tests were not executed in this read-only review.

---

### ⛔ Blocking

<details>
<summary><b>issue (blocking):</b> Retry loop can double-publish a review</summary>

> [`internal/publish/publish.go:88`](https://github.com/o/r/pull/7/files#diff-ce7057dc498c6d64ab8893ac812c640bfd801aa1e0dff85792a9ee9e6a294aebR88)\
> **Confidence:** high\
> **Severity:** major\
> **Verified:** reproduced

Reproduced against the recorded fixture. The catch re-enters the loop after a request
that may already have succeeded, so a 502 produces two reviews.

**Impact**

A 502 on the first send leaves two reviews on the pull request, and the receipt records only one.

**Suggested fix**

```
Return the original write error.
```

**References**

- <https://github.com/o/r/issues/12>

</details>

---

### ⚪ Other

<details>
<summary><b>perf-nit:</b> Redundant sort on every read</summary>

> [`internal/draft/store.go:10–14`](https://github.com/o/r/pull/7/files#diff-cd72d9c08f06a69424f41d70a7dc4f036dc2fd30f90c0466fa71d58f41b2a65bR10-R14)\
> **Severity:** minor

…

</details>

---

loupe · round 2 · reviewed `d23632e`

<!-- loupe digest=<sha256> publication=<uuid> -->
<!-- loupe-meta v=1 round=2 inline=blocking blocking=1 issues=1 suggestions=0 questions=0 other=1 -->
