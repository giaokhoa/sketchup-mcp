# SketchUp RBZ manual smoke test

Target compatibility line: Windows 11 x64 + SketchUp Desktop 2026 `26.x` (`>=26.0,<27.0`). The local session bridge was smoke-tested on SketchUp 26.0.429.

1. Build twice and verify identical hashes:
   - `python scripts/package_rbz.py --output dist/first.rbz`
   - `python scripts/package_rbz.py --output dist/second.rbz`
   - `certutil -hashfile dist\first.rbz SHA256`
   - `certutil -hashfile dist\second.rbz SHA256`
2. Inspect either archive. Its only top-level entries must be `giaokhoa_sketchup_mcp.rb` and `giaokhoa_sketchup_mcp/`.
3. In SketchUp, open **Extension Manager > Install Extension** and install the RBZ. SketchUp should load it without a Ruby load error or UI freeze.
4. Confirm `%LOCALAPPDATA%\SketchUpMCP\sessions\` contains one JSON descriptor for the running SketchUp process. The descriptor must advertise `127.0.0.1`, an ephemeral port, protocol version `1`, the process id, and a per-start token.
5. With TestUp installed, run `test/sketchup/main_thread_dispatcher_smoke_test.rb`. The dispatcher smoke must execute `session.info` on TestUp's SketchUp main thread and access `Sketchup.active_model` successfully.
6. Exercise bridge protocol v1 with a compatible client: perform `hello`, `authenticate`, then call `system.ping` and `session.info`. Both commands must return through the main-thread dispatcher. `session.info` should report the bridge session id, process id, SketchUp version, model GUID/title, and revision.
7. Disable the extension or exit SketchUp. The session descriptor must be removed and the loopback listener must stop accepting connections.

Do not use the SketchUp Ruby Console to invoke bridge internals from a background thread; that would bypass the behavior being tested.
