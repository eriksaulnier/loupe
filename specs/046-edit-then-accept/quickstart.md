# Quickstart: validate the rewrite

## Automated

```sh
mise run check
```

The tests that cover this spec are in `internal/draft/mutate_edit_test.go`, `internal/tui/rewrite_test.go`, `internal/integration` and `internal/cli/plugin_test.go`. They use injected input and the fake GitHub only.

## By eye, through tmux

```sh
mise run build
LOUPE_DEMO_HOME=$(mktemp -d)/demo
tmux new-session -d -s rw -x 100 -y 30 "LOUPE_DEMO_HOME=$LOUPE_DEMO_HOME mise run demo"
tmux send-keys -t rw Enter        # open the first finding
tmux send-keys -t rw w            # the Rewrite pane opens under the finding
tmux send-keys -t rw Tab ' (edited)'
tmux send-keys -t rw C-s          # saves; the notice says still pending
tmux capture-pane -t rw -p
tmux send-keys -t rw a            # accepts at the new revision
tmux kill-session -t rw
```

Expected:

1. After `w`, a pane titled `Rewrite f-001` shows the title, a divider and the body source. The footer shows `ctrl+s save`, `tab title/body` and `esc cancel`.
2. After `C-s`, the pane closes, the body ends with `(edited)`, and the border notice reads `f-001 rewritten · still pending`.
3. Repeat at `-x 60 -y 12`: the pane and the footer fit, and the finding pane keeps its border.

## Not verified here

- Delivery of a real `ctrl+s` key press through the Orca and Herdr panes (research R1).
- The maintainer checks in `specs/001-loupe-v1/validation.md`.
