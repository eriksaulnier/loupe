# The review window's design language

This is the contract for how the review program draws a finding and its header. It governs `internal/style` and `internal/tui`. It does not govern the published review, which [comment-format.md](comment-format.md) owns. Where this document and a spec disagree, the spec wins and the conflict is a bug here. The reference frames are in `specs/052-finding-design-language/frames/`. They show synthetic content, and dynamic text MUST follow these rules rather than copy the fixture values.

It supersedes `specs/038-review-visual-design/contracts/layout.md` for the finding document and the header. That file's row budget and width tiers for the list and the publish screen still hold.

## Sections

A section is a bold heading in its tone, over a `┃` bar in the same tone. The bar runs down every line of the section's body, blank lines included, so a section reads as one block. A section MUST be drawn by `style.Section`. A screen MUST NOT style a heading or a bar inline.

Every block in a finding below the title and meta is a section, except the location pane. The body is the section `Why`. A blank row separates two sections.

Without color, the heading is plain text and the bar stays, so the structure survives `NO_COLOR`.

## Tone says whose words they are

| Tone | Whose words | Sections |
| :--- | :--- | :--- |
| Lavender (accent) | The finding's own case | `Why`, `Impact`, `References` |
| Green (good) | The agent's proposed change | `Suggested fix` on a PR run, `<applier> will` on a local run |
| Amber (note) | The conversation between the human and the agent | Note threads and replies, and `A question for <applier>` on a local finding with no proposal |
| Muted (overlay) | Withdrawn content | `Withdrawn` and every section of a withdrawn finding |

- A tone MUST NOT be chosen for emphasis or importance. It names the speaker.
- `Suggested fix` and `<applier> will` MUST be drawn by one renderer. They are the same kind of content in two modes.
- A closed note recedes to muted, as it does today. An open note is amber.
- A section heading MUST NOT carry a leading glyph. The heading's words and tone carry its meaning.

## Boxes

A box is for code and for anything the human is editing. That is the location pane, the send-back and edit composers, the local start screen and the publish confirmation. Prose that the human reads but does not edit MUST NOT be boxed. It is a section.

A pane title MUST be drawn by a `style` helper. A screen MUST NOT style a pane title inline.

## Order

A finding's document has one order in both modes:

1. Title and meta.
2. `Withdrawn`, when the agent withdrew the finding.
3. The notice that the agent changed text the human wrote, when it did. It is one line in the note color with the note glyph, not a section, because it reports an event rather than holding anyone's words.
4. The location pane.
5. `Why`.
6. The proposed change: `Suggested fix`, `<applier> will`, or `A question for <applier>`. A suggested fix that sits beside a proposal or a question follows it as its own `Suggested fix` section.
7. `Impact`.
8. `References`.
9. Notes.

- A missing section collapses. The rest keep their order.
- Importance MUST NOT move a section. Severity, the blocking mark and the list order already carry importance.
- `Withdrawn` is a state, not content. It sits above the evidence so the human judges the withdrawal before reinstating the finding. A finding that shows `Withdrawn` is muted throughout, including after the human excludes it.

## The location pane

The location pane has a maximum height. A hunk taller than that scrolls inside the pane, so the proposed change stays on screen under it. The pane MUST show which part of the hunk is visible, and the finding's own lines MUST be in view when the finding opens.

## The header

The header is one row, then one blank row, then the body.

| Run | Anatomy |
| :--- | :--- |
| Pull request | `badge  owner/repo#N  rN  title  progress  N/M decided` |
| Local | `badge  [local · returns to X]  <repo> @ <branch> · <capture>  title  progress  N/M decided` |

- A pull request's reference is bold and underlined, and links to the pull request.
- A local run MUST NOT show its reference (`owner/repo:local#N`) in the header. Nothing in it is a link. `loupe list` and `loupe show` still name the run by its reference.
- `<repo>` is the repository name from the run's prefix, or the base name of its root when there is no prefix.
- `<capture>` summarizes what `loupe capture` took: `N files` for `--files`, `changes since <base>` for `--changed --base <base>`, `uncommitted changes` for `--changed` alone, and `no files` for `--no-files`.

The local header has three forms. It MUST draw the richest form that fits with room for up to 16 cells of the title:

| Form | Badge | Place | Progress bar |
| :--- | :--- | :--- | :--- |
| Full | `local · returns to X` | `<repo> @ <branch> · <capture>` | Shown |
| Medium | `local · X` | `<repo> @ <branch>` | Shown |
| Short | `local` | `<branch>`, or `<repo>` on a detached head | Dropped |

In the frames, 124 columns draws the full form, 100 the medium form and 80 the short form. The title then takes what is left and truncates. The bold `N/M decided` count MUST NOT give way.
