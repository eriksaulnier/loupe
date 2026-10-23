# Implementation Plan: Remove plain mode

**Branch**: `feat/remove-plain-mode` | **Date**: 2026-10-22 | **Spec**: [spec.md](spec.md)

## Summary

`internal/tui/plain.go` and its tests go, with `ChooseMode` and `--plain`. `loupe review` and `loupe publish` refuse a terminal the full-screen program cannot draw on with `tty`. The tests that drove plain mode, about fifty integration call sites and the CLI's review tests, drive the full-screen program through a new test driver, `internal/testutil/screen`.

## Technical Context

Go, standard library, Bubble Tea v1.3.10, as today. No new dependency: the driver uses `github.com/charmbracelet/x/ansi`, already a direct dependency, to replay frames. The change spans `internal/tui`, `internal/cli`, `internal/integration`, the new `internal/testutil/screen`, the goldens, and the documents FR-008 names.

## Research

- **No size without a terminal.** Bubble Tea v1 asks the output for its size only when the output is a TTY (`tea.go:318`, `tty.go:121`), and it has no option to set one. A program writing to a buffer never gets a `WindowSizeMsg`. Confirmed in the v1.3.10 source.
- **Keys from a pipe.** Bubble Tea reads any `io.Reader` and treats a short read as an event boundary (`key.go:581`). An `io.Pipe` write returns only once a read has taken it, so each write is one read and one key: a lone `\x1b` is esc and does not join the next key into an alt sequence. `screen.TestPressSendsEachKeyAsItself` pins letters, enter, esc, tab, the arrows, page keys, backspace, space and ctrl+c, and typed text arrives as runs of runes with each space a key of its own, which the note box and the message box take as typing. End of input is dropped, so a review reading a pipe runs until a key ends it.
- **Two pauses that drop keys.** Review drops keys for 500ms after it opens (`settleOnOpen`) and decision keys for 250ms after a decision opens the next finding (`settleAfterDecision`). Neither shows on screen, so a test waiting on screen text cannot know they have passed.
- **Frames are diffs.** The renderer rewrites only changed rows, so the byte stream since a wait holds what changed, and a whole frame is reconstructed by replaying rows from the home position.

## Design

- **The seam.** `tui.Screen{Width, Height}` stands in for a terminal. `tui.Config.Screen`, the new last argument of `tui.Confirm`, and `cli.Deps.Screen` carry it. With it set, the program starts at its size, and neither pause applies, because both guard a human's typeahead and a test types only after the screen shows what it waits for. `cli.Deps.Screen` also stands in for the raw-mode probe, which a pipe always fails. Production leaves it nil.
- **The refusal.** `drawable` in `internal/cli/review.go` refuses with `tty` under `TERM=dumb`, or when the probe fails with no `Screen`. Review calls it after the existing terminal check and before it resolves the run. Publish calls it in its confirmation callback, so the gates still run first, as the plain-mode choice did.
- **The driver.** `screen.Start` runs a command on its own goroutine with stdin as a pipe. `Press` sends named keys one write each, `Type` sends text in one write, `WaitFor` blocks until everything drawn since the last wait contains its text, `View` replays the current frame, and `Wait` returns the outputs and exit code. Waits wake on each write, never on a timer; a 10-second guard turns a hang into a failure that prints the screen.
- **Harness helpers.** `harness.Start`, `review`, `decide`, `Publish`, `PublishJSON` and `confirm` replace `--plain` and the line answers. A publish that refuses before its confirmation runs through `Run` with no keys.
- **Tests with no full-screen twin.** An audit of each plain test against the full-screen tests found ten behaviors the full-screen tests did not pin. Each gets an assertion in the full-screen test nearest it.

## Plain-only behaviors left untested

These existed only because plain mode read lines and printed text. They have no full-screen meaning.

- The answer legend, the `f-001 >` prompt and the `Publish this review? [y/N]` prompt.
- Typed lines such as `yes`, ` y` and `y ` that plain mode refused as answers.
- End of input ending a review, and a refused message asked for again with `Your message was:` printed back. The screen keeps the text in its input instead.
- Readiness as a bare `3 pending` line, and the `b` back key. The full-screen program has `←`.
- Wrapping printed text to the window, where the full-screen tests already pin each view's width.

## Assertions whose form changed

- **`TestPublishEndToEnd`** compared the confirmation's printed body with the sent body as one string. The full-screen confirmation wraps prose and draws the message in a box, so the test reads the frame, drops the box border, and compares the two with spaces and line breaks removed. Every other character must match in order; a one-character change to the sent body fails it.
- **Hand-back on end of input** (`internal/cli`) is hand-back on Ctrl-C, the other way a full-screen session ends.

## Constitution Check

- **I, III, IV.** Untouched.
- **II.** Publish still confirms in one interactive command. Removing a front end narrows what a human can confirm through.
- **V.** `--plain` leaves the contract, amended in `contracts/cli.md`. No refusal code is added; `tty` gains two causes with their own fix.
- **VI.** One fewer front end. The driver serves two test packages, `internal/integration` and `internal/cli`, so it lives in `internal/testutil`.
- **VII.** Tests use injected input and a fake terminal size, not a pseudo-terminal. No test sleeps to wait for the program.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `contracts/cli.md` drops `--plain` and its fallback | The owner's decision; this specification is the amendment. |
| Production code carries a test seam, `Screen` | Bubble Tea reports no size and the probe fails without a real terminal, and Principle VII forbids a pseudo-terminal. `LockBusy` is the precedent. |
