# Implementation Plan: loupe handoff and agent plugins

**Branch**: `006-agent-plugins` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/006-agent-plugins/spec.md`

## Summary

`loupe handoff` makes the three Herdr calls the skill used to compose, in Go, behind one command. Herdr lives in a new `internal/pane` package. The plugin's one skill becomes the loupe workflow for any reviewer: its investigate section and the `/loupe` command go, and its hand-off runs `loupe handoff`. Codex gets a plugin manifest and marketplace, and Pi gets a root `package.json`, all versioned by release-please beside the Claude Code manifest. The README drops the `npx skills` route and the `herdr` allow rules.

## Technical Context

**Language/Version**: Go 1.25, Markdown, JSON, Bash (guard).

**Primary Dependencies**: None added. Herdr 0.9.0, Codex 0.154 and Pi 0.84 are hosts, not dependencies.

**Storage**: N/A. `loupe handoff` reads a run and writes nothing.

**Testing**: `internal/pane` unit tests with an injected runner; `internal/cli/handoff_test.go` with a fake `herdr` shell script on the injected `PATH`; the root and `handoff --help` goldens; `internal/cli/plugin_test.go`; `scripts/check-tests.sh`.

**Target Platform**: Herdr panes on Linux and macOS, with a shell that has `&&` (spec 003).

**Project Type**: CLI plus plugins.

**Constraints**: No pseudo-terminal, network or real Herdr in tests (VII). No host interface (VI).

**Scale/Scope**: One package, one command, two refusal codes, one skill, three manifests, docs.

## Research

Herdr facts come from spec 003's research and were re-observed on 2026-09-15 inside a Herdr 0.9.0 pane: `herdr pane layout --pane <id>` printed `{"id":…,"result":{"layout":{…,"panes":[{"focused":true,"pane_id":"wX:p6","rect":{"height":49,"width":143,…}}],…},"type":"pane_layout"}}`, and a bad pane id printed `{"error":{"code":"pane_not_found","message":"pane nonexistent-pane not found"},"id":"cli:pane:get"}`.

- **Package shape.** Decision: `internal/pane` exports `Detect(getenv) (Herdr, bool)` and `(Herdr) Open(ctx, dataRoot, command string) (Opened, error)`, where `Opened` is `{PaneID, Direction}`. `Herdr` holds the resolved `herdr` path, the agent pane id and a `Runner func(ctx, path string, args ...string) ([]byte, error)`. Rationale: `cli` owns run resolution, the executable and output; `pane` owns only Herdr's argv and JSON. Alternatives: a `Host` interface (rejected, VI: one host).
- **Lookup.** Decision: find `herdr` by walking `filepath.SplitList(getenv("PATH"))` for a regular file with an execute bit, and run it by that absolute path. Rationale: `exec.LookPath` reads the process `PATH`, so tests would need `t.Setenv`; every other input already comes from `Deps.Getenv`.
- **Runner.** Decision: the default runner uses `exec.CommandContext` with the process environment, returns stdout, and on failure returns an error carrying `error.message` from Herdr's stderr JSON, or the trimmed stderr when it is not that shape. Rationale: Herdr's own message is what FR-006 asks for.
- **Environment.** Decision: the `herdr` client gets the process environment unfiltered. Rationale: the owner's plan suggested filtering like `internal/gitenv`, but nothing in the environment redirects the Herdr client the way `GIT_DIR` redirects git, and the split's shell does not inherit the client's environment (spec 003). `LOUPE_HOME` reaches the pane only through `--env`. A filter would guard nothing (VI).
- **Refusals.** Decision: `pane` returns `refusal.New(refusal.PaneFailed, "herdr pane <step>: <message>", fix)`; `cli` returns `NoPaneHost` when `Detect` is false. The fix is `ask the human to run loupe review '<ref>'`, built in `cli` and passed to `Open`. Rationale: the refusal leaf exists so domain packages can return refusals.
- **Order.** Decision: resolve the run, then detect, then layout → split → run. Rationale: every refusal can name the resolved ref, and spec scenario 5 runs no `herdr` for an unresolvable run.
- **Data root.** Decision: `run.DataRoot(deps.Getenv)` made absolute with `filepath.Abs`, passed as `--env LOUPE_HOME=<abs>`. Rationale: `DataRoot` returns `LOUPE_HOME` verbatim, which can be relative, and the split's shell starts elsewhere. Every command already reads a relative root against the process's working directory, so `Abs` names the same root the command just resolved the run in. `LOUPE_HOME` outranks `XDG_DATA_HOME`, so one variable covers both of spec 003's.
- **Command line.** Decision: `<os.Executable()> review <ref.String()> && exit`, each word through the `shellQuote` that `capture` already uses for its cleanup commands: a word of only `[A-Za-z0-9@%+=:,./_-]` stays bare, anything else is wrapped in `'` with `'` written `'\''`. Rationale: the pane runs the same build, which matters for `dist/loupe` and `loupe-demo`; a ref always carries `#`, so it is always quoted; the idiom works in zsh, bash and fish. The resolved ref carries its round, so the pane shows exactly the run the agent filed into.
- **Help text.** Decision: `internal/cli/handoff.go` names Herdr in `handoffHelp` and in the `no-pane-host` message, and the guard allows that one file beside `internal/pane`. Rationale: principle I needs the Herdr requirement in `--help` and in the refusal that names what is missing; moving that text into `pane` would put `cli`'s text in the wrong package. See Complexity Tracking.
- **Human output.** Decision: without `--json`, print `✓ Opened loupe review for <ref> in a new pane <to the right|below>` in the style of `printDone`, without a version. Rationale: the command writes no draft, so there is no version to quote.
- **Codex manifests.** Decision: `plugin/.codex-plugin/plugin.json` with `name`, `version`, `description`, `author`, `repository` and `"skills": "./skills/"`; `.agents/plugins/marketplace.json` with `name: loupe`, `interface.displayName`, and one plugin `{"name": "loupe", "source": {"source": "local", "path": "./plugin"}, "policy": {"installation": "AVAILABLE", "authentication": "ON_INSTALL"}, "category": "Developer Tools"}`. Rationale: the shapes match Codex's curated marketplace cached in `~/.codex/.tmp/plugins`, where plugins declare `"skills": "./skills"` and entries use exactly these keys. Install: `codex plugin marketplace add eriksaulnier/loupe`, then `codex plugin add loupe@loupe` (both from `--help` in Codex 0.154).
- **Pi package.** Decision: root `package.json` `{"name": "loupe", "version", "private": true, "description", "license": "MIT", "repository", "keywords": ["pi-package"], "pi": {"skills": ["./plugin/skills"]}}`. Rationale: Pi 0.84's `docs/packages.md` reads `pi.skills` and finds `SKILL.md` folders recursively; without the manifest it would look for a root `skills/` that does not exist. `private` blocks an accidental `npm publish`. Install: `pi install git:github.com/eriksaulnier/loupe@v<version>`; the tag pins the skills to the binary's release, which answers spec 004's drift edge case.
- **Versions.** Decision: add both new manifests to `release-please-config.json` `extra-files` with `jsonpath: $.version`, and `README.md` as a `generic` extra-file whose Pi install line carries `x-release-please-version`; test that every entry is present and every version equals `.release-please-manifest.json`. Rationale: release-please's `json` updater is what already bumps the Claude Code manifest, and its `generic` updater rewrites only marked lines.
- **Allow rules.** Decision: README names `Bash(loupe handoff:*)` and `prefix_rule(pattern=["loupe", "handoff"], decision="allow")` in `~/.codex/rules/default.rules`, the shape of that file's existing rules. Whether Codex also runs an allowed command outside its sandbox is unverified (FR-025).
- **Retired branches.** `fix/help-pane-rule` (f6ba6a3) supplies the help wording, adapted to name `loupe handoff`; `docs/readme-herdr-allow` (aabd62d) is superseded because the README no longer names any `herdr` rule. Both local branches are deleted after this feature's commits exist.

