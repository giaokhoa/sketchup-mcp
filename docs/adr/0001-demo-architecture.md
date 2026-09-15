# ADR 0001: Demo architecture and bridge contract

- Status: Accepted
- Date: 2026-09-15
- Scope: first local Windows demo
- Parent issue: #2

## Context

The demo must let an MCP client inspect and mutate a model that is already open in SketchUp for Desktop. The public integration should use the official MCP Go SDK, while SketchUp-specific code must obey the Ruby API's main-thread and undo/transaction rules.

The current MCP specification is stateless at the protocol layer. SketchUp, in contrast, is a stateful desktop process and more than one SketchUp process may be running. Therefore the SketchUp process/session is an explicit application-level handle, not hidden MCP transport state.

Research supporting these decisions is recorded in [../research/demo-baseline.md](../research/demo-baseline.md).

## Decision

### 1. Public boundary: MCP over stdio

The public server is a Go executable using the official `github.com/modelcontextprotocol/go-sdk/mcp` package and `mcp.StdioTransport`.

For the demo:

- stdio is the only MCP transport;
- MCP tools are the only public capability required;
- each tool that operates on SketchUp takes an explicit `session_id`;
- tool inputs/outputs are typed Go values so the SDK generates and validates JSON Schemas;
- successful tool results use structured output;
- resources, resource templates, prompts, remote MCP, OAuth, and HTTP transports are out of scope.

The server targets MCP `2026-07-28` semantics but does **not** narrow `ServerOptions.SupportedProtocolVersions` for the demo. The v1.8.0 SDK may therefore negotiate an older supported revision with a client that has not yet moved to `2026-07-28`.

### 2. Internal boundary: private bridge, not MCP

The Go process talks to each SketchUp process through a private bridge protocol over loopback TCP.

The bridge is deliberately not another MCP server. It has a much smaller contract:

- discovery and authentication;
- session/model information;
- correlated requests and responses;
- explicit revision and mutation preconditions.

Internal operation names must describe SketchUp-domain work and must not mirror MCP methods such as `tools/call`.

### 3. Threading

SketchUp Ruby API access is main-thread-only.

The Ruby extension therefore has two sides:

1. A background Ruby I/O thread owns the loopback socket, parses/validates framed JSON that does not touch the SketchUp API, and places work onto a thread-safe queue.
2. A repeating `UI.start_timer`, created from the SketchUp main thread during extension startup, drains a bounded number of queued requests. Only this dispatcher and code it calls may access `Sketchup::*`, `UI`, models, or entities.

The I/O thread must never call `UI.start_timer` or any other SketchUp API itself. Results are placed onto a response queue for the I/O thread to serialize and write.

Observer callbacks also run as SketchUp callbacks. They may update bridge bookkeeping such as an integer revision and cache invalidation flags, but must not edit the model.

### 4. Session model

One loaded SketchUp extension instance exposes one bridge session:

- `session_id`: random UUID for the lifetime of that SketchUp process/bridge instance;
- `pid`: OS process id;
- one loopback listener on an ephemeral port;
- the current model is reported by `session.info`.

Multiple SketchUp processes publish independent descriptors and can be addressed independently by `session_id`.

If the active model identity changes, the bridge reports the new `model_guid`, resets that model's revision to `0`, and clears mutation-idempotency state associated with the previous model. Callers must re-read `session.info`.

### 5. Discovery

For the Windows demo, every SketchUp bridge writes one descriptor file under:

`%LOCALAPPDATA%\\SketchUpMCP\\sessions\\<session_id>.json`

Descriptor v1:

```json
{
  "bridge": "sketchup-mcp",
  "protocol_version": 1,
  "session_id": "5de12fd5-4c26-4ff8-a51b-f33f50db0837",
  "pid": 12345,
  "host": "127.0.0.1",
  "port": 43127,
  "token": "<base64url 32 random bytes>",
  "started_at": "2026-09-15T07:30:00Z"
}
```

Rules:

- bind the listener to `127.0.0.1` before publishing the descriptor;
- write a temporary file in the same directory and atomically rename it into place;
- rely on the per-user `LOCALAPPDATA` profile ACL for this demo;
- remove the descriptor on clean shutdown;
- discovery treats files as hints only: stale/unparseable descriptors are ignored and a session is usable only after a successful authenticated handshake;
- the token is never logged.

This is intentionally simpler than registry entries, named pipes, a broker process, multicast, or fixed ports.

### 6. Bridge framing, versioning, correlation, timeout, and authentication

