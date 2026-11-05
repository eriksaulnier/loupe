# Stage 6: Comprehension Test

- Target: `specs/049-apply-mode/spec.md` (lines 1 to end, read alone, as of 2026-11-03)
- Reviewer: `egm-comprehension`, Claude Opus 5.5, fresh context
- Status: completed. No Blockers, 8 Majors, 6 Nits.

## What this proposes

loupe gets a second mode, `apply`, alongside today's `publish`. In apply mode the target is local work, not a pull request. It can be a Git tree (`--tree`), the branch's working changes (`--changed`), named whole files (`--files`), or nothing at all (`--local --title`). An agent files findings. Each finding carries a `proposal`, shown to the human as "If you accept, <applier> will ...", or it is a `question`. The human decides each finding in the review UI. On apply runs only, the human can also accept with a note (a new "tweak" key) and exclude with an optional reason (`x`). One key then returns the run. `wait` reports `returned`, and `feedback --json` groups the findings into `apply`, `applyWithChanges`, `excluded` and `withdrawn`, with absolute file paths, for the agent to act on. Nothing touches GitHub. A new skill, `human-decide`, covers all apply runs. The whole design is held to "no heavier than sift" (FR-012 to FR-016) and to a small mode interface (FR-017).

## How the existing system works, per this document

An agent captures a pull request into a "run", files findings (with `suggestedFix`, `severity`, `blocking`, a diff `side`, and so on), and hands off to a human. The human decides each finding in a terminal UI with list and detail views: accept, exclude (`x`), rewrite (`w`, spec 046), send back with a note, or undo. `wait` returns `notes` when something is sent back, and the agent edits and replies while review stays open. The human then presses `p` and goes through the Action, Inline comments and Message steps. That posts one GitHub review, seals the draft, writes a receipt with a URL, and makes `wait` return `published`. PR runs have rounds: earlier findings are carried in ("sticky"), with `previous` and `assess`. The plugin ships one skill, `human-review`. Run state lives in files such as `target.json`, a versioned draft and `handback.json`, under a constitution of seven principles.

## Findings

### 1. 🟡 Major: roles of "the agent", "reviewer" and "applier" are never laid out (lines 11, 66, 183, 184, 203, 221)

Most of the document says "the agent". FR-004a says the applier "MAY differ from the reviewer that `--source` and `--model` name". FR-004 says "Either the reviewer supplies proposals, or the applier adds them with `loupe edit` before `handoff`." So a run can involve two agents, but nothing says who captures and sets `--applier`, who calls `handoff`, `wait` and `reply`, or who reads `feedback`. A reader cannot tell whether the two-agent case is supported or just tolerated. A short role table (reviewer, applier, human) with the commands each runs would fix this.

### 2. 🟡 Major: it is unclear what "accept" means on a question that has no proposal (lines 13, 23, 89, 160, 183, 220)

Line 13 says a question "asks the human for an answer". FR-004 allows a question with no proposal. The document never says how the human answers one. Accepting it with no note puts it in `apply`, but then there is nothing to apply. It is also unclear whether US2's decision cards must carry `label: question`, since they have proposals.

### 3. 🟡 Major: there is a `withdrawn` group, but no withdraw command in the allowed command set (lines 54, 78, 195, 203)

Line 54 relies on spec 041 to let the agent withdraw a finding. FR-012 limits the agent to eight commands, and US5 tests that `human-decide` uses nothing else. If withdrawing is its own command, this contradicts FR-012. If it goes through `edit` or `reply`, the document should say so.

### 4. 🟡 Major: the input that would ask for "apply mode on a pull request" is never named (lines 28, 157, 170)

There is no `--mode` flag, and the mode is derived from the capture form. A reader cannot tell what command line would trigger the refusal, so the requirement and its test are unwritable as stated.

### 5. 🟡 Major: "the clone" and "loupe's private refs" are undefined, and this matters for Principle IV (lines 47, 173–174)

For a local run, a reader cannot tell whether "the clone" means the user's own repository or a separate clone that loupe keeps. That is exactly what Principle IV hinges on. It is also not explained why `--tree <sha>`, which captures a tree that already exists, writes any objects or refs at all.

### 6. 🟡 Major: what a run outside a Git repository records is underspecified (lines 156, 159, 171, 172, 195)

Line 156 says `--files` and `--local` work outside a repository, but FR-001 requires every local target to record the worktree root and branch, line 159 refuses paths "outside the repository root", and FR-009 gives paths "in the captured worktree". None of these says what applies when there is no repository.

### 7. 🟡 Major: today's PR feedback shape is not described, so a reader cannot tell what is new (lines 27, 58, 66, 98, 195)

Line 27 implies `applyWithChanges` and `suggestion` already exist on PR runs. Line 58 says the spec adds "the feedback groups of an apply run". One sentence stating today's PR feedback shape would settle which names are new.

### 8. 🟡 Major: the PR-only concepts that apply mode excludes are named but never defined (lines 26, 140, 205)

FR-014 and US5 rely on "rounds", "sticky rounds", "earlier findings", "carried findings", "assessment", `previous`, `assess` and `blocking`, without saying whether these are commands, flags, fields or UI states. The PR run reference format is never shown next to the local one.

### 9. 🟢 Nit: the reference format is inconsistent when there is no GitHub origin (lines 25, 172, 218)

FR-001a says `<owner>/<repo>:local#<n>`, then keys on a directory name. Key Entities says `<key>:local#<n>`. Show the directory-name form outright, and say where runs are stored.

### 10. 🟢 Nit: run lifecycle vocabulary is used without a gloss (lines 76, 153, 182, 194, 222, 241)

"draft", "seal", "receipt", "handoff", `target.json`, the schema names and the error kinds `usage`, `input` and `review-open` are assumed.

### 11. 🟢 Nit: the tweak key is used before it is defined (lines 37, 96, 191, 238)

Line 37 refers to it. FR-007 defines it, and its letter appears only in Assumptions.

### 12. 🟢 Nit: several project-specific terms are not defined

"the good color" (75, 185), "the finding-id rule" (182), "second-use test" (49), "card" (22, 85), "type40" (245), "as spec 046 described for 041" (242), and `ORCA_TERMINAL_HANDLE` and its related terms (256). The constitution is cited without a path, and loupe is never described in one sentence.

### 13. 🟢 Nit: the decision-round labels F1 to F14 and "clarify F1" to F4 can be mistaken for requirement IDs (lines 19–38)

"clarify F1" also points to a second round that is never introduced.

### 14. 🟢 Nit: small internal inconsistencies

- Line 58 lists the target fields of FR-000 and FR-001, while line 241 adds FR-004a.
- Line 76 asks for one confirmation before sealing, while SC-006 counts "one key to return".
- FR-012 says "one for one" but lists eight loupe commands against seven sift commands.
- FR-010 says findings are "flagged" but does not name the flag.
- FR-006a covers heading anchors on `--files` and `--changed` but says nothing about `--tree`.
