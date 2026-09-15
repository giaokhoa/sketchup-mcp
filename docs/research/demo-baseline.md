# Demo research baseline

Verified: **2026-09-15**

This file records only facts and decisions that implementation issues may rely on. It is not a general MCP or SketchUp tutorial.

## Selected baseline

| Area | Selected baseline | Verification |
| --- | --- | --- |
| MCP specification | `2026-07-28` | Current published MCP specification and release announcement |
| Official MCP Go SDK | `github.com/modelcontextprotocol/go-sdk v1.8.0` | Latest stable release published 2026-09-14 08:05 UTC |
| Go language floor | Go `1.25.0` or newer | v1.8.0 `go.mod` declares `go 1.25.0` |
| Public MCP transport | stdio | Smallest common local transport supported by Codex and Claude Desktop |
| Private SketchUp transport | loopback TCP, bridge protocol v1 | Project decision; see ADR 0001 |
| Demo OS | Windows 11 24H2 x64, build 26100 | Pinned validation target |
| Demo SketchUp | SketchUp Desktop 2026 for Windows, major `26.x` (`>=26.0,<27.0`), 64-bit; smoke verified on `26.0.429` | Official SketchUp 2026 docs + real Windows smoke |

Authoritative sources:

- MCP specification: https://modelcontextprotocol.io/specification/2026-07-28
- MCP 2026-07-28 release: https://blog.modelcontextprotocol.io/posts/2026-07-28/
- Official Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Go SDK v1.8.0 release: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0
- Go SDK protocol notes: https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md
- Go SDK package docs: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp
- SketchUp Ruby API: https://ruby.sketchup.com/
- SketchUp Desktop 2026 release notes: https://help.sketchup.com/en/release-notes/sketchup-desktop-20260
- SketchUp compatibility: https://help.sketchup.com/cs/sketchup-requirements-compatibility-considerations

## MCP facts implementation may rely on

### Current protocol

MCP `2026-07-28` is the current published specification as of the verification date.

Relevant changes for this project:

- the protocol core is stateless;
- `initialize` / `initialized` and protocol-level `Mcp-Session-Id` were retired for `2026-07-28`;
- each request is self-contained and carries protocol/client capability metadata;
- `server/discover` exists for discovery but is not required before every request;
- an application may still carry explicit state handles in tool arguments.

Therefore the SketchUp `session_id` is an application-level argument. It must not be inferred from an MCP transport session.

### Go SDK version and negotiation

Official Go SDK `v1.8.0` was published on 2026-09-14. Its release notes state that `2026-07-28` remains the newest revision it negotiates.

The official compatibility table for `v1.7.0+` lists:

- `2026-07-28`;
- `2025-11-25`;
- `2025-06-18`;
- `2025-03-26`;
- `2024-11-05`.

The v1.8.0 SDK can narrow versions with `ServerOptions.SupportedProtocolVersions`. The demo must **not** narrow that set initially. This allows a current client to use `2026-07-28` and lets an initialization-era client negotiate an older revision supported by the same SDK.

This compatibility choice is intentional because the current Codex documentation still describes server instructions returned during initialization, while the latest MCP specification no longer requires that handshake.

### Minimal official stdio server pattern

Use the official typed SDK pattern, not a third-party MCP implementation:

```go
server := mcp.NewServer(
    &mcp.Implementation{Name: "sketchup-mcp", Version: version},
    nil,
)

mcp.AddTool(server, toolDefinition, typedHandler)

if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
    // log to stderr, then exit non-zero
}
```

The v1.8.0 README's typed example uses an input struct and output struct:

```go
type Input struct {
    Name string `json:"name" jsonschema:"the name of the person to greet"`
}

type Output struct {
    Greeting string `json:"greeting" jsonschema:"the greeting to tell to the user"`
}

func Handler(
    ctx context.Context,
    req *mcp.CallToolRequest,
    input Input,
) (*mcp.CallToolResult, Output, error)
```

Implementation issues may rely on the generic `mcp.AddTool` path to derive/validate tool schemas from typed inputs and outputs. Keep hand-written untyped JSON maps out of the public tool boundary unless the SDK cannot express a required shape.

Successful tools should return the typed output so clients receive structured tool output. Human-readable text content may be added only when it materially improves client presentation; it is not the source of truth.

### Stdio constraints

For a local stdio server:

- stdin/stdout belong to MCP framing;
- stdout must contain protocol traffic only;
- logs and diagnostics go to stderr;
- Go SDK v1.8.0 added `StdioTransport.MaxLineLength` to bound one JSON-RPC frame.

Do not print startup banners, debug output, or progress messages to stdout.

### MCP resources

Resources/resource templates are not required for the first demo.

The live desktop model is stateful and changes independently of the client. Explicit tools with `session_id`, `model_guid`, and revision metadata give clearer freshness and mutation semantics than introducing a resource URI scheme at this stage.

Reconsider resources only if a later issue identifies a read-only object that benefits materially from URI addressing/caching.