#### Transport and framing

Bridge v1 uses a persistent TCP connection to `127.0.0.1`.

Every frame is:

1. a 4-byte unsigned big-endian payload length;
2. exactly that many bytes of UTF-8 JSON.

Constraints:

- protocol version: integer `1`;
- maximum JSON payload: `1,048,576` bytes (1 MiB);
- length `0`, oversized frames, invalid UTF-8, and invalid JSON close the connection;
- maximum 32 outstanding request IDs per connection;
- unknown message types or fields that violate a message schema return `INVALID_REQUEST` when correlation is possible.

#### Handshake

The first application exchange is:

```json
{"type":"hello","protocol_version":1,"client":"sketchup-mcp-go","client_nonce":"<uuid>"}
```

```json
{"type":"hello","protocol_version":1,"session_id":"<uuid>","server_nonce":"<uuid>","max_message_bytes":1048576}
```

Then the Go host must authenticate before any request:

```json
{"type":"authenticate","protocol_version":1,"token":"<descriptor token>"}
```

```json
{"type":"authenticate","protocol_version":1,"ok":true}
```

On a bad token or unsupported protocol version the bridge sends no model data and closes the connection. The token is scoped to one bridge `session_id` and is regenerated on every bridge start.

TLS is not used for loopback in this demo.

#### Correlated requests

Normal requests have a unique correlation id:

```json
{
  "type": "request",
  "protocol_version": 1,
  "id": "7e2078c8-46ea-45db-af67-e227ea87b60e",
  "session_id": "<uuid>",
  "operation": "model.summary",
  "timeout_ms": 30000,
  "payload": {}
}
```

A success response is:

```json
{
  "type": "response",
  "protocol_version": 1,
  "id": "7e2078c8-46ea-45db-af67-e227ea87b60e",
  "ok": true,
  "payload": {}
}
```

An error response is:

```json
{
  "type": "response",
  "protocol_version": 1,
  "id": "7e2078c8-46ea-45db-af67-e227ea87b60e",
  "ok": false,
  "error": {
    "code": "STALE_REVISION",
    "message": "model revision changed",
    "details": {"expected_revision": 41, "actual_revision": 42}
  }
}
```

Required v1 error codes are:

`INVALID_REQUEST`, `AUTH_FAILED`, `UNSUPPORTED_PROTOCOL`, `SESSION_NOT_FOUND`, `MODEL_CHANGED`, `STALE_REVISION`, `ENTITY_NOT_FOUND`, `ENTITY_NOT_ADDRESSABLE`, `OPERATION_ID_REUSE`, `OPERATION_IN_PROGRESS`, `TIMEOUT`, and `INTERNAL`.

`timeout_ms` is relative to arrival at the Ruby bridge and is clamped to `100..30000` ms. A queued request whose timeout elapses before main-thread execution starts is discarded with `TIMEOUT`. Once a mutation has started, it is never asynchronously interrupted: it commits or aborts normally. If the Go side times out first, a retry of a mutation must reuse the same `operation_id`.

Closing a connection cancels its queued, not-yet-started requests. It does not interrupt a mutation already executing on the main thread.

No unsolicited `event` message is required for v1. The type is reserved; state freshness is obtained from `session.info` and revisions.

### 7. Entity identity

An addressable entity reference is:

```json
{
  "model_guid": "<Sketchup::Model#guid>",
  "persistent_id": 12345,
  "revision": 42
}
```

The durable identity key is `model_guid + persistent_id`. `revision` records the model state in which the reference was observed.

Resolution uses `Model#find_entity_by_persistent_id` on the main thread. Before resolving, the bridge verifies that `model_guid` still matches the active model.

Only entity types documented as supporting `Entity#persistent_id` are addressable. For an unsupported type, the bridge may return descriptive data but must not manufacture a durable reference. A mutation targeting such an object fails with `ENTITY_NOT_ADDRESSABLE`.

`entityID`, Ruby object ids, array indices, names, and object addresses are not fallback identities.

### 8. Revision semantics

Revision is an in-memory monotonically increasing unsigned integer scoped to the current `session_id + model_guid`.

- initialize to `0` when the bridge begins tracking a model;
- increment by exactly one for every `ModelObserver#onTransactionCommit`;
- increment by exactly one for every `onTransactionUndo`;
- increment by exactly one for every `onTransactionRedo`;
- do not increment for `onTransactionEmpty` or `onTransactionAbort`;
- reset to `0` when `model_guid` changes.

