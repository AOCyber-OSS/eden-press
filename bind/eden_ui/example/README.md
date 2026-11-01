# Eden Desktop Demo

Host shell for driving `EdenView` on Linux desktop: Sheets, Slides, and Docs.

```bash
export PATH="$HOME/flutter/bin:$PATH"   # or wherever Flutter 3.47.5+ lives
flutter pub get
flutter run -d linux
```

The host owns workbook bytes: spreadsheet `onEdit` events are applied into the
demo workbook so switching surfaces does not discard edits.
