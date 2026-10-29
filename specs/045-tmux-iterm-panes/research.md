# Research: tmux and iTerm2 pane hosts

**Feature**: `specs/045-tmux-iterm-panes` | **Date**: 2026-10-28

## tmux samples

Recorded on 2026-10-28 with tmux 3.4 on Linux (WSL2), once, by hand, against a throwaway server on its own socket (`tmux -L loupe045sample`), started detached at 150 by 40 and killed afterwards. No test starts a tmux server.

- **Environment inside a pane.** `TMUX=/tmp/tmux-1000/loupe045sample,3347196,0`, `TMUX_PANE=%0`, `TERM=tmux-256color`, `TERM_PROGRAM=tmux`. The socket path is the text before the first comma, as sift reads it. tmux overwrites `TERM_PROGRAM`, so the iTerm2 condition fails inside tmux on its own (spec Edge Cases).
- **Probe of a live pane.** `tmux -S <sock> display-message -p -t %0 '#{pane_id} #{pane_width} #{session_attached}'` prints `%0 150 0` and exits 0. `session_attached` is `0` for a detached session.
- **Probe of a missing pane.** `display-message -p -t %99 '[#{pane_id}] [#{pane_width}]'` prints `[] []` and **exits 0** with nothing on stderr. A bad target is not an error for `display-message -p` in tmux 3.4. So a probe that only read the width would take a dead `TMUX_PANE` as a pane of unknown width and split some other pane.
- **Split.** `split-window -d -h -P -F '#{pane_id}' -t %0` prints `%1` and exits 0. Afterwards `%0` has `pane_active` 1 and `%1` has 0, so `-d` keeps the agent's pane active, which the spec's Assumptions read only from `new-window`'s wording. A 150-column pane split `-h` gives halves of 75 and 74.
- **Stale socket.** After `kill-server` the socket file stays on disk. A probe against it prints `no server running on /tmp/tmux-1000/loupe045sample` on stderr and exits 1. With the file removed, it prints `error connecting to /tmp/tmux-1000/loupe045sample (No such file or directory)` and exits 1, but detection never gets that far, because FR-002 skips a socket path that does not exist.

## Decisions

### The probe echoes the pane id

