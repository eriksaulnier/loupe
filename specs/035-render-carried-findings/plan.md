# Implementation Plan: Render carried findings

**Branch**: `feat/render-carried-findings` | **Date**: 2026-09-29 | **Spec**: [spec.md](spec.md)

## Summary

`render.Body` already receives the round's assessments for the findings record. It now also reads the open ones: they count in the chips row, they get a compact section of their own, and `loupe-meta` names how many there are. Nothing is carried or assessed differently.

## Technical Context

Go, standard library, as today. No new dependency. The change is confined to `internal/render`, plus `docs/comment-format.md`.

## Design

- **Counting.** `countChips` takes the open assessments beside the round's own findings, so the chips row, the anchor and a collapsed round's pills keep one source of counts. The anchor needs no new key.
- **The section.** `carriedSection` builds each item from `summaryLine` over the carried finding, with the Markdown escaping an inline comment's first line uses, since a list item is Markdown and not raw HTML. The location reuses `locationText` and links to `https://github.com/OWNER/REPO/blob/COMMIT/PATH#L…`, each path segment escaped.
- **Collapse.** The section sits among the round's sections, so `verifiedContent` keeps it and `withoutDividers` drops its divider. No reader changes.
- **`loupe-meta`.** Two keys appended after `regraded=`, before `sticky=`, which `stickyKey` requires at the end of the line.

## Constitution Check

- **I, III, IV.** Untouched.
- **II.** An attended round shows the section in its confirmation, which is the body. It adds no prose: copies of titles already published.
- **V.** New marker keys only. No existing key changes meaning.
- **VI.** No new type, record version or dependency.
- **VII.** Unit tests in `internal/render`, each confirmed by mutation.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/comment-format.md` gains a third section and two `loupe-meta` keys | A carried open finding is otherwise invisible, and the round can open green over it. |
