# Implementation Plan: Install and update the binary and the agent plugins together

**Branch**: `update-flow` | **Date**: 2026-11-06 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/055-update-flow/spec.md`

## Summary

`install.sh` gains `sync_plugins`, which runs after the binary installs. For each of `claude`, `codex` and `pi` on `PATH`, it installs the loupe plugin at `v$version`, the tag of the binary it just installed. It replaces a copy from another remote source and leaves a local-directory marketplace alone. goreleaser attaches `install.sh` to each release and lists it in `checksums.txt`. A new `loupe update` command reads the latest release's `checksums.txt`, downloads and verifies that release's `install.sh`, and runs it for the directory that holds the running binary. The update warning and the review notice name `loupe update`.

## Technical Context

**Language/Version**: Go 1.26, and POSIX `sh` for `install.sh`.

**Primary Dependencies**: None added. `net/http`, `crypto/sha256` and `os/exec` cover `loupe update`. The script needs no `jq`. It flattens each agent's JSON with `tr` and matches it with `case`.

**Storage**: None. The update cache from spec 048 is unchanged.

**Testing**: `install_script_test.go` gains stand-in `claude`, `codex` and `pi` commands that log their arguments and read their answers from files. `update_test.go` serves a release from an `httptest` server and runs a stand-in script through the real `sh`.

**Target Platform**: Linux and macOS, as today.

**Project Type**: CLI.

**Constraints**:

- Stdout stays machine output, so the script's output goes to stderr under `loupe update`.
- The script's output before the plugin step MUST NOT change. Existing goldens change only for the new command and its help.

**Scale/Scope**: About 120 lines of shell, one command of about 150 lines, one goreleaser stanza, a constitution amendment, a contract amendment and the docs.

## Constitution Check

The check is against constitution 4.0.0, after the amendment that is the first commit of this feature.

- **I**: `loupe update` and the script manage loupe's own plugin through each host's plugin command. Under the 4.0.0 wording, that is not invoking a reviewer. No agent session starts, and nothing prompts a model. Each host gets the same treatment, so no code path serves one host only.
- **III**: The only network access that `loupe update` adds is to loupe's own release downloads, which 4.0.0 allows. There is no daemon and no state beyond the binary and the plugins.
- **V**: `update` answers `--help` without run state. With `--json` it prints one result object. Each refusal names its reason and fix. The script's output goes to stderr.
- **VI**: No dependency is added. The script remains the one implementation of an install, and `loupe update` adds no second one.
- **VII**: Tests use an `httptest` release server, stand-in agent commands and a stand-in script. A real release and real agent CLIs are checked by eye and listed as unverified.

## Research

### Agent plugin commands (observed 2026-11-06 in a scratch `HOME`)

| Behavior | Claude Code | Codex | Pi |
| :--- | :--- | :--- | :--- |
| List the source | `claude plugin marketplace list --json` returns `[{name, source: "github"\|"directory", repo, ref, path}]` | `codex plugin marketplace list --json` returns `{marketplaces: [{name, marketplaceSource: {sourceType: "git"\|"local", source}}]}`, with no ref | Not needed |
| Add at the same ref while present | exit 0, "already on disk" | exit 0, "already added from …#v0.21.0" | `pi install` at any tag exits 0 |
| Add at another ref while present | exit 1, "source doesn't match its extraKnownMarketplaces entry" | exit 1, "already added from a different source; remove it before adding" | Replaces the earlier entry, whatever its URL form |
| Remove the marketplace | Also uninstalls `loupe@loupe`. Exit 1 when absent | Also uninstalls `loupe@loupe`. Exit 1 when absent | `pi remove` with any form of the repository, tagged or not, removes the package. Exit 1 when absent |
| Install when already installed | `claude plugin install loupe@loupe` exits 0 | `codex plugin add loupe@loupe` exits 0 | Not needed |
| Pinned form | `claude plugin marketplace add eriksaulnier/loupe@v0.21.0` | `codex plugin marketplace add eriksaulnier/loupe@v0.21.0` | `pi install git:github.com/eriksaulnier/loupe@v0.21.0` |

### Decisions

- **Claude Code state.** Decision: read the list and flatten it with `tr -d ' \t\r\n' | tr '{' '\n'`. Pick the line that holds `"name":"loupe"`, then branch:
  - `"source":"directory"`: skip the agent.
  - No line: add the marketplace.
  - Both `"repo":"eriksaulnier/loupe"` and `"ref":"<tag>"`: keep the marketplace.
  - Anything else: remove it, then add it.

  In every case except a skip, the script then runs `install`. Rationale: the list carries the ref, so the script removes only when it must. A failed add after a remove therefore happens only where the source was going to change anyway.
- **Codex state.** Decision: read the list the same way. A `"sourceType":"local"` on the entry's next line skips the agent. Otherwise run `add` at the tag:
  - exit 0: the marketplace is current or new.
  - Exit 1 with "different source": remove it, then add it.
  - Any other failure: the agent fails, and nothing is removed.

  Then run `codex plugin add loupe@loupe`. Rationale: the Codex list carries no ref. Matching the one message where removal is safe fails closed. If the message changes, the result is a loud error, not a lost plugin.
- **Pi.** Decision: run `pi install git:github.com/eriksaulnier/loupe@<tag>`. Rationale: Pi replaces any earlier loupe entry and is idempotent.
- **Output.** Decision: each agent command runs with its output captured. On success the script prints one line, such as `Claude Code: loupe plugin v0.22.0. Restart running sessions to load it.`. On failure it prints the captured output, then `error: could not set up the loupe plugin for Claude Code` and `fix: <that agent's manual commands>`. A skip prints why. When the script finds no agent, it prints `No Claude Code, Codex or Pi found. To add the plugin later, see <docs>`.
- **Errexit.** Decision: each agent function checks every command explicitly. `main` calls `sync_plugins` last, and `sync_plugins` exits 1 when any agent failed. Rationale: a function called in an `||` list runs without `set -e`, so the checks MUST be explicit.
- **Skips.** Decision: a non-empty `CI` or `LOUPE_NO_PLUGINS` skips every agent with one line. Rationale: `action.yml` and the CI jobs run the script, and spec 048 already treats `CI` this way.
- **`mise run link` and `unlink`.** `link` points the Claude Code marketplace at the worktree, so the script skips it (FR-004). Decision: `unlink` removes that marketplace first, then runs the script, which adds it back at the release tag. Rationale: `unlink` keeps one implementation, and the script never removes a marketplace that a developer pointed at a worktree.
- **The release asset.** Decision: list `install.sh` in both `release.extra_files` and `checksum.extra_files` in `.goreleaser.yaml`, and confirm it in a snapshot's `dist/checksums.txt`. Rationale: `loupe update` refuses a script that the checksums do not cover.
- **Finding the latest version.** Decision: `loupe update` fetches `<releases>/latest/download/checksums.txt` and takes the version from the line `loupe_<v>_<GOOS>_<GOARCH>.tar.gz`. Rationale: this is what the script does. It needs no token and no API, and the release it names is the one the script then installs.
- **The download base.** Decision: `Deps.Releases`, which defaults to `https://github.com/eriksaulnier/loupe/releases`. Tests point it at an `httptest` server. Rationale: `check-tests.sh` allows only local URLs in tests.
  - Alternative considered: a fake `curl` for `update_test.go`. Rejected, because `loupe update` downloads in Go, not with `curl`.
