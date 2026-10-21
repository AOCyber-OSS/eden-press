# Copyright (c) 2026 AO Cyber Systems
# SPDX-License-Identifier: MIT
#
# CocoaPods spec for the eden_press Flutter FFI plugin.
#
# The native code is the Go c-archive scripts/build-ios.sh packages as
# EdenPress.xcframework (device arm64 + a lipo'd arm64/x86_64 simulator slice).
# It is consumed as a prebuilt vendored framework; this pod compiles no
# Objective-C of its own beyond the placeholder in Classes/.
#
# static_framework is REQUIRED, not stylistic: the payload is a STATIC archive
# (.a inside the xcframework), and Go's iOS support builds c-archive rather
# than c-shared. That is also why lib/src/native_loader.dart uses
# DynamicLibrary.process() on iOS instead of .open() — the symbols are linked
# into the app binary itself, so there is no separate library to open.
#
# The xcframework is NOT committed (.gitignore excludes *.xcframework/). Run
# `make dart-native-vendor` after scripts/build-ios.sh to place it here.
Pod::Spec.new do |s|
  s.name             = 'eden_press'
  s.version          = '0.1.0'
  s.summary          = 'JS-free Dart/Flutter rendering surface for Eden Press.'
  s.description      = <<-DESC
Binds the Eden Press C ABI (PressRender/PressFree) via dart:ffi, rendering
press.Output.Model's schema-v3 blocks natively. No JavaScript, no HTML/DOM
parsing, anywhere in the render path.
                       DESC
  s.homepage         = 'https://github.com/AOCyber-OSS/eden-press'
  s.license          = { :type => 'MIT', :file => '../../../LICENSE' }
  s.author           = { 'AO Cyber Systems' => 'support@aocyber.ai' }
  s.source           = { :path => '.' }

  s.source_files     = 'Classes/**/*'
  s.dependency 'Flutter'
  s.platform = :ios, '12.0'

  s.vendored_frameworks = 'EdenPress.xcframework'
  s.static_framework = true

  # Flutter.framework does not contain a i386 slice.
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES', 'EXCLUDED_ARCHS[sdk=iphonesimulator*]' => 'i386' }
end
