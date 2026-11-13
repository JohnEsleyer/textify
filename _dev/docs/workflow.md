
- Use textify to apply patches.
- Use cat to write _dev/scripts/patch.xml
- Then perform dry-run: textify patch _dev/scripts/patch.xml -n
- If the human verifies the dry run. The human will perform: textify patch _dev/scripts/patch.xml

If you want to read more information then you have to write _dev/scripts/request.xml and write requests for full files not snippets.