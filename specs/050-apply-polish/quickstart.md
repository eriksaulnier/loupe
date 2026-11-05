# Quickstart: Apply-mode polish

## Prerequisites

The worktree on `050-apply-polish`, with `mise install` done.

## 1. The flow, by eye

```sh
export LOUPE_DEMO_HOME=/tmp/loupe-050
mise run demo -- review 'acme/widgets:local#1'
```

1. The start screen shows the title, the "local · returns to Claude" badge, the summary and the count. Press Enter.
2. The first pending finding opens, led by "Claude will:". Press `a`; the next opens. Press `A`, type a note and press Enter. Press `x`. Press `X`, type a reason and press Enter.
3. After the last decision, the return step opens. Type a closing note and press Enter. Review quits and prints "Returned to Claude: …".
4. Run `mise run demo -- feedback --run local#1 --json` and find `note` and `applyWithChanges`.

## 2. PR runs untouched

```sh
mise run demo
```

The PR run looks exactly as before: no badge, `t` is not offered, `w` rewrites.

## Automated

`mise run check`. `go test ./internal/cli/ ./internal/tui/` with no `-update` keeps every PR golden.
