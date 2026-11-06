# Feature Specification: Forgiving keys

**Feature Branch**: `keyboard`

**Created**: 2026-11-06

**Status**: Draft

**Input**: A reviewer used loupe with caps lock on. Every letter arrived uppercase, so most keys did nothing. On the publish confirmation, `Y` fell to the branch that cancels on any unknown key, so every attempt to publish went back to the list. The owner wants caps lock to be harmless and fewer keys to learn.

## Relationship to earlier specifications

- **`specs/001-loupe-v1/research.md`** said "Only `y` sends; any other key, Esc, Ctrl-C or end of input cancels." FR-004 replaces the second half.
- **`specs/002-review-ux/spec.md`** drops `N` from UX-006 and from its finding-detail key table. §5.5's "every other answer MUST continue to cancel" is amended by FR-004. The cancel-sentence rules in §4.5 and §5.7 go with the sentence.
- **`specs/051-apply-polish/spec.md`** FR-014 is amended and FR-017 is superseded: `A` becomes `t` and `X` becomes `c` (FR-006). Its contract `contracts/cli-apply-051.md` is amended to match.
- This feature changes PR-run screens, which `specs/051-apply-polish` FR-015 pinned for that spec's changes only.
- **`specs/038-review-visual-design/contracts/layout.md`** named the cancel sentence among the rows a pane keeps whole. The stray-key line takes its place.
- No `--json` envelope, refusal code, exit code or byte of the published review changes.

## Clarifications

### Session 2026-11-06 (owner)

- Q: Should letters stay case-sensitive? → A: No. Outside a text input an uppercase letter is its lowercase key, and no key differs from another by case alone.
- Q: Should `h`/`l` step between findings? → A: No. `h`/`l` mean left and right in vim, but nothing makes them the usual keys for "previous and next item", and `l` already opens the list on a local run. `[`/`]` step between findings, as they step between markers in the file diff.
- Q: How does a local run reject with a reason once `X` is gone? → A: `x` still rejects at once, and `c` rejects with a reason. The first answer was "`x`, then `t` for the reason". It failed because `x` moves to the next pending finding, or to the return step after the last one, so `t` landed on the wrong finding. `x` opening the reason input was turned down, because it makes a plain reject two keys.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Caps lock is harmless (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the publish confirmation and caps lock on, **When** the human presses `Y`, **Then** the review publishes.
2. **Given** any review screen outside a text input, **When** the human presses an uppercase letter, **Then** it acts as its lowercase key.
3. **Given** a focused text input (the send-back note, the rewrite, the confirmation's message), **When** the human types uppercase, **Then** the text keeps its case.

### User Story 2 - The confirmation cancels only on purpose (Priority: P1)

**Acceptance Scenarios**:

1. **Given** the confirmation with the message input not focused, **When** the human presses `n`, `q` or Ctrl-C, or the input ends, **Then** nothing is sent.
2. **Given** the same, **When** the human presses a key with no meaning there, **Then** the screen stays and says `y publishes, n cancels` until the next key.

### User Story 3 - One letter per action on a local run (Priority: P2)

**Acceptance Scenarios**:

1. **Given** a local finding that is not withdrawn, **When** the human presses `t`, **Then** the tweak input opens, holding the note of an accept that has one.
2. **Given** the same, **When** the human presses `c`, **Then** the reason input opens, holding the reason of a reject that has one.
3. **Given** a withdrawn local finding, **When** the human presses `t` or `c`, **Then** a notice says that `u` reinstates it, and no input opens.

## Requirements *(mandatory)*

- **FR-001**: Outside a text input, a key press whose text is one ASCII uppercase letter and that carries no ctrl, alt, meta, super or hyper modifier MUST be read as its lowercase key. This holds in the review program and in `loupe publish`'s confirmation.
- **FR-002**: A focused text input MUST receive letters in the case typed.
- **FR-003**: No binding MAY differ from another by letter case alone.
- **FR-004**: On the confirmation, `y` MUST publish, and so MUST `p` when there is a message input. `n`, `q`, Ctrl-C and end of input MUST cancel. Scrolling, `v`, Tab and Esc keep their meaning. Any other key MUST do nothing except show `y publishes, n cancels`, which the next key clears.
- **FR-005**: The confirmation footer MUST show `n cancel` beside the publish key, and MUST stay one line at 71 columns with Unicode glyphs. The publish verb shortens from "publish this review" to "publish" to make room. The sentence "any other key cancels, nothing is sent" is removed.
- **FR-006**: On a local run, `A` and `X` are removed. `t` MUST open the tweak input (accept with a note) and `c` the reason input (reject with a reason), each starting from the note that decision already holds. `x` still rejects at once. On a withdrawn finding, `t` and `c` MUST say that `u` reinstates it. An empty note or reason is the plain decision.
- **FR-007**: In the finding detail, `N` is removed. `[` and `]` MUST open the previous and next finding, as `←` and `→` do, and `n` stays as an alias for `→`.
- **FR-008**: The published screen keeps "any other key quits", because the review is already sent. With FR-001, `O` opens the browser as `o` does.

## Success Criteria *(mandatory)*

- **SC-001**: With caps lock on, a human can decide every finding and publish without turning it off.
- **SC-002**: No stray key on the confirmation discards the review or the message typed into it.

## Assumptions

- A terminal with caps lock on sends the uppercase letter as the key's text. ultraviolet's `Key.String()` returns that text, so the fold sees what a shifted key would send.
- This specification authorizes no commit, push, pull request, release or live review.
