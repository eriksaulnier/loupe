# Implementation Plan: loupe skill for Codex and Pi

**Branch**: `004-agent-hosts` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/004-agent-hosts/spec.md`

## Summary

Codex and Pi install the existing `loupe` skill with the common `skills` installer, which puts it in `~/.agents/skills`. Both hosts were observed to discover it there, so no manifest, package or binary change is needed. The change is one skill rule for sandboxed shells, a stricter frontmatter test, a README install section and a validation entry.

## Technical Context

**Language/Version**: Markdown (skill, README), Go 1.25 tests.

**Primary Dependencies**: None added. `skills` (vercel-labs, run through `npx`) is a user-side installer the README names, not a loupe dependency.

**Storage**: N/A.

**Testing**: `internal/cli/plugin_test.go`.

**Target Platform**: Codex 0.154 and Pi 0.84. Observed on Linux only; macOS, where Codex uses a different sandbox, is unobserved.

**Constraints**: FR-001 single skill source; FR-006 no binary change.

**Scale/Scope**: One skill line, one test file, README, validation.md.

## Research

Observed on 2026-09-15 on Linux with a scratch `HOME`, `DO_NOT_TRACK=1`, and no live pull request.

- **Installer discovery.** `npx skills@1.5.26 add <repo path> --list` found `plugin/skills/loupe` and also the ten `.claude/skills/speckit-*` skills. Decision: the README passes `--skill loupe`. Without it, `-y` installs all eleven.
- **Install layout.** `add <path> -g -a codex -a pi -y` copied the skill to `~/.agents/skills/loupe` (reported as "universal: Codex") and symlinked `~/.pi/agent/skills/loupe` to it. `add eriksaulnier/loupe --skill loupe -g -a codex -a pi -y` did the same from GitHub. The user's git config has no credential helper; `skills` 1.5.26 clones with `gh repo clone`, switching to the SSH URL when `gh auth status` reports the ssh protocol, so a logged-in `gh` is what reaches the private repository. Its install telemetry is sent only when an unauthenticated `api.github.com/repos/<owner>/<repo>` call reports the repository public, which a private repository's 404 does not (read in `dist/cli.mjs`, not observed on the wire). Without `-g` the installer targets the project.
- **Codex discovery.** `codex debug prompt-input` under the scratch `HOME` listed `~/.agents/skills` as skill root `r0` and `loupe` with its description. Decision: no `.codex-plugin/plugin.json` (FR-007). Alternative: a Codex plugin beside the Claude manifest, rejected as a second install route with nothing to add.
- **Pi discovery.** Pi's `docs/skills.md` names both `~/.pi/agent/skills/` and `~/.agents/skills/`, keeping the first on a name collision. `pi --mode rpc` answered `get_commands` with `skill:loupe` from `~/.pi/agent/skills/loupe/SKILL.md` and printed nothing on stderr.
- **Codex sandbox.** A probe script ran under `codex sandbox -P :workspace` and `-P :read-only` in this repository. Both blocked `https://api.github.com/`, writes under `~/.local/share`, and `herdr pane layout` over the socket; `loupe list --json` (read-only) succeeded. Without the sandbox all four succeeded. In a scratch repository, `git update-ref` under `-P :workspace` failed with `Read-only file system` on `.git`, so `loupe capture`'s fetch into the clone is blocked too. `-c sandbox_workspace_write.network_access=true` did not open the network under a permission profile. Codex's own model instructions tell it to rerun a sandbox-blocked command with `sandbox_permissions: "require_escalated"` and a justification, and to offer a `prefix_rule` the user can persist. Decision: one skill rule naming the four needs and the escalation route, overriding the follow-`error.fix` rule when the sandbox caused the refusal, plus a ban on redirecting `LOUPE_HOME` or `XDG_DATA_HOME` (FR-003). Rationale: a redirected data root puts runs where the human's `loupe review` cannot see them. Alternative: document `writable_roots` and network config, rejected because it was not observed to work and widens the sandbox for every command. Codex matches rules per command segment and skips commands with substitutions or variable assignments, so section 6's `herdr pane split --env "LOUPE_HOME=$LOUPE_HOME"` likely prompts every time even with a remembered rule; not observed.
- **Pi sandbox.** Pi's `docs/security.md`: "Pi does not include a built-in sandbox." Nothing is needed.
- **Shell timeouts.** Pi's bash tool: "Timeout in seconds (optional, no default timeout)". Codex 0.154 has a run-to-completion shell tool whose `timeout_ms` "Defaults to 10000 ms" and kills the process, and a PTY exec tool that yields after `yield_time_ms` (default 10000) and keeps the process running. Decision: keep the skill's existing line, "run `loupe wait` in the foreground with `--timeout` under the shell's limit, and run it again on a `timeout` refusal". It already covers both hosts; a fixed number would be wrong for one of them.

## Constitution Check

- **I. A tool for agents.** PASS. The binary is unchanged; hosts reach it through their shell.
- **II. Nothing posts on its own.** PASS. The skill's prohibitions are unchanged. Escalation needs the human's approval per command.
- **III–V.** Unaffected: no run state, checkout or contract change.
- **VI. Simplicity.** PASS. No subcommand, manifest or dependency; the README names an installer the user already reaches with `npx`.
- **VII. Verified means ran.** PASS with a gap: install and discovery ran in both hosts; a live round did not and is on the Unverified list (FR-008).

MCP stays deferred, as the constitution's scope section says; nothing here builds toward it.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/004-agent-hosts/
├── spec.md
└── plan.md

plugin/skills/loupe/SKILL.md      # sandbox rule in Rules
internal/cli/plugin_test.go       # Agent Skills limits and the pinned rule
README.md                         # Codex and Pi install section
specs/001-loupe-v1/validation.md  # test row and Unverified bullet
```

**Structure Decision**: No new directories outside the spec. No tasks.md: the change is four files and the plan lists them.

## Complexity Tracking

None.