- **Running the script.** Decision: `sh <tmp>/install.sh`, with this environment:
  - `LOUPE_VERSION=<v>`
  - `LOUPE_INSTALL_DIR=<dir of filepath.EvalSymlinks(os.Executable())>`

  `Stdout` and `Stderr` are both `deps.Stderr`, and `Stdin` is empty. Tests run the real `sh` against a stand-in script, so the process needs no seam. Rationale: the release script sets `LOUPE_VERSION`, so the script and the archive come from the same release. Resolving the symlink replaces the file that actually runs.
- **The executable.** Decision: `Deps.Executable` defaults to `os.Executable` and is resolved through `filepath.EvalSymlinks`. A path that contains `/mise/installs/` refuses. Rationale: tests need a chosen path, and mise's layout is `~/.local/share/mise/installs/<tool>/<version>/`.
- **Refusals.** Decision: three new codes.
  - `update-unsupported` with `details.reason` set to `not-release` or `mise`. The fix is the curl command, or `mise use -g github:eriksaulnier/loupe@latest`.
  - `download` for a failed fetch, a missing asset or a checksum mismatch. The fix is the network and the release URL.
  - `install-failed` with `details.exitStatus`. The fix is to read the script's output above, or to rerun the curl command.

  Rationale: each code needs a different fix.
