# MCP host bootstrap verification

Verified: **2026-09-15**

Scope: implementation gate for issue #3 only.

## Locked dependency

- Official SDK: `github.com/modelcontextprotocol/go-sdk v1.8.0`.
- v1.8.0 is the latest stable release as of this verification.
- Its `go.mod` declares `go 1.25.0`.
- The SDK supports MCP revisions `2026-07-28`, `2025-11-25`,
  `2025-06-18`, `2025-03-26`, and `2024-11-05`.

Authoritative sources:

- https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/go.mod
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/README.md

## Server and stdio pattern

The v1.8.0 server pattern used here is:

1. construct `mcp.NewServer`;
2. register typed tools with the generic `mcp.AddTool`;
3. call `Server.Run(ctx, &mcp.StdioTransport{})`.

`StdioTransport` owns process stdin/stdout and uses newline-delimited JSON.
Application diagnostics therefore go to stderr. No startup banner or ordinary
log line is written to stdout.

Authoritative sources:

- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/protocol.md
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/transport.go

## Typed tool schemas and structured output

For generic `mcp.AddTool[In, Out]`:

- missing input and output schemas are inferred from the Go types;
- `jsonschema` tags supply field descriptions;
- input arguments are schema-validated before handler invocation;
- the typed output is marshaled to `CallToolResult.StructuredContent`;
- output is validated against the inferred output schema;
- when the handler leaves text content empty, the SDK also provides the JSON
  encoding of the typed output as unstructured content.

This repository therefore does not hand-write JSON Schema or structured-output
maps for `sketchup.sessions.list`.

Authoritative source:

- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/server.md

## Test pattern

v1.8.0 exposes `mcp.NewInMemoryTransports` for in-process client/server tests.
A test client can request a specific supported protocol revision with
`mcp.ClientSessionOptions.ProtocolVersion`, and
`ClientSession.InitializeResult().ProtocolVersion` exposes the negotiated
revision.

The stdio integration test uses the SDK's `mcp.CommandTransport` against a
helper subprocess that executes the same `run` function as the binary. The
helper emits a diagnostic to stderr before serving; successful MCP discovery
and tool invocation prove that stderr diagnostics do not contaminate stdout.

Authoritative sources:

- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/mcp_example_test.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/client.go
- https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/mcp/cmd.go