## SketchUp Ruby API facts implementation may rely on

### Main-thread requirement

The official SketchUp Ruby API release notes state that all access to the SketchUp API, including console output, must occur from the main thread.

Implication:

- model/entity/UI access must run in a SketchUp callback on the main thread;
- the local session bridge uses a main-thread repeating timer with bounded nonblocking socket I/O (`accept_nonblock`, `read_nonblock`, `write_nonblock`) because that pattern was verified against the target SketchUp 26.0.429 host;
- transport code must remain bounded per tick and must not perform model/entity work.

Authoritative source:

- https://ruby.sketchup.com/file.ReleaseNotes.html

### `UI.start_timer`

`UI.start_timer(seconds, repeat = false)` schedules a Ruby block after the requested delay and supports repeating timers.

The bridge timer pattern selected by ADR 0001 is:

1. create a bounded repeating transport timer from the main thread during extension startup;
2. the transport timer accepts/reads/writes sockets only through Ruby nonblocking socket APIs and pushes validated pure-data work items into a thread-safe queue;
3. the existing dispatcher timer drains a bounded number of work items;
4. only dispatcher/observer code invokes model/entity APIs.

This keeps both transport and SketchUp API execution cooperative with the host UI loop while preserving a strict API boundary.

Authoritative source:

- https://ruby.sketchup.com/UI#start_timer-class_method

Known API note: SketchUp documents a modal-window bug for non-repeating timers. The bridge should not depend on a one-shot timer being fired from a background thread.

### Undoable operations

From `Sketchup::Model`:

- `start_operation` begins an undoable operation;
- operations are sequential and cannot be nested;
- SketchUp recommends `disable_ui = true`;
- `commit_operation` commits the operation to the undo stack;
- `abort_operation` cancels the current operation and is normally used from error handling;
- aborting a transparent operation is explicitly warned against.

Bridge mutations therefore use:

```ruby
model.start_operation(label, true)
begin
  # mutation
  model.commit_operation
rescue
  model.abort_operation
  raise
end
```

The real implementation must guard the abort path so it does not abort after a successful commit. Bridge operations are not transparent.

Authoritative source:

- https://ruby.sketchup.com/Sketchup/Model.html

### Persistent entity identity

`Entity#persistent_id`:

- is a persistent identifier;
- persists between SketchUp sessions;
- is supported only by a documented subset of entity types.

`Model#find_entity_by_persistent_id` resolves one or more persistent IDs and may return `nil` for IDs that cannot be found.

The demo entity reference is therefore:

`model_guid + persistent_id`

with an explicit observed model revision.

Do not fall back to `entityID`, Ruby object ids, names, array positions, or object addresses for durable references. If a type does not support a persistent ID, it is not durably addressable by bridge v1.

Authoritative sources:

- https://ruby.sketchup.com/Sketchup/Entity.html#persistent_id-instance_method
- https://ruby.sketchup.com/Sketchup/Model.html#find_entity_by_persistent_id-instance_method

### Transaction observers and revision semantics

Current `Sketchup::ModelObserver` documentation states that transaction-related observer events are queued and, since SketchUp 2016, fire at commit time rather than when `start_operation` is called.

It also warns against editing the model from transaction observer callbacks because doing so can cause crashes or model corruption.

Relevant callbacks:

- `onTransactionCommit`: non-empty committed transaction;
- `onTransactionEmpty`: committed transaction with no model changes;
- `onTransactionAbort`: aborted transaction;
- `onTransactionUndo`: undo;
- `onTransactionRedo`: redo.

Bridge v1 revision rules are therefore:

- +1 on `onTransactionCommit`;
- +1 on `onTransactionUndo`;
- +1 on `onTransactionRedo`;
- no increment on empty or aborted transactions.

An observer callback may update the revision counter but must not edit model geometry/state.

Authoritative source:

- https://ruby.sketchup.com/Sketchup/ModelObserver.html

### RBZ structure and Extension Warehouse constraints

Current Extension Requirements specify:

- an RBZ is a ZIP archive with the `.rbz` extension;
- it contains exactly two top-level items: a root `.rb` registration file and a same-named support folder;
- the root file should register the extension and should not contain extension logic;
- extension code must use a unique wrapping module;
- `Sketchup.require` is preferred for extension files;
- user model changes should be grouped into undoable operations;
- unconditional `puts`, `print`, and `p` should be removed/disabled before publishing;
- `eval` should not be used because it enables code injection;
- gems are problematic in SketchUp; vendored code under the extension namespace is preferred when third-party Ruby code is unavoidable;
- Extension Warehouse update infrastructure must not be bypassed by self-installing update code.

Bridge v1 exposes no arbitrary Ruby evaluation endpoint.

Authoritative source:

- https://ruby.sketchup.com/file.extension_requirements.html

## SketchUp product background

The official SketchUp Claude Connector page, last updated 2026-08-21, states that the connector can create `.skp` files but cannot edit or render an existing `.skp` file, and that iterations create a new file.

