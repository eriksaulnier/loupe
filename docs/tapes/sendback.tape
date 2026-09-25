# The send-back loop, which is the agent's half of a round: a note the human sent, the agent's answer,
# the human resolving it, and the box the next note is typed into. It is its own gif rather than a beat
# in the hero one, which is about the human's own decisions. See docs/tapes/walkthrough.tape for the
# settings these share and why.

Output docs/assets/sendback.gif

Set Shell "bash"
Set Width 1100
Set Height 560
Set FontSize 16
Set Padding 20
Set Framerate 24
Set Theme "Catppuccin Mocha"
Set WindowBar "Colorful"

Env LOUPE_ICONS "unicode"
Env TERM "xterm-256color"

Hide
Type "clear" Enter
Sleep 300ms
Type "dist/loupe-demo review acme/widgets#42" Enter
Sleep 4s
Show

Sleep 1200ms

# f-003 carries the answered note in the seed, the second row once the board is ordered by severity.
# The cursor stops on it, so the note glyph on f-003 is visible before it is opened.
Type "j"
Sleep 1200ms
Enter

# The open note and the agent's reply to it, then resolving it.
Sleep 3s
Type "r"
Sleep 1400ms
# The thread is taller than the room left under the finding, so it is scrolled to its end, where the
# agent's reply is.
PageDown
Sleep 2800ms
Type "a"
Sleep 2500ms

# Accepting moves on to the next finding in the list's order, f-001, where a new note is started in the
# send-back box and left, which keeps its text for the next s.
Type "s"
Sleep 900ms
Type@60ms "Could the network call move outside the lock?"
Sleep 2500ms
Escape
Sleep 1500ms