- **Decision**: The probe format is `#{pane_id} #{pane_width} #{session_attached} #{pane_synchronized}`. The reply is readable only when it has four space-separated fields, the first equals `TMUX_PANE`, and the third is a non-negative integer. A fourth field of `1` means the pane is synchronized, and anything else means it is not (added after review of #89, 2026-10-29). Otherwise the probe refuses `pane-failed` at `probe` with `unreadable reply` and the reply quoted, or, when the first field is empty, `tmux has no pane %N`. A width that is not a positive integer is unknown under FR-008, not a refusal.
- **Rationale**: tmux 3.4 exits 0 with empty fields for a missing pane (sample above), so the id echo is the only way to see that `TMUX_PANE` is dead. This keeps acceptance scenario 1.3's outcome, a refusal at `probe` that opens nothing. Its message is loupe's, because tmux prints none. Herdr's probe does the same check against its layout ("the result does not list pane").
- **Alternatives considered**: `list-panes -F` with a filter, which returns the same nothing for a missing pane and lists every pane to find one. `has-session`, which checks the session, not the pane.

### Steps and argv

Every call is `tmux -S <socket> <command> ...` (FR-004). The step key a fake reads is therefore the third argument.

| Step | argv after `tmux -S <socket>` |
| :--- | :--- |
| `probe` | `display-message -p -t <TMUX_PANE> '#{pane_id} #{pane_width} #{session_attached} #{pane_synchronized}'` |
| `split` | `split-window -d -P -F '#{pane_id}' -h\|-v -t <TMUX_PANE>` |
| `run` | `set-option -p -t <new> synchronize-panes off` when the probe reports the pane synchronized, then `send-keys -t <new> -l <command>`, then `send-keys -t <new> Enter` |

- **Synchronized panes.** A throwaway tmux 3.4 server sampled on 2026-10-29, with `synchronize-panes on` in the window: `#{pane_synchronized}` prints `1` (`0` with it off). After `split-window -d`, the two `send-keys` calls to the new pane also typed the line into the first pane. With `set-option -p -t <new> synchronize-panes off` before them, the first pane received nothing.
- **Decision**: `run` is two calls under one step name, three when the pane is synchronized. `-l` makes every key literal, so Enter cannot ride in the same call. Either failing refuses at `run`.
- **Alternatives considered**: one invocation joined with tmux's `;` separator. It saves one exec and makes a fake parse tmux's command grammar.

### The unattended skip lives in `internal/pane`

- **Decision**: `Detect(getenv, goos)` returns every host whose conditions are all met, in order. `pane.Open(ctx, hosts, req)` opens in the first. A tmux probe that reports zero attached clients returns an unexported sentinel, and `Open` moves on to the next host. When every host passed, `Open` returns a nil host, and `handoff` refuses `no-pane-host` with the same message as for no host.
- **Rationale**: FR-005 makes detection depend on a probe result, which a pure `Detect` cannot see. Keeping the loop in `pane` keeps `handoff` free of host logic (FR-015), and only tmux ever passes. Herdr inside tmux never reaches tmux, because Herdr refuses or opens and never passes.
- **Alternatives considered**: running the tmux probe inside `Detect`, which makes detection run a process and the probe run twice. An exported sentinel with the loop in `handoff`, which moves host order into `cli`.

### The operating system rides in `Deps`

- **Decision**: `Deps.GOOS string`, set to `runtime.GOOS` by `cmd/loupe` and `cmd/loupe-demo`. iTerm2 is detected only when it is `darwin`. A zero `Deps` is not macOS, so no existing test meets iTerm2's conditions by accident.
- **Alternatives considered**: reading `runtime.GOOS` inside `pane`, which a Linux test cannot change (FR-003).

### iTerm2 scripts

- **Decision**: Three AppleScript sources, one per step, held as Go constants. Each runs as `osascript -e <line> -e <line> ... <arg> ...`, one `-e` per line, which is how osascript(1) documents a multi-line script. Each script reads its values from `on run argv`, so no value is spliced into source. A shared `findSession` handler walks windows, tabs and sessions for the one whose `unique id` matches, and raises `error "no iTerm2 session has id <id>"` when none does. `probe` returns that session's `columns`. `split` tells it to `split vertically with default profile` for `right` or `split horizontally with default profile` for `down`, then tells the agent's session to `select`, and returns the new session's `unique id`. `run` tells the new session to `write text <command>`. Selecting in `split` rather than after `write text` came from review of #89 (2026-10-29): it keeps the time the new session may hold focus to the one `osascript` call. No script calls `activate`.
- **Session id**: the text after the last colon of `ITERM_SESSION_ID`.
- **Error text**: osascript prints `<script>: execution error: <message> (<code>)` on stderr and exits 1. loupe carries the trimmed stderr, else the exit error. A refused Apple event arrives the same way, with code `-1743`.
- **Step key for fakes**: the `run` script contains `write text`, the `split` script contains `split vertically`, and the `probe` script has neither. The CLI fake matches on those, in that order.
- **Unverified**: none of this has run against iTerm2. The whole host is on the Unverified list (FR-018).

### The host guard

- **Decision**: `scripts/check-tests.sh` greps `herdr|orca|tmux|iterm|osascript` over non-test Go files outside `internal/pane` and `internal/cli/handoff.go`, then drops the one `internal/style/style.go` line whose text contains `"TMUX",` inside its environment key list. A new mention of tmux in `style.go`, such as a comment, still fails.
- **Alternatives considered**: excluding `internal/style` by path, which FR-015 forbids.

### Where the docs section lives

FR-017 names the README's "Handing off in a terminal pane" section. That section moved to `docs/review-workflow.md` before this feature, and the README links to it. The change lands there, and its permission advice gains `tmux split-window`, `tmux send-keys` and `osascript` beside `herdr pane split`.
