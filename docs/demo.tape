# Records docs/demo.gif with `vhs docs/demo.tape`.
# Read-only session: no key in here writes to the robot.
Output docs/demo.gif

Set FontSize 15
Set Width 1280
Set Height 760
Set Padding 12
Set Theme "Catppuccin Mocha"
Set Shell "bash"
Set TypingSpeed 40ms

# Keep the card on screen long enough to read, then step through the panes.
Hide
Type "cd /Users/simsies/github/sharkboard && ./sharkboard -host 192.168.1.79 -dry-run"
Enter
Sleep 4s
Show

Sleep 3s
Type "x"
Sleep 1s

Type "2"
Sleep 2s
Type "j"
Sleep 1s
Type "a"
Sleep 1s

Type "4"
Sleep 2s

Type "1"
Sleep 2s
Type "?"
Sleep 2s
Type "x"

Type "q"
Sleep 1s
