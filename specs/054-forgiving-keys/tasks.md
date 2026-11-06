---
description: "Task list for forgiving keys"
---

# Tasks: Forgiving keys

**Input**: [spec.md](spec.md), [plan.md](plan.md)

- [X] T001 Add `foldLetter` in `internal/tui/keys.go` and call it from `Model.Update` and `ConfirmModel.Update` outside text inputs (FR-001, FR-002).
- [X] T002 Make the confirmation cancel only on `n`, `q`, Ctrl-C and end of input, draw the stray-key notice, and put `n cancel` in its footer (FR-004, FR-005).
- [X] T003 Replace `A` with `t` and `X` with `c` on local runs, in the keys, the footer, the help and the settle keys (FR-006).
- [X] T004 Bind `[` and `]` in the detail and drop `N`, in the keys and both help sections (FR-007).
- [X] T005 Update the tests and goldens, then `docs/review-workflow.md`, `specs/001-loupe-v1/research.md`, `specs/002-review-ux/spec.md` and `specs/051-apply-polish/spec.md`.
- [X] T006 Run `mise run check` and drive `mise run demo` through tmux.
