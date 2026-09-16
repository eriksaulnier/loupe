# loupe Constitution

loupe gives any shell-capable agent a standard way to file pull-request review findings into a local draft, gives the human a terminal interface to decide each finding, and posts exactly one GitHub review per publication: confirmed by the human, or posted by a GitHub App and marked unattended. These principles govern every specification, plan and task in this repository.

## Core Principles

### I. A tool for agents, not a tool that uses agents

loupe MUST NOT invoke, prompt, supervise, sandbox or authenticate a reviewer. Any agent with a shell is a first-class user, and every workflow MUST be completable from `loupe --help` alone. Host integrations (the plugin for Claude Code, Codex and Pi) MAY package instructions and commands, but MUST NOT add code paths that only one host can reach.

### II. Nothing posts unread under a human's name

Every finding that reaches GitHub under a human's identity MUST have been individually accepted by that human, and the review as a whole MUST have been confirmed by the human in one interactive command that sends exactly one GitHub request. That command MUST refuse without an interactive terminal. An unattended publication MAY skip the terminal and the per-finding decisions only when it authenticates with a GitHub App installation token. It MUST post as that App, MUST send a COMMENT review, and MUST mark the review as unattended, so a review no one read never approves or blocks a merge and never reads as a person's. Agents working in a human's session are forbidden from publishing by contract. Per-finding sign-off is the product, not a safety rail: designs that let a human approve a batch without seeing each item are out of scope.

### III. Local files, no service

Run state is plain files on disk, one directory per run. There MUST be no daemon, no database and no network access beyond Git and the GitHub API. loupe MUST NOT delete a run directory or publication evidence automatically.

### IV. Never touch the user's checkout

Capture MAY write Git objects and loupe-owned private refs into the user's clone. It MUST NOT change working files, the index, the current branch or any other ref, and it MUST NOT create a checkout of its own.

### V. Machine contract first

Every agent-facing command MUST answer `--help` without run state, MUST accept structured input from a file or stdin, and with `--json` MUST emit exactly one versioned result object on stdout. Every refusal MUST name its reason and the corrective command. Stdout is machine output; diagnostics go to stderr.

### VI. Simplicity over ceremony

Prefer a version number to a hash, a refusal to a recovery path, and the standard library to a dependency. Every runtime dependency MUST carry a one-line reason in the plan. Comments explain why, never what. New abstractions MUST be justified by a second concrete use, not a possible one.

### VII. Verified means ran

A completion claim MUST rest on a check that ran after the last edit, with its output shown. Tests MUST use a fake GitHub, local Git repositories and injected terminal input. Tests MUST NOT use a real pseudo-terminal, a real reviewer or real publication.

## Boundaries

- The browser feedback surface, MCP server mode and a `gh` extension alias are deferred, not rejected. Designs MUST NOT preclude them, and MUST NOT build toward them speculatively.
- The published comment format in `docs/comment-format.md` is a contract shared with humans reading GitHub. Changes to it require a spec amendment.
- Observed GitHub behavior recorded in `docs/github-facts.md` MUST be treated as evidence, not as a guarantee about future behavior.

## Development Workflow

- Each task follows failing test → implementation → passing check. The repository check is `mise run check`, which runs `go vet ./...`, `golangci-lint run`, `actionlint`, `scripts/check-tests.sh` and `go test ./...`; every one of them MUST pass before a commit.
- Commits MUST use Conventional Commits: `type(scope): subject`, imperative lowercase subject of at most 72 characters, no trailing period, blank line before a body. One logical change per commit.
- No push, release, GitHub review or PR MAY be created without an explicit request from the user.
- Live runs against GitHub MUST target a pull request the user names, on a repository the user controls.
- Documentation uses American spelling, RFC 2119 keywords for normative statements, one line per paragraph, and a final newline.

## Governance

This constitution supersedes every other practice in the repository. An amendment MUST state what changed and why, bump the version below (MAJOR for a removed or redefined principle, MINOR for a new principle or section, PATCH for wording), and update the specification and plan when a principle they rely on changes. Plans MUST include a Constitution Check and justify each violation in Complexity Tracking.

**Version**: 2.0.1 | **Ratified**: 2026-09-13 | **Last Amended**: 2026-09-16
