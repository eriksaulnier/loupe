# Withdrawal reasons

An agent that withdraws a finding with `loupe edit <id> --exclude` SHOULD say why. The human reads the reason beside the withdrawn finding in `loupe review`, in its row and at the top of its detail, and decides from it whether to reinstate the finding with `u`. A withdrawn finding is never published, so the reason never reaches GitHub (`specs/041-withdraw-reason`).

```sh
loupe edit f-004 --exclude --reason "Test name only, assertion unchanged." --reason-code no-runtime-effect --json
```

`--from` input carries the same pair as `"withdrawReason": {"text": "...", "code": "..."}`, together with `--exclude`.

- **Text** is what the human reads: one line of Markdown, at most 200 characters, that passes the allowlist a reply passes. It is required whenever a reason is given.
- **Code** is optional. It is one word that a later view MAY group withdrawals on, across runs and across skills. It follows the `--label` rule: it starts with a letter or digit, holds only letters, digits, `_`, `.` and `-`, and is at most 40 characters.

A withdrawal without a reason still succeeds in this release. `loupe edit` then prints a warning on stderr, and the `--json` result carries `"warnings": ["withdraw-reason-missing"]`. A later release MAY refuse it.

To add or correct the reason of a finding already withdrawn, run the same call with the new reason. It keeps the finding's `rev` and the human's decision, and only the newest reason shows.

## Recommended codes

loupe recommends these codes and does not enforce them. What a skill withdraws for is that skill's policy, so loupe MUST NOT own the list. A skill SHOULD use one of these words where it fits, so its withdrawals group with other skills' withdrawals, and MAY use its own word where none fits.

| Code | When it applies |
| :--- | :--- |
| `no-runtime-effect` | The finding claims nothing changes at run time, such as naming, wording or style |
| `conceded` | The agent agreed with a send-back note and withdrew the finding instead of revising it |
| `duplicate` | Another finding in the review says the same thing |
| `pre-existing` | The problem predates the pull request |

`loupe edit --help` carries the same list, so an agent finds it without this page.
