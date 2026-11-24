# The README's palette recording: the command palette finding jj's verb from a
# git word, and the legend explaining a key when it is pressed.
# `mise run demo` builds ./jojo and runs this.

Output docs/images/palette.gif

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 560
Set Padding 18
Set Theme "Catppuccin Mocha"
Set Framerate 8
Set TypingSpeed 90ms

Env COLORTERM "truecolor"
Env ARAGONITE_ICONS "unicode"

Hide
Type "demo/scene.sh palette" Enter
Sleep 8s
Show

Sleep 1500ms

# A git word finds the jj verb, with the command it runs and its flags.
Type ":"
Sleep 1500ms
Type "amend"
Sleep 3s
Escape
Sleep 800ms

# The legend lists every key here; pressing one explains it instead.
Type "?"
Sleep 2s
Type "r"
Sleep 3s
Type "S"
Sleep 3s
Escape
Sleep 800ms

Type "q"
Sleep 500ms
