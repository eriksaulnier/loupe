# The publish still. See docs/tapes/walkthrough.tape for the settings these share and why.
#
# acme/widgets#43 is seeded ready to publish with a blocking finding included, so the envelope shows a
# comment review rather than an approval. The tape stops at the confirmation and never presses y.

# vhs needs an Output even when only the screenshot is kept, so the gif goes to the ignored dist/.
Output dist/publish.gif

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
Type "dist/loupe-demo review acme/widgets#43" Enter
Sleep 4s
Show

Sleep 500ms
# Step 1 picks the review action, step 2 the inline comments; both take their default.
Type "p"
Sleep 1500ms
Enter
Sleep 1500ms
Enter
Sleep 3s
Screenshot docs/assets/publish.png
Sleep 2s
