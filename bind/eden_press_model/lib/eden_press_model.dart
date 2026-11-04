// Copyright (c) 2026 AO Cyber Systems
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// SPDX-License-Identifier: MIT

/// `eden_press_model`: the Eden Press document model (`eden-press.model/v7`)
/// as plain Dart.
///
/// [ModelDocument.fromJson] reads the JSON the Go engine serves
/// (`--format json`, `press.Output.Model`); every class's `toJson` writes the
/// bytes Go's `json.Marshal` would, so a document (or a single [Block] for an
/// `insert_block` / `update_block` op) can go back to the server unchanged.
///
/// This package has no Flutter, FFI, plugin or native dependency. Render it
/// with `eden_docs`; render Markdown through the engine with `eden_press`,
/// which re-exports this library.
library;

export 'src/model.dart';
