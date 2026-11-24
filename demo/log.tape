# The README's log recording: rows that say how big each revision is, and the
# diff docked beside the log, following the cursor. `mise run demo` builds
# ./jojo and runs this. demo/scene.sh builds a throwaway jj repository from
# invented content, with a home of its own.

Output docs/images/log.gif

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 480
Set Padding 18
Set Theme "Catppuccin Mocha"
Set Framerate 8
Set TypingSpeed 90ms

Env COLORTERM "truecolor"
Env ARAGONITE_ICONS "unicode"

Hide
Type "demo/scene.sh log" Enter
Sleep 8s
Show

# Each row: change id, bookmarks, description, files and lines touched, age.
Sleep 2500ms
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "k"
Sleep 700ms
Type "k"
Sleep 1s

# Dock the diff; it follows the cursor.
Type "p"
Sleep 2s
Type "j"
Sleep 2s
Type "j"
Sleep 1500ms
Type "j"
Sleep 1500ms

# Into the dock, stepping hunks, then marking one read.
Tab
Sleep 800ms
Type "]h"
Sleep 1200ms
Type "]h"
Sleep 1200ms
Space
Sleep 1500ms
Tab
Sleep 1s

Type "q"
Sleep 500ms
