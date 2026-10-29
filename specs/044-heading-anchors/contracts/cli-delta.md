# Contract delta: `specs/001-loupe-v1/contracts/cli.md`

This file lists the edits the implementation makes to the CLI contract (FR-020). It is not a second contract. Once the edits land, `contracts/cli.md` is the only source.

## Error codes table

The `location` row becomes:

| `location` | A path or line not in the stored diff, a range that spans hunks, or a heading that is missing, repeated, holds no change or holds changes in more than one hunk | nearest valid lines, or the headings and ranges to use |

## `loupe capture`

Add, after the sentence on `comments`: capture also stores a heading outline of each `.md`, `.markdown` and `.mdx` file the diff changes, read at the captured head, for heading anchors (spec 044). It writes the outline on every capture, with no files in it when the diff changes no Markdown file. A failure to read a changed Markdown file at the head refuses the capture as any other git failure does. The result payload does not change.

## `loupe add`

Add after the paragraph that starts "Exactly one of `location`":

`location` MAY instead be a heading anchor, `{"path": "specs/007-x/plan.md", "heading": "Rollout"}`, on a `.md`, `.markdown` or `.mdx` path in any letter case (spec 044). `heading` is the heading's text as the file writes it, without its `#` marks; a match is exact and case-sensitive after leading and trailing whitespace is trimmed and each inner run of whitespace is read as one space. Only an ATX heading at the top level of the file counts, not one in a fence, an indented block, front matter, a block quote or a list. The section runs from the heading to the line before the next heading of the same or a higher level. `add` resolves the anchor against the stored diff and the outline capture stored, never the clone, to the new-side lines of the one hunk that changes the section, and stores that range with `side` `RIGHT` and the heading's text. A heading anchor that carries `line` or `startLine`, a `side` other than `RIGHT`, or an empty `heading` refuses with `input`. It refuses with `location` when the path is not Markdown, the file is deleted or binary, the run was captured before spec 044, no heading matches (the fix lists the headings whose sections hold a change, and `details.headings` lists them as `{heading, level, line}`), the section holds no change (the same fix and details), the heading matches more than one heading (the fix gives the line ranges to use instead, and `details.matches` lists `{level, line, ranges: [{startLine, line}]}`), or more than one hunk changes the section (the fix lists one range per hunk to file as line locations, and `details.ranges` lists them as `{startLine, line}`). The flag equivalent is `--heading <text>` with `--path`.

## `loupe edit`

Add: `location` MAY be a heading anchor, resolved as for `add`. Any edit that sets `location` replaces the stored heading, and a line form removes it; `null` and `--clear-location` remove it with the location. `--heading` is a location flag, so it replaces the whole location as the other location flags do.

## `loupe show`

Add: a finding stored from a heading anchor carries `location.heading`. Every other output, the publication and the receipt carry the range alone.
