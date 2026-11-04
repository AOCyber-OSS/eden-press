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
import 'package:eden_press_model/eden_press_model.dart';

// From a decoded model (JSON envelope's `.model`):
EdenDocsView(document: model);

// Or from wire JSON bytes:
EdenDocsView.fromJson(jsonMap);
```

Depends on the pure-Dart `eden_press_model` (`../eden_press_model`) for the
model types and their JSON, **not** on the `eden_press` FFI plugin. A host app
can therefore depend on `eden_docs` with no native artifacts (no
`EdenPress.xcframework`, no prebuilt `jniLibs`). As a git dependency the path
dependency resolves inside the same checkout:

```yaml
eden_docs:
  git:
    url: https://git.aocyber.ai/eden-press.git
    path: bind/eden_docs
    ref: vX.Y.Z
```

`test/pubspec_contract_test.dart` fails if `eden_press` ever comes back.
