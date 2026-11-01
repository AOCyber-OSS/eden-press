# eden_docs

Standalone Flutter package that paints an Eden `eden-press.model/v7`
document as real block widgets (headings, todos, callouts, sheet embeds,
figures, toggles, math, code, …). It does **not** dump JSON.

## Modes (client chooses)

| Client | Ask Go for | Render with |
|---|---|---|
| Flutter app | `--format json` / `?format=json` (`Accept: application/json`) | this package (`EdenDocsView`) |
| Browser / HTML host | `--format html` / `?format=html` (default) | Go-assembled HTML |

```dart
import 'package:eden_docs/eden_docs.dart';
import 'package:eden_press/eden_press.dart';

// From a decoded model (JSON envelope's `.model`):
EdenDocsView(document: model);

// Or from wire JSON bytes:
EdenDocsView.fromJson(jsonMap);
```

Depends on `eden_press` only for the shared model types / JSON parse.
