# Implementation Plan: Open findings across rounds

**Branch**: `031-open-findings` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

## Summary

A round records a status, `open` or `addressed`, for each earlier finding it assessed, with `loupe assess`. The statuses travel in the envelope, the receipt and the findings record. `show --previous` derives `earlier`: the previous round's open assessments, then its filed findings. So an open finding rides forward one round at a time, and every round sees every finding still open without reading more than the one previous round.

## Technical Context

Go, standard library and cobra, as today. No new dependency. Storage is the run directory: the draft gains `assessments`, `previous.json` gains `commit` and `assessments`, the envelope gains `assessments`. Each is omitted when empty, so every file an older run holds still reads.

## Design

- **Types.** `draft.EarlierFinding` is an envelope finding plus `filedIn {round, reviewUrl, commit}`. `draft.Assessment` is `{ref, status, finding}`. The envelope, the receipt and `previous.json` carry `[]draft.Assessment` as is. `render.RecordAssessment` mirrors it in the record, as `RecordFinding` mirrors the envelope finding, because `render` does not import `draft`.
- **Record version 2.** `withRecord` writes version 1 when the round has no assessments, so existing bodies and goldens keep every byte. With assessments it writes `v=2` whose data, deflated and base64 as before, is `{"findings": [...], "assessments": [...]}`. The checksum, the omission line and the one-record rule are unchanged. `ReadRecord` reads both versions. The sticky reader finds the record by its prefix, so it needs no change, and a loupe from 0.13 can still continue a sticky series whose top round carries `v=2`.
- **The earlier list.** `publish.Earlier(filed, assessments, filedIn)` returns the open assessments' findings in recorded order, then the filed findings, and the CLI numbers them `e-1` onward. It lives in `publish` next to the receipt and the stored previous round it reads.
- **The previous round's commit.** From a receipt, the envelope's `commitId`. From GitHub, a plain review's `commit_id`, or for a sticky body its current round's commit read back through `render.ReadSticky` and `render.PreviousRound`, since an edited review keeps its first `commit_id`. An unreadable commit leaves it empty rather than refusing, because the findings are still exact.
- **`assess`.** Resolves the run's previous round as `show --previous` does, builds `earlier`, and under the run lock replaces or adds each named assessment, sorted by ref. It takes `--expect-version` and `--run` like any mutation.

## Constitution Check

- **I.** `assess` is a plain command documented in `--help`. The skill is unchanged.
- **II.** An attended round publishes its assessments under the human's name. They add no prose: a status word and copies of findings already published. The confirmation's stand-in names the counts. No note field, for this reason.
- **III.** Files only. `show --previous` and `assess` read no network.
- **IV.** Untouched.
- **V.** `assess` accepts `--from`, emits one versioned result, and every refusal names its fix.
- **VI.** One new record version, justified by a second payload. No new dependency.
- **VII.** Tests use the fake GitHub and local repositories.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/comment-format.md` gains record version 2 | The next round has to read the assessments back from GitHub, and the version 1 data is a bare list an older reader requires. |
