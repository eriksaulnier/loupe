# Research: Pane hosts

**Feature**: `specs/019-pane-hosts` | **Date**: 2026-09-21

## Orca facts

Recorded on 2026-09-21 inside the owner's Orca session, from the managed shell's environment, `orca --help` and Orca's own `orca-cli` guide, and one `show` against a stale handle. The split itself is under "Live probe".

- **Environment.** Managed shells carry `ORCA_TERMINAL_HANDLE=term_<uuid>` (the agent's terminal), `ORCA_PANE_KEY`, `ORCA_TAB_ID`, `ORCA_WORKTREE_ID` and `TERM_PROGRAM=Orca`. `orca` is on `PATH` from `$ORCA_REMOTE_CLI_BIN_DIR`. tmux overwrites `TERM_PROGRAM`, and Claude Code teammates spawn into tmux, so only `ORCA_TERMINAL_HANDLE` and `orca` on `PATH` are reliable.
- **Split.** `orca terminal split --terminal <handle> --direction horizontal|vertical --command <text> --json`. `vertical` is left-and-right and `horizontal` is top-and-bottom: the word names the divider, not the placement. The `orca-cli` guide states the opposite and is wrong, see the live probe below. There is no `--focus` and no `--env`. `--command` is typed into the new shell, which loads the user's profile, so `LOUPE_HOME` MUST travel in the command line.
- **Switch.** `orca terminal switch --terminal <handle> --json` focuses a terminal.
- **Show.** `orca terminal show --terminal <handle> --json` validates a handle and opens nothing. A stale handle prints `{"ok":false,"error":{"code":"terminal_handle_stale","message":...}}` on **stdout** and exits 1.
- **Size.** No command reports a terminal's columns: `show` carries no size and `list --include-visual-layouts` carries topology only. On Orca only the agent's own controlling terminal can give a width (see "Width sources").
- **Handshake line.** Every call prints `[relay-connect] Handshake OK at version=...` on stderr, on success and on failure.

## Live probe

One split ran on 2026-09-21 in the owner's Orca session, with the owner's approval: `orca terminal split --terminal "$ORCA_TERMINAL_HANDLE" --direction horizontal --command 'echo probe && exit' --json`. It settles the three questions left open by the facts above.

- **Split result shape.** Stdout, exit 0: `{"id":"…","ok":true,"result":{"split":{"handle":"term_28c2bdd2-…","tabId":"…","paneRuntimeId":1,"leafId":"…"}},"_meta":{"runtimeId":"…"}}`. The new handle is at `result.split.handle`, not the expected `result.terminal.handle`. Stderr carried only `[relay-connect] Handshake OK at version=0.1.0+955da90487c0`.
- **Pane closing on shell exit.** It closes. `orca terminal show --terminal <new>` right after the split returned `ok:true` with `result.terminal.{handle, connected:true, writable:true, paneRuntimeId:2, …}`. After the shell ran `exit`, the same call still returned `ok:true`, now with `orphaned:true, connected:false, writable:false, paneRuntimeId:-1, exitCause:{kind:"unknown",reason:"cause_unreported"}`, and `orca terminal list` no longer listed the handle. So `&& exit` gives 003 FR-015's lifetime on Orca.
- **`show` on an exited terminal succeeds.** The `probe` step catches only a stale or unknown handle (`ok:false`, `error.code: terminal_handle_stale` on stdout, exit 1), not an exited one. That is enough: the handle probed is the agent's own, which is live while the agent runs.
- **Direction words.** A second live split on 2026-09-21, sent as `--direction vertical` for a `down` request under the guide's mapping, opened to the right of the agent pane, and `orca terminal list --include-visual-layouts` reported the node as `{"type":"pane-split","direction":"vertical"}`. So `vertical` is left-and-right and `horizontal` is top-and-bottom, and `right` maps to `vertical`.
- **Focus on split.** Observed on 2026-09-21: the review pane took focus after the `switch` step, and `q` in review closed it. Not observable from the CLI: no command reports which terminal has focus, and `list` has no focus field. The `switch` step makes the answer moot.

## Focus

Recorded on 2026-09-22 from live probes in the owner's Orca session. `orca terminal split` never moves the view: two splits into a background tab changed no group's `activeTabId`, and the owner saw nothing move. `orca terminal switch` is the only call that moves the view, and it has no option to focus without bringing the tab to the front. `orca terminal list --include-visual-layouts --json` reports, under `result.visualLayouts[]`, one entry per worktree whose `root` is `{"type":"group","groupId":…,"activeTabId":…,"tabs":[{"tabId":…,"title":…,"activeLeafId":…,"panes":…}]}`. Managed shells carry `ORCA_TAB_ID`, the agent's tab. Nothing reports which group's window is in front. Decision (owner's ask, spec FR-024): run `switch` only when the agent's tab is the active tab of its group, and otherwise open the pane in that tab and leave the view alone. loupe walks each layout whole rather than reading only `root`, so a worktree whose root holds several groups still resolves. Alternatives: always switch (pulls the human off the tab they are in), never switch (a foreground handoff loses focus it had before).

## Decisions

- **Width sources** (amended 2026-09-21, superseding the tty-only decision). Decision: the host's own report of the agent pane's width first (Herdr's `pane layout`; Orca has none), then the agent's tty through `pane.TTYWidth` (`/dev/tty` and `term.GetSize`), then `right`. Rationale: the tty-only rule never gave `right` from an agent's shell; see "Width sources" below. Under `--json` stdout is a pipe, so the stdout width `Deps.TermWidth` reads is not the agent pane's either. `golang.org/x/term` is already a direct dependency. Alternatives: tty only, with unreadable as `down` (every Claude Code handoff opens `down` on both hosts, and Herdr loses the width spec 006 already read).
- **Detection order.** Decision: Herdr, then Orca. Rationale: a Herdr session started in an Orca terminal gives its panes both environments and the agent sits in the Herdr pane; Orca cannot run inside Herdr.
- **Error text.** Decision: Herdr reads `error.message` from stderr JSON, else trimmed stderr, as today. Orca reads `error.message` from stdout JSON, else the last stderr line that does not start with `[relay-connect]`, else the exit error. Rationale: FR-011 and the stale-handle and handshake facts above.
- **`LOUPE_HOME` in the command line on both hosts.** Decision: `LOUPE_HOME='<abs root>' '<exe>' review '<ref>' && exit`, built once in `cli`, and Herdr drops `--env`. Rationale: Orca has no `--env`, and one host-neutral command removes a host-specific path. The `NAME=value command` prefix works in zsh, bash and fish 3.1+, the shells 003 already requires.
- **Refusal message.** Decision: `<host> <step>: <message>` for every host, built by one function, so Herdr's `herdr pane layout: ...` becomes `herdr probe: ...`. Rationale: one builder for every host; the contract pins `details`, not the message text.

## Width sources

Observed on 2026-09-21 in the owner's Orca session. From Claude Code's Bash tool loupe has no controlling terminal: opening `/dev/tty` fails with "no such device or address", `COLUMNS` is 0, and stdin, stdout and stderr are pipes. Orca reports no pane size in any command. So a rule that reads only the agent's tty can never yield `right` from an agent's shell, on either host; Herdr only ever opened `right` because spec 006 read the pane width from `herdr pane layout`. The owner's decision the same day: take the host's report of the agent pane's width first, then the tty, and open `right` when neither gives a width (spec FR-005).
