# Quickstart: validate heading anchors

## Automated

```sh
mise run check
go test ./internal/markdown/ -run Heading
go test ./internal/diff/ -run Heading
go test ./internal/integration/ -run Heading
go test ./internal/publish/ -run Schema
go test ./internal/cli/            # goldens: only help output may change
git diff --stat testdata/golden    # MUST show no published-format golden
```

The integration tests capture a local Git repository through the fake GitHub, so they exercise `outline.json`, `add`, `edit`, `show --json` and a fake publish end to end. Expected results:

- A heading whose section one hunk changes stores `{path, side: "RIGHT", line, startLine, heading}` (US1).
- Each refusal of US2 returns `location` or `input` with the fix and details of research R6, and applying the fix stores the findings on the next call (SC-003).
- The fake GitHub receives a comment with `start_line` and `line` only, and the receipt holds no `heading` (FR-016).

## By eye

```sh
LOUPE_DEMO_HOME=<scratch>/demo mise run demo -- list   # seeds the root
# add "heading": "Rollout" to one finding's location in that root's acme/widgets#42 draft.json
LOUPE_DEMO_HOME=<scratch>/demo mise run demo -- review acme/widgets#42
```

The demo's diff holds no Markdown file, so the by-eye check sets a heading on a seeded finding by hand; the resolution itself is covered by the automated tests. Read that finding's detail: the code pane's border shows `path:start-end`, and its first row shows `· Rollout`. Open the whole-file diff and check that the mark sits on the range's last line. Check a second finding's detail is unchanged, and repeat under `NO_COLOR=1`.

## Live probe (FR-022, owner-approved, once)

```sh
loupe capture https://github.com/eriksaulnier/loupe-probe/pull/1 --json
loupe add --run eriksaulnier/loupe-probe#1 --from probe.json --json   # a heading anchor whose range starts on a context line
loupe show --run eriksaulnier/loupe-probe#1 --json                    # confirm startLine is a context line in pr.diff
# the owner accepts the finding in loupe review, then:
loupe publish eriksaulnier/loupe-probe#1 --action comment --inline all
```

Record the date, the review id and GitHub's response in `docs/github-facts.md` under Inline review comments.
