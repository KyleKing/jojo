# The README's picker recording: squash turns the log into a destination
# picker that previews the graph as it will be, and u takes it back.
# `mise run demo` builds ./jojo and runs this.

Output docs/images/picker.gif

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 400
Set Padding 18
Set Theme "Catppuccin Mocha"
Set Framerate 8
Set TypingSpeed 90ms

Env COLORTERM "truecolor"
Env ARAGONITE_ICONS "unicode"

Hide
Type "demo/scene.sh picker" Enter
Sleep 8s
Show

Sleep 2s

# Squash the working copy: the parent is the default destination.
Type "s"
Sleep 2500ms

# Moving the destination redraws the graph and the jj line under it.
Type "j"
Sleep 1500ms
Type "j"
Sleep 2500ms
Type "k"
Sleep 1s
Type "k"
Sleep 1500ms

# Run it as one operation; the footer names how to take it back.
Enter
Sleep 3s
Type "u"
Sleep 3s

Type "q"
Sleep 500ms
