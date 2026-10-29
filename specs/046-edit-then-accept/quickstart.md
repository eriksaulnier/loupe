# Quickstart: validate the rewrite

## Automated

```sh
mise run check
```

The tests that cover this spec are in `internal/draft/mutate_edit_test.go`, `internal/draft/derive_test.go`, `internal/tui/rewrite_test.go`, `internal/integration/rewrite_test.go`, `internal/cli/show_test.go` and `internal/cli/plugin_test.go`. They use injected input and the fake GitHub only.

## By eye, through tmux

```sh
mise run build-demo
export LOUPE_DEMO_HOME=$(mktemp -d)/demo
tmux new-session -d -s rw -x 100 -y 30 "dist/loupe-demo; sleep 600"
tmux send-keys -t rw Enter                 # open the first finding, f-002
tmux send-keys -t rw w                     # the Rewrite pane opens under the finding
tmux send-keys -t rw -l ' (edited)'
tmux send-keys -t rw Tab
tmux send-keys -t rw -l ' Human line.'
tmux send-keys -t rw C-s                   # saves; the notice says still pending
tmux capture-pane -t rw -p
tmux send-keys -t rw a                     # accepts at the new revision
tmux kill-session -t rw
```

Expected:

1. After `w`, a pane titled `Rewrite f-002` shows the title, a divider and the end of the body source. The footer shows `ctrl+s save`, `tab title/body` and `esc cancel`.
2. After `C-s`, the pane closes, the title ends with `(edited)`, the body ends with `Human line.`, and the border notice reads `f-002 rewritten · still pending`.
3. After `a`, `draft.json` holds f-002 at `rev` 2 with an accept at `findingRev` 2, and its newest history entry is by `human` with `title` and `body`.
4. Repeat at `-x 60 -y 12`: the pane and the footer fit, and the finding pane keeps its border.

Ran on 2026-10-28 at 100x30 and 60x12, with each expectation seen in the captures.

## Not verified here

- Delivery of a real `ctrl+s` key press through the Orca and Herdr panes (research R1). tmux and the integration test's pipe both deliver the byte `0x13`, which Bubble Tea reads as `ctrl+s`. Neither is a person's keyboard in those hosts.
- The maintainer checks in `specs/001-loupe-v1/validation.md`.