That limitation is exactly why this project uses a local in-process SketchUp extension plus a Go MCP host: the demo target is live interaction with the model already open in SketchUp for Desktop.

Authoritative source:

- https://help.sketchup.com/de/sketchup-claude-connector

## Current local MCP client setup

### Codex

Verified against current official OpenAI Codex MCP documentation on 2026-09-15.

Codex stores MCP server configuration in:

- user scope: `~/.codex/config.toml`;
- trusted project scope: `.codex/config.toml`.

Official CLI pattern for a local stdio server:

```text
codex mcp add sketchup -- C:\absolute\path\to\sketchup-mcp.exe
```

Equivalent TOML shape:

```toml
[mcp_servers.sketchup]
command = 'C:\absolute\path\to\sketchup-mcp.exe'
```

Optional `args`, `env`, `env_vars`, and `cwd` are supported by the current configuration model.

For the first demo, prefer the CLI add command so Codex owns the exact config serialization.

Authoritative source:

- https://developers.openai.com/codex/mcp
- current canonical documentation resolves under https://learn.chatgpt.com/docs/extend/mcp?surface=cli

### Claude Desktop

Anthropic's current guidance recommends **Desktop Extensions** (`.mcpb`) as the normal installation/distribution path for local MCP servers. The June 30, 2026 help article says Desktop Extensions support binary MCP servers.

For development/manual local stdio, Claude Desktop still supports locally configured servers through `claude_desktop_config.json`. Anthropic's August 11, 2026 connector article explicitly distinguishes those local servers from remote MCP connectors.

Windows config location:

`%APPDATA%\\Claude\\claude_desktop_config.json`

Manual stdio shape for the demo binary:

```json
{
  "mcpServers": {
    "sketchup": {
      "command": "C:\\absolute\\path\\to\\sketchup-mcp.exe",
      "args": []
    }
  }
}
```

Fully restart Claude Desktop after changing its local config.

For this repository:

- manual local stdio configuration is acceptable for the first developer demo;
- `.mcpb` packaging is a later distribution concern;
- remote custom connectors are not a substitute because they connect from Anthropic infrastructure and are not the local-desktop bridge required here.

Authoritative sources:

- https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop
- https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp
- official MCP SDK host example: https://py.sdk.modelcontextprotocol.io/get-started/real-host/

## Bridge constraints fixed for implementation

These are project decisions from ADR 0001, not upstream API claims:

- loopback only: `127.0.0.1`;
- per-user discovery: `%LOCALAPPDATA%\\SketchUpMCP\\sessions`;
- one random `session_id` and 256-bit token per SketchUp bridge lifetime;
- length-prefixed UTF-8 JSON: 4-byte big-endian length + payload;
- bridge protocol version: `1`;
- max frame: 1 MiB;
- max outstanding requests per connection: 32;
- request timeout accepted by Ruby: 100 ms through 30 s;
- no unsolicited events in v1;
- mutations require `operation_id`, expected model GUID, and exact expected revision;
- mutation idempotency is process/session scoped;
- no arbitrary Ruby source or `eval`.

## Rejected approaches

### Make the SketchUp extension itself an MCP server

Rejected. It would duplicate protocol/client compatibility work in Ruby and couple SketchUp's UI-thread constraints to the public protocol. The Go host remains the single official MCP boundary.

### Use MCP internally between Go and SketchUp

Rejected. The internal needs are discovery, authenticated local session selection, revision checks, and serialized UI-thread execution. MCP adds unnecessary public-protocol surface and would blur state ownership.

### Call SketchUp APIs from the socket thread

Rejected by the SketchUp API main-thread requirement.

### Use a fixed TCP port

Rejected because multiple SketchUp processes must coexist and stale process state must not prevent startup. Each instance binds an ephemeral loopback port and publishes a descriptor.

### Use named pipes first

Rejected for the first demo. Windows named-pipe support is possible, but loopback TCP is simpler to implement using Ruby's standard library and still meets the local-only requirement. Revisit only if loopback causes a concrete deployment problem.

### Use `entityID` or Ruby object identity

Rejected because bridge references must survive ordinary object re-fetching and, for supported entities, SketchUp provides persistent IDs specifically for this purpose.

### Ignore user edits between read and mutation

Rejected. Every mutation requires an exact expected revision and fails stale rather than silently applying against a model state the caller did not observe.

### Mutate from `ModelObserver` callbacks

Rejected by current SketchUp guidance; callbacks update revision/bookkeeping only.

### Add resources/resource templates now

Rejected because they do not materially simplify the demo. Typed tools are sufficient and keep freshness/revision semantics explicit.

### Add arbitrary Ruby evaluation

Rejected for security and Extension Warehouse compliance. The bridge exposes a fixed allowlist of operations only.

### Add cloud, OAuth, HTTP MCP, Desktop SDK, C++, or remote access

Rejected as outside issue #2 and outside the first demo. None is required to prove local live editing of an existing SketchUp desktop model.
