# Quickstart: tmux and iTerm2 pane hosts

How to check this feature. The behavior is in [spec.md](spec.md), and the contract change lands in `specs/001-loupe-v1/contracts/cli.md`.

## Automated

1. `go test ./internal/pane/ ./internal/cli/ -run 'Handoff|Open|Detect|Direction|Skill|Tmux|ITerm'`. Expected: with the fake `tmux`, the calls are `display-message`, `split-window`, `send-keys -l` and `send-keys Enter`, with `set-option -p ... synchronize-panes off` before the `send-keys` when the probe reports a synchronized pane, each with `-S <socket>`, and the result carries `host: tmux` and `focused: false`. A zero-client probe passes to Orca or refuses `no-pane-host`. With the OS injected as `darwin`, the fake `osascript` sees three calls with the session id and command as arguments, and the result carries `host: iterm2`.
2. `go test ./internal/cli/ -update`, then `git diff testdata/golden/cli`. Expected: only `handoff-help.80.txt` and `handoff-help.100.txt` move.
3. `mise run check`. Expected: green, including the widened host guard.

## Live, in tmux (owner)

1. In a tmux pane at least 120 columns wide: `LOUPE_DEMO_HOME=.demo mise run demo -- handoff 'acme/widgets#42' --json`. Expected: `host: tmux`, `direction: right`, `focused: false`, and the agent's pane stays active.
2. Press `q` in review. Expected: the pane closes.
3. Hand off a published run so review refuses. Expected: the pane stays open with `error:` and `fix:`.
4. Repeat in a pane narrower than 120 columns. Expected: `direction: down`.

## Live, in iTerm2 on macOS (owner)

1. In an iTerm2 session with no tmux, run step 1 above. Expected: `host: iterm2`, a split beside the agent, and keyboard focus still on the agent's session. macOS may ask to allow loupe's Apple events once.
2. Repeat steps 2 to 4.

Record each result in `specs/001-loupe-v1/validation.md`. Any step not run stays on its Unverified list (spec FR-018).
