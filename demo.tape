#!vhs demo.tape
Output demo.gif

Require sqlite-migrate

Set Rows 33
Set TypingSpeed 80ms

Type "sqlite-migrate deploy main.db"
Sleep 500ms
Enter
Sleep 1s

Type "sqlite-migrate status main.db"
Sleep 500ms
Enter
Sleep 1s

Type "sqlite-migrate rollback --target 1 main.db"
Sleep 500ms
Enter
Sleep 1s

Type "sqlite-migrate status main.db"
Sleep 500ms
Enter

# hold open for a while to allow a good read
Sleep 10s
