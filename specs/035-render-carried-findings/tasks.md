# Tasks: Render carried findings

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task is test first: the failing test, then the change, then `mise run check`.

- [x] T001 `render`: open assessments count in the chips row and the anchor; `🟢 no findings` only with nothing open.
- [x] T002 `render`: `### Still open from earlier rounds`, ordered and escaped, with its location link.
- [x] T003 `render`: `loupe-meta` `carried=` and `carriedblocking=`.
- [x] T004 `render`: a sticky round carrying a finding collapses with matching pills and keeps its section.
- [x] T005 `docs/comment-format.md`.