SketchUp may internally split some API work into more than one transaction; each observed non-empty commit is a real revision increment. Therefore a single higher-level bridge mutation can advance the revision by more than one. The mutation response reports the revision observed after `commit_operation` returns and its queued transaction callbacks have fired.

Observer callbacks do not mutate the model.

### 9. Mutation semantics

Every mutation request carries this envelope in addition to operation-specific payload:

```json
{
  "mutation": {
    "operation_id": "460adf6d-8b18-4c65-a6c0-f16e42975c4c",
    "expected_model_guid": "<guid>",
    "expected_revision": 42,
    "undo_label": "SketchUp MCP: <action>"
  }
}
```

All preconditions are checked on the main thread immediately before the operation begins.

Order:

1. verify `session_id` and current `model_guid`;
2. verify `expected_revision == current_revision`;
3. check `operation_id`;
4. call `model.start_operation(undo_label, true)`;
5. perform the operation-specific API calls;
6. call `model.commit_operation`;
7. read the post-commit revision and return it.

On any exception after a successful `start_operation`, abort the operation from the same main-thread call path and return an error. Bridge mutations do not use transparent operations.

Idempotency is scoped to one bridge session. The bridge retains every completed mutation's `operation_id`, request fingerprint, and response for that session lifetime:

- same id + same fingerprint: return the cached response without reapplying the mutation;
- same id while the first request is running: join/wait for the same result, or return `OPERATION_IN_PROGRESS` without executing again;
- same id + different fingerprint: return `OPERATION_ID_REUSE`.

The demo intentionally keeps this cache in memory for the process lifetime. A production eviction/persistence policy is a later concern. A caller must never replay an operation id across a different `session_id`.

### 10. Logging

MCP stdout is protocol-only. No banner, diagnostic, progress line, or accidental `fmt.Print*` may be written there.

- Go diagnostics: stderr.
- Ruby diagnostics: a bridge log file under `%LOCALAPPDATA%\\SketchUpMCP\\logs`; no unconditional `puts`/console noise in a packaged extension.
- Secrets and descriptor tokens are always redacted.
- Request payloads that may contain user/model data are not logged by default.

### 11. Threat model

The demo assumes:

- Go MCP host and SketchUp run as the same local Windows user;
- the bridge binds only to `127.0.0.1`;
- the discovery directory is under that user's profile;
- a random 256-bit session token is required before model/session data is exposed;
- the token protects against accidental or lower-privilege local callers but does not defend a fully compromised same-user account or administrator;
- no bridge listener is exposed to LAN/WAN;
- no arbitrary Ruby source, `eval`, shell command, file write, or generic reflection endpoint is accepted over the bridge.

Explicit non-goals: cloud service, OAuth, remote MCP, ChatGPT/Claude Desktop SDK integration, C/C++ extension code, cross-user security isolation, encryption on loopback, and internet-facing operation.

### 12. First demo target

The first supported validation target is:

- Windows 11 x64;
- SketchUp Desktop 2026 for Windows, major version `26.x` (`>=26.0,<27.0`), 64-bit.

Issue #5 smoke validation uses SketchUp `26.0.429`. That build is the first verified build, not a minimum-version requirement. Other `26.x` maintenance releases are within the intended compatibility line; a future `27.x` release requires separate validation before being claimed as supported.

## Session information message

After authentication the Go host can request a fresh session snapshot:

```json
{
  "type": "request",
  "protocol_version": 1,
  "id": "<uuid>",
  "session_id": "<uuid>",
  "operation": "session.info",
  "timeout_ms": 5000,
  "payload": {}
}
```

The response payload is:

```json
{
  "session_id": "<uuid>",
  "pid": 12345,
  "sketchup_version": "26.1.256",
  "model": {
    "guid": "<guid>",
    "title": "Example",
    "revision": 42
  }
}
```

No product operation beyond this protocol envelope is standardized by this ADR. Later issues define the smallest concrete model/query/mutation operations required by the demo.

## Consequences

- Go and Ruby implementations can be developed independently against bridge protocol v1 once this ADR is merged.
- MCP remains a clean public boundary and does not leak into the SketchUp extension.
- Model mutation stays undoable and serialized on SketchUp's main thread.
- Explicit `session_id`, `model_guid`, and revision checks make stale multi-process/model state visible instead of silently acting on the wrong model.
- The protocol is intentionally local and demo-sized. Remote security, durable idempotency, streaming, events, and richer discovery require a new ADR.
