# Quickstart: validate apply mode

These are the by-eye checks for PR 5 and later. Integration tests automate sections 1 to 3.

## Prerequisites

1. `mise run build`. Inside the repository, `loupe` is then `dist/loupe`.
2. `export LOUPE_HOME=$(mktemp -d)`, so that the runs stay out of the real data root.
3. A scratch Git repository with a `main` branch and a feature branch `feat` that has one commit on top of `main`. No `GITHUB_TOKEN` is needed.

## 1. Local code review (User Story 1)

1. In the scratch repository, make three changes:
   - Edit tracked file `a.txt` without staging it.
   - Edit tracked file `b.txt` and stage it.
   - Create untracked file `c.txt`.
2. Snapshot the user's state, in this order:

   ```sh
   sha256sum .git/index > /tmp/before
   git --no-optional-locks status --porcelain=v2 >> /tmp/before
   git for-each-ref >> /tmp/before
   git rev-parse HEAD >> /tmp/before
   git ls-files -z | xargs -0 sha256sum >> /tmp/before
   sha256sum c.txt >> /tmp/before
   ```

3. Run `loupe capture --changed --base main --applier Claude --json`.
   - Expect `ok: true` and a `run` of the form `<prefix>:local#1`.
   - Expect `files` to hold `a.txt`, `b.txt` and `c.txt`, with only `c.txt` marked `new: true`.
4. Repeat step 2 into `/tmp/after`. `diff /tmp/before /tmp/after` prints nothing (SC-005).
5. Run `loupe add --run <ref> --from f.json --json` with two findings that carry a `proposal`.
6. Run `loupe handoff --run <ref>`. In the pane:
   - The first card shows "If you accept, Claude will" under its title. Press `a`.
   - On the second card, press `x`, then Enter.
   - In the list, press `p`, then `y`. The screen says "Returned to Claude". Press `q`.
7. Run `loupe wait --run <ref> --json`. It returns `reason: "returned"`.
8. Run `loupe feedback --run <ref> --json`. It holds one entry in `apply` and one in `excluded`, each with an absolute `path`.
9. Run `loupe add --run <ref> --from f.json --json`. It refuses with `returned`. `loupe review <ref>` refuses with `returned` too.

## 2. Plan decisions (User Story 2)

1. Run `loupe capture --files ~/.claude/plans/<some-plan>.md --json`.
   - Expect `ok: true`, and `files[0].path` of the form `~/.claude/plans/<some-plan>.md`, with `abs` absolute.
   - Expect `headings` for that file.
2. Run `loupe add --run <ref> --from f.json --json` with two findings: one anchored to a heading with a proposal, and one general finding with no proposal.
3. Run `loupe handoff --run <ref>`. In the pane:
   - The code pane shows plain text, with no `+` gutter.
   - On the first finding, press `t`, type a note and press Enter.
   - On the second, press `a`.
   - Press `p`, then `y`.
4. Run `loupe feedback --run <ref> --json`. It holds the first finding in `applyWithChanges` with `suggestion`, and the second in `apply`.

## 3. No files (User Story 3)

1. `loupe capture --no-files --title "Cache design" --json` succeeds.
2. `loupe add --run <ref> --from f.json --json` with a finding that has a `location` refuses, and the fix names `"general": true`.
3. `loupe capture --no-files --json` refuses with `usage`, and the fix names `--title`.

## 4. PR runs untouched

1. `go test ./internal/cli/` with no `-update` passes. Every golden except `help.*` and `handoff-help.*` is byte-identical (SC-003).
2. Run `mise run demo`. On `acme/widgets#42`:
   - No `t` key appears.
   - `x` still excludes at once.
   - `e` and `o` work as before.

## 5. Cost of `--changed`

1. In a repository of about 50,000 tracked files, edit one tracked file.
2. Run `time loupe capture --changed --json`, and record the time here.

The expected cost is close to `git add -A` into an empty index, because the temporary index starts with no stat cache.

## Automated

`mise run check`. The integration tests cover sections 1 to 3 with injected input. They assert that the fake GitHub, which also serves the release endpoint, receives no request (SC-004).
