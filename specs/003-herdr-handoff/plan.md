# Implementation Plan: Herdr review handoff

**Branch**: `003-herdr-handoff` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/003-herdr-handoff/spec.md`

## Summary

Inside Herdr, the `loupe` skill opens `loupe review <ref>` for the human in a split beside the agent pane, then blocks on `loupe wait` as today. Outside Herdr nothing changes. The change is skill text, the tests that pin it, a repository guard that keeps Herdr out of the binary, and two documentation notes. No Go code in the binary changes. The Herdr plugin and its picker are deferred (spec, Deferred).

## Technical Context

**Language/Version**: Markdown (skill), Go 1.25 tests, Bash (guard).

**Primary Dependencies**: None added. Herdr 0.9.0 is a host the skill drives through its CLI, not a dependency of loupe.

**Storage**: N/A.

**Testing**: `internal/cli/plugin_test.go` pins the skill's phrases and whole-line prohibitions. `scripts/check-tests.sh`, run by `mise run check`, gains the Herdr guard.

**Target Platform**: Herdr 0.9.0 panes on Linux and macOS, with a shell that supports `&&` (zsh, bash, fish 3+).

**Project Type**: CLI plus Claude Code plugin.

**Constraints**: FR-017: no Herdr path in the binary. SC-004: the non-Herdr handoff text is unchanged.

**Scale/Scope**: One skill file, one test file, one script, README, validation.md.

## Research

Observed in Herdr 0.9.0 on 2026-09-15 from inside a Claude Code pane, using scratch panes only.

- **Pane lifetime.** Decision: run `loupe review '<ref>' && exit` in the split's shell. Spike: a split running `true && exit` closed within about two seconds; a split running `echo 'o/r#1@1' && false && exit` was still open after five seconds with its output on screen, and was then closed by the spike. Rationale: `herdr pane split` takes no command and has no keep-open option, so the shell's own exit is the only close signal, and `&&` turns review's exit status into FR-015's behavior. Alternatives: `pane send-text` plus Enter (the same effect in two calls, and a second text send to the pane); a wrapper script shipped with the plugin (needs the plugin, which is deferred).
- **Quoting.** Decision: single-quote the ref. `echo 'o/r#1@1'` printed `o/r#1@1` under zsh. Rationale: with `EXTENDED_GLOB`, an unquoted `#` is a glob operator. Refs contain only `/`, `#`, `@`, digits and GitHub name characters, none of which need escaping inside single quotes.
- **Direction.** Decision: `herdr pane layout --pane "$HERDR_PANE_ID"` and read `rect.width` of the entry whose `pane_id` is the agent's own; split `right` when it is at least 120, else `down`. Rationale: each half of a right split keeps spec 002's 60 columns. The result lists every pane in the tab, so the entry MUST be matched by id. The skill does not require `jq`; the agent reads the JSON.
- **Environment.** Decision: pass `--env LOUPE_HOME=…` and `--env XDG_DATA_HOME=…` for whichever is set in the agent's environment. Rationale: the split's shell loads the user's profile, not the agent's environment, and these are the two variables `run.DataRoot` reads before `HOME`. Without them an agent pointed at a scratch data root would open review against the default one.
- **Socket access.** Claude Code's Bash tool reached the Herdr socket: `pane layout`, `pane split`, `pane run`, `pane get` and `pane close` all worked. A user in a stricter permission mode MAY allow `Bash(herdr pane layout:*)`, `Bash(herdr pane split:*)` and `Bash(herdr pane run:*)`; otherwise a denied call takes FR-008's fallback. Superseded by `specs/006-agent-plugins`: agents run `loupe handoff`, which one allow rule covers, and no `herdr` rule is recommended.
- **Dry run of section 6.** The skill's commands were run as written in a scratch pane, with `loupe list` in place of `loupe review`. The agent pane was 106 wide, so the split went `down` and took focus. `herdr pane run` sent immediately after the split still ran once the shell started. `loupe list 'o/r#1@1' && exit` refused with `usage` and the pane stayed open showing `error:` and `fix:`; `loupe list && exit` closed the pane in about two seconds and focus returned to the agent pane. A split with `--env "LOUPE_HOME=/tmp/loupe marker"` saw that exact value. The `right` branch was not exercised, because no wider pane was available.
- **Failure shape.** API errors print a JSON `error` object on stderr and exit 1 (observed as `pane_not_found`). Any non-zero exit or missing `pane_id` is a failure that takes the fallback.

## Constitution Check

- **I. A tool for agents.** PASS. Herdr lives only in the plugin's skill text. The guard fails the check if a non-test Go file under `cmd/` or `internal/`, or `go.mod`, names Herdr.
- **II. Nothing posts on its own.** PASS. The agent opens review; the human decides every finding and confirms publication in review. The skill still forbids running `loupe publish` and scripting confirmation, and adds that the agent MUST NOT send to, read, resize or close the review pane. The spec's "Why the human-only rule can relax" records that this rests on instructions, not a mechanism.
- **III–V.** Unaffected: no run state, checkout or contract change.
- **VI. Simplicity.** PASS. No dependency, no new command, no plugin.
- **VII. Verified means ran.** PASS with a gap: tests pin text only and MUST NOT drive a real Herdr. The live round is added to the Unverified list (FR-020).

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/003-herdr-handoff/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

plugin/skills/loupe/SKILL.md      # Rules and section 6
internal/cli/plugin_test.go       # pinned phrases and prohibition lines
scripts/check-tests.sh            # Herdr guard
README.md                         # Install note
specs/001-loupe-v1/validation.md  # test row and Unverified bullet
```

**Structure Decision**: No new directories. No data-model, contracts or quickstart changes: the CLI contract and run layout are untouched.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| SC-005 is read as "no non-test Go file under `cmd/` or `internal/`, and not `go.mod`, names Herdr" | `internal/cli/plugin_test.go` pins skill text that now names Herdr | Moving the skill tests out of `internal/` breaks the established test layout for one word |
