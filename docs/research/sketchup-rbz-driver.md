# SketchUp RBZ driver research gate

Verified: **2026-09-15**

Scope: issue #4 only.

- SketchUp 2026.1.3 remains on the Ruby 3.2 line; the latest Ruby upgrade documented by SketchUp is Ruby 3.2.2 in SketchUp 2024. No later Ruby upgrade is documented in the current 2025/2026 Ruby API release notes.
- All SketchUp Ruby API access must run on the SketchUp main thread. Background Ruby threads are therefore limited here to TCP/file/JSON work and queue operations.
- `UI.start_timer(seconds, repeat)` remains the supported timer primitive. The bridge creates one repeating timer from extension startup and drains a bounded number of queued requests per tick. `UI.stop_timer(id)` is used during shutdown.
- An RBZ must contain exactly a root `.rb` registration file and a same-named support directory. The root file only registers `SketchupExtension`; implementation lives under the support directory and uses `Sketchup.require`.
- Extension Warehouse requirements prohibit `eval`, monkey-patching SketchUp API modules/classes, self-installing update logic, and silent/destructive saves. This bridge does none of those.
- SketchUp documents TestUp as its Minitest wrapper for tests that need to execute inside SketchUp. Pure protocol/dispatcher tests remain ordinary Minitest outside SketchUp; one TestUp smoke test covers timer/main-thread behavior.

Authoritative sources:

- https://ruby.sketchup.com/
- https://ruby.sketchup.com/file.ReleaseNotes.html
- https://ruby.sketchup.com/UI#start_timer-class_method
- https://ruby.sketchup.com/file.extension_requirements.html
- https://ruby.sketchup.com/SketchupExtension.html
- https://github.com/SketchUp/testup-2
