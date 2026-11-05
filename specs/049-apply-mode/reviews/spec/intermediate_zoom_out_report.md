# Stage 8: Intermediate Zoom Out Review

- Target: `specs/049-apply-mode/spec.md`, full file, after the decisions of the 2026-11-03 round were applied
- Reviewer: `egm-zoom-out`, Claude Opus 5.5, fresh context
- Standard: `zoom-out-standard.md` (EGM 0.1.0)
- Status: completed. No Blockers, 3 Majors, 7 Nits.

## Verdict

The revision resolved most of Stage 5:

- Stage 5 M1, the problem statement and the preamble.
- M2, one stored field.
- M4, `--tree` removed.
- M6, `--files` outside the repository.
- M7, the file-change flag removed.
- M8, no receipt reuse.
- M9, `stats` and `list`.
- N1 to N5.

The new prose sections are short. The added weight is in three pieces of mechanism: the checked reference prefix, the copy of the decisions in `return.json` on top of the seal, and an absolute FR-030 rule that the spec's own requirements break. The seal, `returned` and `wrong-mode` are justified.

## 🟡 Major 1: The checked prefix brings back the machinery D3 removed, and adds an error code the spec never lists (lines 77, 119, 132, 187, 236–238, 251–252, 277–278, 325)

The prefix is derived three ways: from a GitHub origin, from the sanitized directory name, or not at all outside a repository. It needs a character rule and a mismatch refusal. Line 132 rejects per-repository numbering for needing exactly that derivation and rule. Two same-named repositories still share a prefix. loupe finds a run by n alone, and `list` already shows `repository`, so the check catches only a mislabeled reference. `invalid-ref` is not an existing code: `run.ParseRef` refuses with `usage` (`internal/run/ref.go:40`), and line 119 names only `returned` and `wrong-mode` as new codes. Fix: make the reference `local#<n>`, or make the prefix display-only. Either way, keep `usage` for a bad reference.

## 🟡 Major 2: `return.json` copies the decisions, and the seal already protects them (lines 87, 120, 299, 301, 328)

With the seal, `draft.json` is the authority and cannot change, so the copy is redundant. Keeping both creates a second versioned shape for decisions, and the spec does not say which one `feedback` and `wait` read. Fix: `return.json` holds only the time and the draft version it sealed. The draft is the one source `feedback` reads.

## 🟡 Major 3: FR-030's rule against branching on kind is broken by the spec's own requirements (lines 98, 249, 278–280, 303, 307, 317)

These still branch on kind outside the listed members:

- reference parsing and resolution (FR-006a);
- implicit selection (FR-007);
- the release lookup (FR-008);
- `wrong-mode` (FR-025);
- `stats` (FR-026);
- the scope of `review-open` (line 249).

"One function MUST map a run to its mode" is a code-level detail. Fix: add command availability and run reference and selection to the list, or narrow the rule to the review interface and command handlers. Leave "one function" to the plan.

## 🟢 Nit 1: The return gates do not name their refusal codes (line 298)

Say that return reuses `not-ready` and `empty`.

## 🟢 Nit 2: README.md and AGENTS.md still say loupe is PR-only (lines 117–121)

`README.md` line 8 and `AGENTS.md` line 3. The list of amended documents leaves them out.

## 🟢 Nit 3: Two glossaries, and one entry is a contract snapshot (lines 28–38, 321–328)

Fold Terms and Key Entities into one list. Drop "Feedback today", which will go stale.

## 🟢 Nit 4: The parity bar is stated four times, and Goals cites the wrong FR (lines 24, 127, 312, 346)

Keep one statement in Risks and point to it. The effort bar is FR-027 and FR-029, not FR-028.

## 🟢 Nit 5: The secrets risk relies on a skill rule no FR requires (lines 342, 319)

Add "read the untracked list before `handoff`" to FR-032, or reword the mitigation.

## 🟢 Nit 6: sift's closing note has no counterpart, and nothing records the choice (lines 311, 313)

sift's feedback carries the human's closing `note`. Record it as excluded by decision, or add it.

## 🟢 Nit 7: Wording leftovers (lines 269, 302)

FR-004 defines "the clone" but no longer uses it. FR-024 adds `returned` without a type.

## Stage 5 findings not fully resolved

- M3: partly. See Major 3.
- M5: applied with extra mechanism. See Major 1.
- M8: resolved, with the duplication in Major 2.
- M1: resolved except Nit 2.
