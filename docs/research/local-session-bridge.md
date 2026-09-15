# Local SketchUp session bridge research

Verified: **2026-09-15**

Scope: authenticated local SketchUp session discovery, transport, and liveness on Windows.

## Official sources checked

- SketchUp Ruby API overview: https://ruby.sketchup.com/
- SketchUp UI.start_timer: https://ruby.sketchup.com/UI#start_timer-class_method
- SketchUp AppObserver#expectsStartupModelNotifications: https://ruby.sketchup.com/Sketchup/AppObserver.html#expectsStartupModelNotifications-instance_method
- SketchUp Extension Requirements: https://ruby.sketchup.com/file.extension_requirements.html
- Ruby 3.2 TCPServer#accept_nonblock: https://docs.ruby-lang.org/en/3.2/Socket.html
- Ruby 3.2 IO#read_nonblock / IO#write_nonblock: https://docs.ruby-lang.org/en/3.2/IO.html
- Go os.UserCacheDir: https://pkg.go.dev/os#UserCacheDir

## Findings

### SketchUp threading boundary

The official SketchUp Ruby API states that all SketchUp API interactions must run on the main thread.

The existing MainThreadDispatcher isolates SketchUp API work on the main thread; the local session bridge must preserve that boundary.

### Timer primitive

UI.start_timer(seconds, repeat) is the documented repeating callback primitive in SketchUp.

The smallest host-compatible transport pattern is:

1. bind TCPServer on 127.0.0.1 with an ephemeral port;
2. create one repeating transport timer on the SketchUp main thread;
3. each tick performs bounded nonblocking socket I/O only;
4. decoded authenticated requests are pushed to the existing request queue;
5. the existing dispatcher timer executes SketchUp API operations;
6. completed responses are queued back to the connection and flushed nonblocking on later transport ticks.

No Ruby background thread is required.

### Ruby 3.2 nonblocking socket APIs

Ruby 3.2 documents:

- TCPServer#accept_nonblock(exception: false) returning :wait_readable when no connection is ready;
- IO#read_nonblock(..., exception: false) returning :wait_readable and nil at EOF;
- IO#write_nonblock(..., exception: false) returning :wait_writable;
- nonblocking writes may be partial and the caller must retain the unwritten suffix.

These operations are used only with sockets. The Ruby docs note that Windows limitations on nonblocking I/O apply to some non-socket IO objects; sockets are the intended use here.

### Windows discovery directory

Go documents `os.UserCacheDir()` as `%LocalAppData%` on Windows. The Go host therefore resolves the ADR discovery root with `os.UserCacheDir()` and appends `SketchUpMCP/sessions`; no Windows-specific filesystem wrapper is needed. `os.UserConfigDir()` is intentionally not used because it maps to `%AppData%`.

### Startup model lifecycle

AppObserver#expectsStartupModelNotifications returning true is the documented opt-in for startup model notifications. The AppObserver documentation also carries a timing caveat for onOpenModel when a .skp is opened from the command line, so extension startup must not rely on that callback alone. The runtime already reads Sketchup.active_model during startup and uses observers for subsequent lifecycle changes.

No Welcome-window workaround belongs in the extension lifecycle. For unattended smoke testing, launching SketchUp with a shipped .skp template provides a real active model without UI automation.

### Extension structure

The current root registration file + same-named support directory matches the official Extension Requirements. Keep that layout unchanged.

## Real SketchUp 26.0.429 observation

Observed on Windows with SketchUp 26.0.429:

- while the Welcome window is shown, user plugin files are not loaded yet;
- launching SketchUp with a shipped .skp template enters the model and loads the extension;
- the existing background-thread bridge published its descriptor but did not answer the Go client's first handshake frame before the 5 s Go deadline.

This observation motivates replacing the Ruby background socket threads with the documented timer + nonblocking socket pattern. It does not change bridge protocol v1.

## Minimal implementation decision

Keep unchanged:

- loopback TCP;
- ephemeral port;
- descriptor format;
- per-session token;
- protocol v1 framing and schemas;
- Go client/registry;
- existing main-thread command dispatcher.

Change only the Ruby transport execution model:

- remove accept/read/write Ruby background threads;
- add one bounded repeating transport timer;
- use accept_nonblock, read_nonblock, and write_nonblock;
- retain bounded frame size, connection limits, handshake/idle/write deadlines, request correlation, and outstanding-request caps.

This transport layer contains no model mutation commands.

