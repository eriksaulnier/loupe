# Implementation Plan: A curl install path for loupe

**Branch**: `047-install-script` | **Date**: 2026-11-02 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/047-install-script/spec.md`

## Summary

`install.sh` at the repository root resolves a release from `checksums.txt`, verifies the archive, stages the binary in the target directory, runs it and moves it into place. `pages.yml` publishes the file. `ci.yml` and `release.yml` run it against real releases, and a Go test runs it against a fake `curl`.

## Technical Context

**Language/Version**: POSIX `sh`. Go 1.26 for the test.

**Primary Dependencies**: None added. The script uses `curl`, `tar`, `awk`, `mktemp` and `sha256sum` or `shasum`.

**Storage**: None. The script writes to a temporary directory and the target directory.

**Testing**: `internal/cli/install_script_test.go` runs the script under `sh` with a fake `curl` and `uname` first on `PATH`, as `install-action-checksums` does in `ci.yml`. `mise run check` adds shellcheck.

**Target Platform**: `linux`/`darwin` × `amd64`/`arm64`.

**Project Type**: Distribution file for a CLI.

**Constraints**: The CLI contract, the published format and the constitution do not change.

**Scale/Scope**: One script, one workflow, one job in each of two workflows, one test file, four documents.

## Constitution Check

- **I**: The script installs the binary and runs nothing else.
- **III**: The script is not loupe. It downloads only from the loupe release on GitHub.
- **VI**: The script has no dependency to justify. The Go test uses the standard library.
- **VII**: Tests use a fake `curl` and stand-in archives, and reach no network.

## Research

- **Latest resolves through `checksums.txt`.** Decision: fetch `releases/latest/download/checksums.txt`, and read the version from the archive name in the matching line. Rationale: FR-005. The redirect has no rate limit, and one file gives both the version and the digest. Alternative rejected: the releases API, which limits anonymous callers.
- **The archive comes from the tag, not from `latest`.** Decision: after the line names a version, every later download uses `releases/download/v<version>`. Rationale: a release published between the two downloads would otherwise give a 404, because `latest` no longer holds the older archive name.
- **One match rule for both modes.** Decision: an `awk` pattern on the second field, with the version as `[^_]+` when unpinned, and one result line only when exactly one line matched. Rationale: FR-005. A field comparison cannot match a longer name such as a signature file. The version in the pattern comes from the same tag as the file, so a dot that matches any character cannot select another release.
- **Digest over stdin.** Decision: pipe the archive into `sha256sum` or `shasum -a 256`. Rationale: both escape a path that holds a backslash, which would change the first field. A temporary path could hold one, since `TMPDIR` sets it.
- **The staged binary runs before the move.** Decision: copy to a hidden name in the target directory, run `--version` on that copy, then `mv`. Rationale: FR-008 and FR-009 together. A run after the move would replace a working binary with one that fails. A run in the target directory also catches a `noexec` mount.
- **Signals.** Decision: an `EXIT` trap removes the temporary directory and the staged file, and a `HUP`, `INT` and `TERM` trap exits through it. Rationale: `dash` does not run an `EXIT` trap on death by signal.
- **A directory named `loupe`.** Decision: refuse. Rationale: `mv` would move the binary into the directory and report success.
- **Pages content.** Decision: copy `install.sh` into an otherwise empty `_site/`. Rationale: FR-013. Nothing else in the repository becomes public at that host.
- **Action pins.** Decision: `pages.yml` pins `checkout`, `configure-pages`, `upload-pages-artifact` and `deploy-pages` by commit sha, at the highest major tags on 2026-11-02, with the job guarded to `refs/heads/main`. Rationale: this job publishes what people pipe into `sh`, so neither a moved tag nor a dispatch on another ref may reach it. Dependabot keeps the pins current.
- **The URL pin carries no scheme.** Decision: the test searches for `eriksaulnier.github.io/loupe/install.sh`. Rationale: `scripts/check-tests.sh` refuses a URL literal that names another host, and the test fetches nothing.

## Verification

- `mise run check` runs the new test and shellcheck.
- `sh install.sh` with `LOUPE_INSTALL_DIR` set to a scratch directory installs a real release on a Mac. `xattr` shows no `com.apple.quarantine`, and the binary starts.
- A tampered `checksums.txt` through the fake `curl` refuses, and nothing lands in the target.
- `actionlint` passes on the new workflow.

Not verifiable before merge: the `pages` deploy, the live URL and the `release.yml` job. After merge, run `curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | cmp - install.sh`.

## Complexity Tracking

None.
