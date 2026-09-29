# Tasks: Render carried findings

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task is test first: the failing test, then the change, then `mise run check`.

- [ ] T001 `render`: open assessments count in the chips row and the anchor; `🟢 no findings` only with nothing open.
- [ ] T002 `render`: `### Still open from earlier rounds`, ordered and escaped, with its location link.
- [ ] T003 `render`: `loupe-meta` `carried=` and `carriedblocking=`.
- [ ] T004 `render`: a sticky round carrying a finding collapses with matching pills and keeps its section.
- [ ] T005 `docs/comment-format.md`.
