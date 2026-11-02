# Security policy

## Supported versions

Only the latest release receives security fixes.

## Report a vulnerability

Report a vulnerability privately through [GitHub's private vulnerability reporting](https://github.com/eriksaulnier/loupe/security/advisories/new). A vulnerability MUST NOT be reported in a public issue.

Include the version from `loupe --version`, the steps that reproduce the problem and what an attacker gains.

loupe has one maintainer, so there is no promised response time.

## Scope

These are in scope:

- A way to post a review under a person's name that they did not accept and confirm. The [constitution](.specify/memory/constitution.md) makes this loupe's central guarantee.
- A way to expose the GitHub token that loupe reads, or to write outside its run directory and its private Git refs.
- A flaw in an install path or a release workflow that could change what people install.

A compromised review agent is out of scope when it stays inside what the CLI allows an agent to do. Findings that an agent files are untrusted input, and the human decides each one.
