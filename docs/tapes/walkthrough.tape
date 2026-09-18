# The README's hero gif and the board still. Redraw every tape with `mise run screenshots`.
#
# One story, told in the time a reader will actually give it: an agent filed the findings, the human
# decided the blocking one, and the human wrote what the review says. The send-back loop is its own
# gif, docs/tapes/sendback.tape, because it is the agent's half and was competing with this one.
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
Sleep 1400ms

# Publishing needs every finding decided AND every note resolved, so the hero cannot skip to the
# confirmation: publish.ReadinessRefusal gates on both. The decisions are taken at speed here and the
# send-back loop is passed through in one keypress; docs/tapes/sendback.tape is where it is explained.
#
# f-002 is the blocking finding the board opens on, since specs/014 orders the list by severity. A
# decision advances by itself, in the draft's order rather than the list's, which is why the walk to
# f-003 is spelled out.
Enter
Sleep 1600ms
Type "a"
Sleep 1300ms
Right
Sleep 400ms
Right
Sleep 400ms
Right
Sleep 900ms
Type "r"
Sleep 1400ms
Type "a"
Sleep 1300ms
Right
Sleep 600ms
Type "x"
Sleep 1500ms

# The board again: nothing pending, no note open, publish now available. Steps 1 and 2 take their
# default, so they are shown rather than dwelt on.
Escape
Sleep 1600ms
Type "p"
Sleep 1300ms
Enter
Sleep 1100ms
Enter
Sleep 2200ms

# The point of the gif: the review opens on the human's own sentence, typed into the body where it will
# appear. The confirmation opens with the box already focused, so the text goes straight in. Slower than
# the rest, because this is the part to read rather than watch.
Set TypingSpeed 55ms
Type "The cache bug blocks this one; the rest can land."
Set TypingSpeed 50ms
Sleep 1800ms

# esc leaves the box, and only then does y send. A y typed into the message is a y in the message.
Escape
Sleep 1300ms
Type "y"
Sleep 3s
