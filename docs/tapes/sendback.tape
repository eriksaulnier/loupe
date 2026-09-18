# The send-back loop, which is the agent's half of a round: a note the human sent, the agent's answer,
# and the human resolving it. It is its own gif rather than a beat in the hero one, which is about the
# human's own decisions. See docs/tapes/walkthrough.tape for the settings these share and why.
#
# f-003 carries the answered note in the seed, five rows down once the board is ordered by severity.

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

# One row at a time, so the note glyph on f-003 is visible before it is opened.
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 1200ms
Enter

# The open note and the agent's reply to it, then resolving it. The still comes after r, not before:
# while the note is open the footer carries r and d, wraps to a second row, and takes the row the
# reply would render on.
Sleep 3s
Type "r"
Sleep 2200ms
Screenshot docs/assets/detail.png
Sleep 1600ms
Type "a"
Sleep 2500ms