- **Version check.** Decision: export `updatecheck.IsRelease(version string) bool`, a thin wrapper over `parse`. `Enabled` keeps its own environment rules. Rationale: `update` needs the version rule without the `CI` rule, because a person may run `loupe update` in a CI shell on purpose.
- **Warning text.** Decision: the text becomes `loupe 0.23.0 is available, you have 0.22.0. Run loupe update`, in capture's `warning:` line and in the review notice. `InstallDocs` gives way to `Command`, since the refusal fixes name the curl command. This amends spec 048 FR-006. Rationale: the warning should name a command the person can run, and `update` itself refuses with the right fix for a mise install.
- **Help group.** Decision: `human`, beside `review` and `publish`. It does not require a terminal. Rationale: updating is a person's choice (spec assumption). A refusal without a terminal would break a script that a person writes on purpose.
- **Uninstall.** Decision: `install.sh` gains `LOUPE_UNINSTALL=1`, an internal mode that `loupe uninstall` sets.
  - It removes the Claude Code and Codex marketplace named `loupe` unless its source is a local directory. Removing the marketplace also uninstalls the plugin.
  - For Pi, when `pi list` shows `github.com/eriksaulnier/loupe`, it runs `pi remove git:github.com/eriksaulnier/loupe`, with no tag.
  - Then it removes `$LOUPE_INSTALL_DIR/loupe` and prints the data root. It downloads nothing.
  - `loupe uninstall` fetches the verified script of its own version, `download/v<current>/install.sh`, rather than the latest one. It shares `verifiedScript`, `installDir` and `runInstallScript` with `update`.

  Rationale: the script stays the one place that knows what an install adds. The cost is that an uninstall needs the network.
- **Identifier names.** Decision: `newSelfUpdateCmd`, `selfupdate.go`. Rationale: `internal/cli` already declares `update` as the golden flag.

## Files

| File | Change |
| :--- | :--- |
| `.specify/memory/constitution.md` | 4.0.0: Principles I and III, and a history entry |
| `install.sh` | Add `sync_plugins`, `sync_claude`, `sync_codex`, `sync_pi` and `agent_failed`. Document `LOUPE_NO_PLUGINS` in the header |
| `mise.toml` | `unlink` removes the marketplace, then runs the script |
| `.goreleaser.yaml` | `install.sh` in `release.extra_files` and `checksum.extra_files` |
| `internal/updatecheck/update.go` | `IsRelease`, and the warning text |
| `internal/cli/selfupdate.go` | The `update` command |
| `internal/cli/root.go` | `Deps.Releases`, `Deps.Executable`, and the command in the `human` group |
| `internal/refusal/refusal.go` | `update-unsupported`, `download`, `install-failed` |
| `internal/cli/capture.go`, `capture_local.go`, `review.go` | The new warning text |
| `specs/001-loupe-v1/contracts/cli.md` | The `update` command, its three codes, `LOUPE_NO_PLUGINS`, and the amended `update-available` text |
| `README.md`, `docs/install.md` | As in the approved plan, section 5 |
| `plugin/skills/*` | No change. The skills MUST NOT name `loupe update` |

## Verification

- `mise run check`.
- Install script tests for each acceptance scenario of stories 1, 2 and 4, including the call sequence for each agent and the state that the stand-ins hold before and after.
- `loupe update` tests for these cases:
  - the happy path, checking the environment the stand-in script received
  - already current
  - a checksum mismatch, a missing `install.sh` and a missing archive line
  - a dev build, a pre-release and a mise path
  - a script that exits non-zero
  - the `--json` envelope
- `goreleaser release --snapshot --clean`: `dist/checksums.txt` lists `install.sh`.
- By eye: run the real script in a scratch `HOME`, with real `claude`, `codex` and `pi`, at `v0.21.0`. Then run it at `v0.20.0` and check that each plugin moves and leaves no second copy. Then run it with a local-directory Claude marketplace and check the skip.
- Unverified: `loupe update` against a real release. The first release that carries `install.sh` as an asset is the first one that can be updated *from*. Until a release after it exists, `loupe update` reinstalls the current release.

## Complexity Tracking

None. The constitution amendment is recorded in the constitution itself.
