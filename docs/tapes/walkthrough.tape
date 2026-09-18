# The README's hero gif and its two stills. Redraw every tape with `mise run screenshots`.
#
# Nothing overlays the keys being pressed; vhs cannot. The pauses are sized so the footer legend and
# the notice loupe prints after each decision are both readable.
#
# vhs writes no file for a Screenshot taken in the last frame, so the tape MUST NOT end on one.

Output docs/assets/walkthrough.gif

Set Shell "bash"
Set Width 1100
Set Height 560
Set FontSize 16
Set Padding 20
Set Framerate 24
Set Theme "Catppuccin Mocha"
Set WindowBar "Colorful"

# The default icon tier, so no Nerd Font is needed to draw these. NO_COLOR is deliberately unset: the
# tests turn color off, the README wants it on.
Env LOUPE_ICONS "unicode"
Env TERM "xterm-256color"

# Launch off camera, so neither the prompt nor the command appears. acme/widgets#42 is seeded mid-review
# by midReview in cmd/loupe-demo/seed.go; the key sequence below reads that state, so a change to the
# seed is a change to this tape.
Hide
Type "clear" Enter
Sleep 300ms
Type "dist/loupe-demo review acme/widgets#42" Enter
Sleep 4s
Show

Sleep 1s
Screenshot docs/assets/list.png
Sleep 1800ms

# f-002 is the blocking finding the cursor already sits on. Accept it; a decision advances by itself.
Enter
Sleep 2500ms
Type "a"
Sleep 2300ms

# f-003 is where the send-back loop shows: the note, the agent's reply and the resolution. The still
# comes after r, not before: while the note is open the footer carries r and d, wraps to a second row,
# and takes the row the reply would render on.
Sleep 2s
Type "r"
Sleep 2300ms
Screenshot docs/assets/detail.png
Sleep 1800ms
Type "a"
Sleep 2200ms

# f-004 is a question with no location, and the one worth dropping.
Type "x"
Sleep 2200ms

# The board again: nothing pending, no note open, publish now available.
Escape
Sleep 2800ms

# Three steps, then the one key that sends it. Steps 1 and 2 take their default. Publishing needs every
# finding decided AND every note resolved; publish.ReadinessRefusal gates on both.
Type "p"
Sleep 2000ms
Enter
Sleep 1800ms
Enter
Sleep 2600ms

# The last beat, and the point of the whole gif: the review opens on the human's own sentence, typed
# into the body where it will appear. The confirmation opens with the box already focused, so the text
# goes straight in. Slower than the default, because this is the part to read rather than watch.
Set TypingSpeed 55ms
Type "The cache bug blocks this one; the rest can land."
Set TypingSpeed 50ms
Sleep 2200ms

# esc leaves the box, and only then does y send. A y typed into the message is a y in the message.
Escape
Sleep 1600ms
Type "y"
Sleep 4s
