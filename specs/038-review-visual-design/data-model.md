# Display data model

No persisted entity or schema changes. `draft.Draft`, findings, notes, decisions, preview, receipt, and captured target retain their existing formats.

## Progress projection

Input is `draft.Ordered`, `draft.Dispositions`, and open note records. A counted finding has one display state: accepted, excluded, pending with open note, pending blocking, or pending. Precedence follows that order. Derived withdrawn findings have no bar cell. `decided = accepted + excluded`. `total = decided + pending`. A reply does not close a note. Counts MUST be nonnegative and `decided <= total`.

The grouped bar uses a width of 20 cells. Every nonempty state receives one cell. Remaining cells use each state's share of the total, rounded down, then largest remainders in state order. Counts remain exact even though grouped cells approximate them. The style helper takes generic display states, not draft objects.

## Pane and theme inputs

A pane receives outer width/height, four border-title slots, content, focus, and optional scroll offset/visible/total rows. Content width is `max(0, width - 4)` and content height is `max(0, height - 2)`. The scroll thumb occupies the right border. The theme receives dark/light, terminal color profile, icon tier, and the explicit no-color gate. Appearance belongs to `internal/style`.

## Session additions

A notice gains a generation token and optional expiry. Success notices expire after two seconds. Errors have no expiry. A late expiry MUST match the active token before clearing it. Browser requests carry a request id, target URL, and origin context. Completion of a list/detail request MUST NOT quit or overwrite an active editor. The current underlying view remains authoritative if navigation occurs before completion.

Summary expansion stores the human preference separately from automatic height collapse. Resizing recomputes rectangles without clearing selection, pending input, offsets, or confirmation state. The existing editor, note, and publication state machines remain authoritative.

## Publication display

The tab state derives from existing Action, Inline, and Confirm views. The Message explanation on earlier steps is display text, not a stored review message. The confirmation input remains human-authored and session-local. Preview content MUST come from the existing compose result, including sticky history and warnings. No appearance helper may compose or send a review.
