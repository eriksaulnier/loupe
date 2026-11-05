# Stage 5: Initial Zoom Out Review

- Target: `specs/049-apply-mode/spec.md` (full file, as of 2026-11-03)
- Reviewer: `egm-zoom-out`, Claude Opus 5.5, fresh context
- Standard: `zoom-out-standard.md` (EGM 0.1.0)
- Status: completed

Verdict: no blockers. The objective is clear, and the parity clause (line 201) is a good test of whether the feature belongs in loupe. The design carries more mechanism than the user stories need, mostly in the target and reference model, and some of it pushes against the spec's own "no heavier than sift" bar.

## 🟡 Major 1: The root problem is never stated, and the constitution's preamble is not addressed (lines 9–13, 42–50, 68, 201)

The spec says what apply mode is, and that it is "the job sift does today" (line 68). It never says what a loupe user gains over running sift: one install, one review interface, more pane hosts, one decision loop to maintain. Line 201 names a cheaper fallback, a shared finding contract between the two tools, and nothing argues against it. The Constitution check (42–50) walks the seven principles, but the constitution preamble (`constitution.md` line 3) still defines loupe as PR-only, ending in "exactly one GitHub review per publication". `README.md` line 8 and `AGENTS.md` line 3 say the same. Fix: state the gain in one paragraph, and either amend the preamble or say why it does not bind.

## 🟡 Major 2: Mode and target kind are two stored fields that are always 1:1, plus a refusal no caller can trigger (lines 28, 157, 170–171, 216)

FR-000 stores `mode` and FR-001 stores `kind`, and local always means apply. No flag selects a mode, so "Capture MUST refuse any other pairing" and the edge case on line 157 guard an input nobody can send. This fails Principle VI's second-use test for the field. Fix: store one field and derive the other. Add the second when apply mode on a pull request lands.

## 🟡 Major 3: FR-017's mode interface leaves out most of the behavior that differs by mode (lines 185, 191–193, 204, 212, 239)

FR-017 lists capture, location check, end step and feedback shape, and says mode checks MUST NOT spread through the review interface. But the spec puts at least seven mode-dependent behaviors in that interface: the proposal callout and question marker (FR-005), the tweak key (FR-007), what `w` rewrites (FR-007a), what `x` does (FR-007b), the `p` "return" label, the hidden publish steps (FR-013), and the hidden open-PR key (FR-002). The four members also mix two axes: capture and location check depend on target kind, end step and feedback on mode. Fix: add a review-surface member, or narrow the claim. Separately, one `x` for both modes removes a mode branch outright.

## 🟡 Major 4: `--tree` is a public flag no story uses, and FR-001c fixes an implementation detail (lines 47, 171, 173–174, 156)

No story, scenario or success criterion runs `capture --tree`, and sift has no equivalent. It adds contract and `--help` surface, a refusal and a target field. The temporary index is a mechanism, stated twice, and SC-005 already pins the outcome. The review interface reads only the stored diff and outline, so nothing needs a Git tree after capture. `git diff <merge-base>` under `GIT_OPTIONAL_LOCKS=0` plus `--no-index` diffs for untracked files writes no Git object at all. Fix: drop `--tree` from the contract and let the plan choose the mechanism.

## 🟡 Major 5: The FR-001a reference scheme brings a lot of machinery, and its shape is inconsistent (lines 25, 38, 155–156, 172, 218)

Keying by repository brings a three-tier key derivation, a shared counter under a lock, an accepted collision case, the outside-repo special case, and the refusal of `--files` paths outside the root. With a bare directory key, `<owner>/<repo>:local#<n>` has no owner, while Key Entities says `<key>:local#<n>`. `run.ParseRef` needs owner/repo, and `run.Walk` walks only numeric PR directories. Simpler: one sequence for the whole data root, `local#<n>`, with the repository and worktree recorded in `target.json`. If the owner/repo form stays, reuse `gitx.OriginURLs`, `gitx.ParseGitHubURL` and `run.Lock`.

## 🟡 Major 6: Refusing `--files` outside the repository root breaks the spec's own parity bar (lines 159, 201, 203)

The sift skill tells agents to pass a plan kept in the host's own directory, such as `~/.claude/plans`. Under line 159 an agent must change directory to capture it, and the run is then keyed on a directory name such as "plans". That is heavier than sift for sift's own job. The global sequence in Major 5 removes the restriction.

## 🟡 Major 7: The FR-010 content hash and drift flag change nothing the agent does (lines 149, 171, 196, 217, 258)

The agent already has to find each change by content or heading, because its own edits shift the anchors (sift step 6, and line 196). The flag leads to no different action, but costs a hash per file, re-hashing at the end step, a feedback field and target fields. Fix: drop it, or move it to the out-of-scope export draft.

## 🟡 Major 8: The end step reuses the publish receipt, which is GitHub-shaped (lines 194, 222, 241)

FR-008 requires "a receipt … but no URL" and to "seal the draft as publish does". The code has no seal; a run counts as finished when `receipt.json` exists. `Receipt` (`internal/publish/records.go:100`) is GitHub-shaped, so a receipt with no URL redefines that record, and `list` and `stats` would label returned runs `published`. Fix: give the apply end step its own small record, have `wait` read it, and leave the receipt alone.

## 🟡 Major 9: Stats and list scope is untested or unamended (lines 58, 155, 197)

FR-011 requires `stats` to count local runs separately, but no story or success criterion covers it, and the current stats shape is PR-specific. Line 155 assumes a local listing, yet line 58 does not list `list` among the contract amendments. Fix: specify both shapes with a success criterion, or leave local runs out of `stats` here and amend only `list`.

## 🟢 Nit 1: The FR-004 handoff gate is ceremony (lines 158, 160, 183)

It makes the free-form label `question` load-bearing in one mode, adds a draft check to `handoff`, which validates no draft state today, and introduces a reviewer/applier hand-over no story exercises. sift treats the label as SHOULD.

## 🟢 Nit 2: `--source` and `--model` on local runs (line 177)

On PR runs these feed the published footer and `loupe-meta`. A local run publishes nothing. Keep them as provenance only, and say so, or drop them.

## 🟢 Nit 3: Reuse spec 041 for the human's exclude reason, and count the new draft fields (lines 58, 182, 191, 193, 243)

FR-007b's reason should reuse spec 041's rules (one line, at most 200 characters, the reply allowlist). The accept note and the exclude reason are new draft data, since `draft.Decision` has no field for either, but lines 58 and 182 count only `proposal` toward draft schema 5.

## 🟢 Nit 4: `--local` and "local run" mean different things (lines 113–116, 176, 171)

`--local` names one capture form, while "local" is the target kind of all four. A distinct flag name, such as `--no-files`, removes the ambiguity.

## 🟢 Nit 5: Two standalone skills will drift (lines 21, 31, 210–211)

`human-decide` will repeat most of `human-review`'s rules. The owner accepted this (F1, F11). The plan should name who keeps the shared rules in step.