## Constitution Check

- **I. A tool for agents.** PASS. `loupe handoff` is reachable from `loupe --help` by any agent in any harness. The plugins package instructions only.
- **II. Nothing posts on its own.** PASS. Handoff opens review; the help and the skill keep publish human-only and forbid touching the pane. The reliance on instructions is spec 003's, unchanged.
- **III. Local files, no service.** PASS. `herdr` is a local socket client; no run state is written.
- **IV. Never touch the checkout.** PASS. No git call.
- **V. Machine contract first.** PASS. `--help` without run state, one `--json` result, both refusals name a fix. It takes no structured input, like `wait`.
- **VI. Simplicity.** PASS. No dependency, no interface, no environment filter.
- **VII. Verified means ran.** PASS with the live Herdr, Codex and Pi checks on the Unverified list (FR-025).

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/006-agent-plugins/        spec.md, plan.md, tasks.md, checklists/requirements.md
internal/pane/                  pane.go, pane_test.go
internal/refusal/               two codes and the table test
internal/cli/                   handoff.go, handoff_test.go, root.go (register), help.go (rootRule),
                                review.go (help), help_test.go, review_test.go, golden_test.go, plugin_test.go
testdata/golden/cli/            help.80.txt, help.100.txt, handoff-help.80.txt, handoff-help.100.txt
plugin/skills/human-review/    SKILL.md, the workflow skill; no review method
plugin/commands/                removed
plugin/.codex-plugin/           plugin.json
.agents/plugins/                marketplace.json
package.json                    Pi package
release-please-config.json      two extra-files
scripts/check-tests.sh          Herdr guard narrowed
README.md, AGENTS.md            commands, install, allow rules, layout
specs/001-loupe-v1/             contracts/cli.md, validation.md
specs/002-review-ux/spec.md     UX-013 note
specs/003-herdr-handoff/        spec.md FR-017, FR-018; plan.md socket note
specs/004-agent-hosts/spec.md   FR-005, FR-007, Deferred notes
```

**Structure Decision**: One new package and one new skill directory. No data-model or quickstart: no run layout changes.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| The Herdr guard allows `internal/cli/handoff.go` beside `internal/pane` | Its `--help` and `no-pane-host` message have to name Herdr (principle I) | Moving `cli`'s help text into `pane` splits one command's contract across two packages |
| No environment filter for `herdr`, which the owner's plan named | Nothing in the environment redirects the Herdr client | A filter with an empty list is ceremony (VI) |
