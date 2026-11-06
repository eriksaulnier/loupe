# Implementation Plan: Forgiving keys

**Branch**: `keyboard` | **Date**: 2026-11-06 | **Spec**: [spec.md](spec.md)

## Summary

`foldLetter` in `internal/tui/keys.go` turns an unmodified uppercase ASCII letter into its lowercase key. `Model.Update` calls it for every key press unless the send-back note, the rewrite or the confirmation's message has the keyboard. `ConfirmModel.Update` calls it unless its message has the keyboard. The confirmation's non-typing switch gains `n`, `q` and `ctrl+c` as cancel keys, and its default case sets a `stray` flag that `noticeLines` draws. `applyUI.detailKey` maps `t` to the tweak input and `c` to the reason input, which `A` and `X` opened before. `updateDetail` binds `[` and `]` and drops `N`.

## Constitution Check

- **II**: A stray key no longer cancels, but nothing sends without `y` or `p`, so the confirmation still never publishes unread.
- **V**: No command, flag, envelope or refusal changes.
- **VI**: One helper of a dozen lines and no new dependency.
- **VII**: Tests drive the keys through `Model.Update` and `ConfirmModel.Update`. Real caps lock in a physical terminal is unverified.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `specs/001-loupe-v1/research.md` and `specs/002-review-ux` §5.5 said every unknown key cancels the confirmation | Under caps lock that made publishing impossible. Cancel now takes an explicit key, and nothing still sends without `y` or `p` |
| `specs/051-apply-polish` FR-017 made shift add the human's words | Caps lock turned every decision into its note-taking variant. `t` and `c` replace `A` and `X` |

## Verification

- `mise run check`.
- By eye through `mise run demo` in tmux: a PR run published with uppercase keys only, a stray key on the confirmation, and `t` and `c` on a local run.
- Unverified: `mise run screenshots` and `mise run review-screenshot`, which are not rerun, and caps lock in a physical terminal.
