# Reference frames

These frames translate the chosen round-five titled-pane design into the complete view set. Each text file has exactly its stated number of rows. Trailing spaces are omitted. Dark frames have an adjacent `.ansi` file with the same stem that preserves colors and padding. `NO_COLOR` frames contain no escapes. The fixture has seven findings, including one withdrawn, so the review header reads `3/6 decided` while row position reads `1 of 7`. Publish fixtures settle the three pending findings first and read `6/6 decided`.

The local throwaway program generated these frames. It is not product code or an implementation of the plan. The [layout contract](../contracts/layout.md) defines dynamic behavior, muted syntax roles, width tiers, glyph fallbacks, and clipping. The reference frames do not prove runtime behavior.

The list preserves current keys, despite decision shortcuts in the source mock. The note frame shows an existing note/reply thread. The send-back frame shows its composer. The Message frame shows focused human input. Confirmation after blur, payload view, sending, errors, empty rounds, and overflow need the implementation tests in [tasks.md](../tasks.md). The too-small frames are presentation studies of a 40x10 refusal placed on the requested reference canvases. A real 100x30 or 80x24 terminal MUST NOT activate that refusal.

## Frame matrix

| View | 100x30 dark | 80x24 dark | 100x30 NO_COLOR | 80x24 NO_COLOR |
| :--- | :--- | :--- | :--- | :--- |
| list | [text](list-100x30-dark.txt) · [ANSI](list-100x30-dark.ansi) | [text](list-80x24-dark.txt) · [ANSI](list-80x24-dark.ansi) | [text](list-100x30-nocolor.txt) | [text](list-80x24-nocolor.txt) |
| detail | [text](detail-100x30-dark.txt) · [ANSI](detail-100x30-dark.ansi) | [text](detail-80x24-dark.txt) · [ANSI](detail-80x24-dark.ansi) | [text](detail-100x30-nocolor.txt) | [text](detail-80x24-nocolor.txt) |
| note | [text](note-100x30-dark.txt) · [ANSI](note-100x30-dark.ansi) | [text](note-80x24-dark.txt) · [ANSI](note-80x24-dark.ansi) | [text](note-100x30-nocolor.txt) | [text](note-80x24-nocolor.txt) |
| send-back | [text](send-back-100x30-dark.txt) · [ANSI](send-back-100x30-dark.ansi) | [text](send-back-80x24-dark.txt) · [ANSI](send-back-80x24-dark.ansi) | [text](send-back-100x30-nocolor.txt) | [text](send-back-80x24-nocolor.txt) |
| action | [text](action-100x30-dark.txt) · [ANSI](action-100x30-dark.ansi) | [text](action-80x24-dark.txt) · [ANSI](action-80x24-dark.ansi) | [text](action-100x30-nocolor.txt) | [text](action-80x24-nocolor.txt) |
| inline | [text](inline-100x30-dark.txt) · [ANSI](inline-100x30-dark.ansi) | [text](inline-80x24-dark.txt) · [ANSI](inline-80x24-dark.ansi) | [text](inline-100x30-nocolor.txt) | [text](inline-80x24-nocolor.txt) |
| message | [text](message-100x30-dark.txt) · [ANSI](message-100x30-dark.ansi) | [text](message-80x24-dark.txt) · [ANSI](message-80x24-dark.ansi) | [text](message-100x30-nocolor.txt) | [text](message-80x24-nocolor.txt) |
| published | [text](published-100x30-dark.txt) · [ANSI](published-100x30-dark.ansi) | [text](published-80x24-dark.txt) · [ANSI](published-80x24-dark.ansi) | [text](published-100x30-nocolor.txt) | [text](published-80x24-nocolor.txt) |
| help | [text](help-100x30-dark.txt) · [ANSI](help-100x30-dark.ansi) | [text](help-80x24-dark.txt) · [ANSI](help-80x24-dark.ansi) | [text](help-100x30-nocolor.txt) | [text](help-80x24-nocolor.txt) |
| small | [text](small-100x30-dark.txt) · [ANSI](small-100x30-dark.ansi) | [text](small-80x24-dark.txt) · [ANSI](small-80x24-dark.ansi) | [text](small-100x30-nocolor.txt) | [text](small-80x24-nocolor.txt) |
| edit | [text](edit-100x30-dark.txt) · [ANSI](edit-100x30-dark.ansi) | [text](edit-80x24-dark.txt) · [ANSI](edit-80x24-dark.ansi) | [text](edit-100x30-nocolor.txt) | [text](edit-80x24-nocolor.txt) |
| file | [text](file-100x30-dark.txt) · [ANSI](file-100x30-dark.ansi) | [text](file-80x24-dark.txt) · [ANSI](file-80x24-dark.ansi) | [text](file-100x30-nocolor.txt) | [text](file-80x24-nocolor.txt) |
| toast | [text](toast-100x30-dark.txt) · [ANSI](toast-100x30-dark.ansi) | [text](toast-80x24-dark.txt) · [ANSI](toast-80x24-dark.ansi) | [text](toast-100x30-nocolor.txt) | [text](toast-80x24-nocolor.txt) |

## Reading the frames

The ANSI files MUST be viewed only from these trusted fixtures. For example:

```sh
cat specs/038-review-visual-design/frames/detail-100x30-dark.ansi
```

The dark palette and muted syntax replace the overly colorful code in the mock. Light mode uses the exact tokens in the layout contract. Light and 16-color output require implementation validation, although this frame set covers only the requested dark and monochrome modes.
