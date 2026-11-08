#!vhs demo.tape

Require smidge

Set FontFamily "Consolas"
Set FontSize 34
Set Rows 36

Type "smidge deploy main.db"
Enter

Type "smidge status main.db"
Enter

Type "smidge rollback --target 1 main.db"
Enter

Type "smidge status main.db"
Enter

Screenshot demo.png
