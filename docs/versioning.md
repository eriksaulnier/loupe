# Versioning loupe's records

loupe versions two kinds of record. Each JSON file in a run directory carries a `schema` number, and the findings record in a published review carries a `v=` number. This page holds the one rule for both. Principle V of the [constitution](../.specify/memory/constitution.md) points here.

Two other numbers are not record schemas, and this page does not govern them. The draft's `version` is the `--expect-version` counter, and `"loupe": 1` versions the `--json` result envelope in [`contracts/cli.md`](../specs/001-loupe-v1/contracts/cli.md).

## Run files

- A writer MUST bump a file's `schema` when it adds, removes or redefines a field.
- A reader MUST accept every schema from 1 up to its own.
- A reader MUST refuse a higher schema with code `record`, a message that names both numbers, and the fix `upgrade loupe`. It MUST check the schema before it checks for unknown fields, so that a newer file never reads as a damaged one.
- A reader MUST refuse an unknown field in a file at or below its own schema as damage, with the fix to inspect the file.
- A bump that only adds fields needs no migration. `ReadJSON` decodes an older file into the current struct, and the new fields stay empty.
- The first bump that removes or redefines a field MUST add a migration step to `ReadJSON` in the same change. That step MUST decode by schema before it decodes the current struct, because the strict decode would refuse a valid older file.
- A writer MUST NOT rewrite a file it refused. On its next save, a newer binary writes its own schema.

The readers are strict because the draft and the attempt are read, changed and saved again. An older binary that ignored unknown fields would drop a newer binary's fields on its next save, and nobody would see the loss.

A test in `internal/publish/schema_test.go` pins each file's JSON field set, with each field's whole tag, for each schema number. A field change without a bump fails `mise run check`. A bump adds the new schema's field set to that table and keeps the older ones. A removal or redefinition shows there as a field that the new set drops or retags, and that is the change that MUST carry the migration step. The bumps so far only add fields, so no reader migrates today.

## The findings record

- A writer MUST bump the record's `v=` when its data adds, removes or redefines a field.
- A writer MUST write the lowest `v=` that holds the content, so that an older reader reads every round it can.
- A reader of a higher `v=` MUST report that there is no previous round, and why. It MUST NOT refuse, because the previous round is an aid to the reviewer and a refusal would stop the capture.

A reader decodes the data of a `v=` it knows leniently, so an unknown field there drops without a word. That is one more reason a writer MUST bump.

The format of the record is in [`comment-format.md`](comment-format.md).

## `loupe-meta`

- A new key MAY come without a bump, because readers look keys up by name.
- A removed or redefined key MUST bump `v=`.

`round=` changed its meaning once without a bump, before this rule. [`comment-format.md`](comment-format.md) records that exception.

## Current versions

| Record | Version | Note |
| :--- | :--- | :--- |
| draft | `schema` 2 | 2 added `assessments` and `assessedAgainst` (`specs/031-open-findings`) |
| previous | `schema` 2 | 2 added `commit`, `publicationId` and `assessments` (`specs/031-open-findings`) |
| hand-back, target, comments | `schema` 1 | Strict. Unchanged until the next field change |
| attempt, receipt | `schema` 2 | `author`, `edited` and `envelope.editReviewId` came at 1 without a bump, before this rule. 2 added `envelope.assessments` (`specs/031-open-findings`) |
| findings record | `v=1`, `v=2` | Lenient data. `v=2` adds the round's assessments, and a round with none still writes `v=1` |
| `loupe-meta` | `v=1` | Key=value |
