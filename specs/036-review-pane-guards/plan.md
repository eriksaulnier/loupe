# Implementation Plan: Review pane guards

**Branch**: `fix/review-pane-guards` | **Date**: 2026-10-22 | **Spec**: [spec.md](spec.md)

## Summary

Review stops falling into plain mode for size and instead draws a "too small" screen until the window grows. `handoff` splits down when no width is known, and its `review-open` guard covers every round of the pull request and names the open run. A publish from the full-screen review ends on a published screen with the review's link, which review also prints on exit and `loupe wait` returns.

## Technical Context

Go, as today. No new dependency: the hyperlink escape comes from `github.com/charmbracelet/x/ansi`, already a direct dependency, and the browser is started with `os/exec`. The change touches `internal/tui`, `internal/style`, `internal/pane`, `internal/run`, `internal/cli`, `cmd/loupe`, the skill, `contracts/cli.md` and `research.md`.

## Design

- **Mode.** `ChooseMode` loses its size triggers, and `Options` loses `Width` and `Height`. `minWidth` and `minHeight` stay in `term.go` and now gate the "too small" screen. `review.go` still reads the size for plain mode's wrap width. The edit to `term.go` is kept small because `specs/037-remove-plain-mode` rewrites the same file.
- **Too small.** `tooSmall(width, height)` and `(*Model).smallView()` live in a new `internal/tui/small.go`. `Model.View()` checks it before the help overlay. `ConfirmModel.View()` checks it before the confirmation, through the shell model it already holds. Key handling uses one rule in both programs: while too small, `q` becomes `ctrl+c` and every other key is dropped. So `q` quits the list, declines the confirmation, and does nothing while a confirmed review is sending, which is what `ctrl+c` already does on each of those. The message is three short lines, wrapped to the width and centered with `lipgloss.Place`. Nothing is reset, and `layout()` already runs on every `WindowSizeMsg`.
- **Direction.** `direction()` returns `down` when neither source gives a width. Its comment and `handoff --help` change with it, and so do the help goldens.
- **Per-PR guard.** `run.OpenSession(root, ref)` returns the open run's ref. It probes the resolved run first, then the pull request's other rounds from `run.Rounds`, newest first, with the existing `SessionOpen`. `runHandoff` calls it in place of `SessionOpen` and puts `details.run` on the refusal. The fix keeps its text when the open run is the resolved run. Otherwise it reads `ask the human to finish or quit loupe review of <open>, then run loupe handoff --run <ref> --json again`.
- **Published screen.** A new `viewPublished` holds the URL. `publishFinished` switches to it on success in place of the list. Its view is the list header, then `Published` and the link, and a footer with `o open in browser` and `any other key quits`. `Config.OpenURL func(string) error` is the opener. When it is nil, the `o` hint is not drawn and `o` quits like any other key. A failed open shows a warning, and the next key quits. `Model.PublishedURL()` reports the URL to the command.
- **Link.** `style.Link(url, text)` wraps `text` in `ansi.SetHyperlink`/`ansi.ResetHyperlink` only when `Color` is true and the URL is `https://` with no control character. Otherwise it returns `text`. Under `NO_COLOR` that is no escape at all, which matches the style package's rule. The text is `render.ForDisplay(url)`.
- **Opener.** `cli.OpenURL` runs `open` on darwin and `xdg-open` on other platforms, and waits for it to exit. `Deps.OpenURL` carries it, and `cmd/loupe` and `cmd/loupe-demo` set it. The demo also sets it, because opening the URL the fake GitHub returns sends nothing.
- **Exit line.** `reviewSession` returns the published URL. After the session is released, `reviewDone` prints `✓ Published <url>` on the interface's output, which is stdout, or stderr under `--json`. The `--json` payload does not change.
- **wait.** On `published`, `pollWait` reads the receipt with `publish.LoadReceipt`. The JSON payload gains `reviewUrl`, and the human line gains the URL.
- **Skill.** §6 and §7 as the spec says. `plugin_test.go` pins those lines, so it changes with them.

## Constitution Check

- **I.** The opener is the platform's own URL handler, not an agent. Host-neutral.
- **II.** Unchanged. The published screen appears only after the human confirmed and GitHub accepted.
- **III.** No new file or state. `wait` reads the receipt that is already there.
- **IV.** Untouched.
- **V.** `wait` adds a key and `review-open` adds `details.run`. No key, code or exit changes meaning. Both are recorded in `contracts/cli.md`.
- **VI.** No new dependency or record version. `OpenSession` reuses `SessionOpen` and `Rounds`.
- **VII.** Tests use `teatest`, the fake GitHub, an injected opener and real flocks on temporary directories. No pseudo-terminal and no browser.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `specs/019-pane-hosts` FR-005: an unknown width chooses `down`, not `right` | Review needs 60 columns but only 12 rows. Orca reports no width, so `right` halved a pane that was often already narrow. |
| `specs/017-live-review` FR-012: the guard covers every round of a pull request | A new round is a new run, so the per-run guard let an agent open a second pane on the same pull request. |
| `research.md` plain mode: size no longer chooses plain mode | Plain mode in a narrow pane read as a frozen pane. The "too small" screen says what is wrong. |
