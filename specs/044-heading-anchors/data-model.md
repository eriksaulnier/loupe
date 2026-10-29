# Data model: Anchor a finding to a Markdown heading

## `markdown.Heading`

| Field | JSON | Meaning |
| :--- | :--- | :--- |
| `Text` | `heading` | The heading's text after FR-007 normalization |
| `Level` | `level` | 1 to 6, the length of the opening `#` run |
| `Line` | `line` | 1-based line in the file at the head |

The same shape is an outline entry and an entry of a refusal's `details.headings`.

## `outline.json` (new run file, `schema` 1)

| Field | Type | Meaning |
| :--- | :--- | :--- |
| `schema` | int | `run.OutlineSchema`, 1 |
| `files` | array | One entry per changed Markdown file at the head, in diff order |
| `files.path` | string | The file's path at the head |
| `files.headings` | array of `markdown.Heading` | Every top-level ATX heading, in file order; empty when the file has none |

- Written once by `capture`, never changed. Strict, like every run file (`docs/versioning.md`).
- Missing: a run captured before this feature. `files` empty: a pull request that changes no Markdown file.
- In memory it is `diff.Outline`, a `map[string][]markdown.Heading`. A nil map means the file is missing.

## `draft.FindingLocation` (draft `schema` 3)

| Field | JSON | Meaning |
| :--- | :--- | :--- |
| `Path` | `path` | As `draft.Location` |
| `Side` | `side` | Always `RIGHT` when `Heading` is set |
| `Line` | `line` | The resolved range's last line |
| `StartLine` | `startLine,omitempty` | The resolved range's first line, or 0 for a single line |
| `Heading` | `heading,omitempty` | The heading's text from the outline, as written; empty for a line form |

- `Published()` returns `draft.Location{Path, Side, Line, StartLine}`. Every reader outside the draft and the review interface uses that (FR-016).
- Set by `add` and `edit`. Any edit that sets `location` replaces the whole value, so a line form drops the heading. `null` and `--clear-location` remove it with the location (FR-015).
- Draft schema 2 files read into it unchanged, with an empty `Heading`.

## `draft.LocationInput` (input only)

| Field | JSON | Rule |
| :--- | :--- | :--- |
| `Path` | `path` | Required in both forms |
| `Side` | `side` | Line form: `RIGHT` or `LEFT`. Heading form: empty or `RIGHT`, else `input` |
| `Line` | `line` | Line form: required. Heading form: MUST be 0, else `input` |
| `StartLine` | `startLine,omitempty` | Heading form: MUST be 0, else `input` |
| `Heading` | `heading,omitempty` | A pointer, so an empty string refuses with `input` |

## State

A heading anchor has no state of its own. `add` or `edit` resolves it once, against `pr.diff` and `outline.json`, which never change within a run. After that the finding holds an ordinary range plus the heading's text. A later edit to any other field validates the stored range as a line form and does not resolve the heading again.
