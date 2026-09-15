# Local session bridge smoke test

Verified: **2026-09-15**

## Verified environment

- Windows x64, OS build `26200` on the smoke machine.
- SketchUp Desktop 2026 for Windows, build `26.0.429`.
- Compatibility policy for this bridge: SketchUp `26.x` (`>=26.0,<27.0`); `26.0.429` is a verified build, not a minimum requirement.
- Go host built from this repository with the official MCP Go SDK.

## Preconditions

1. Install the extension as the official RBZ layout requires: root `giaokhoa_sketchup_mcp.rb` plus the same-named support directory.
2. Launch SketchUp with a real model. For unattended smoke testing, opening one of SketchUp's shipped `.skp` templates directly bypasses the Welcome window without UI automation.
3. Confirm `%LOCALAPPDATA%\SketchUpMCP\sessions\` contains one descriptor for the running process.
4. Do not copy descriptor tokens into logs, test output, issue comments, or MCP results.

## Single-session handshake

1. Start one SketchUp 26.x process with a model open.
2. Run the Go host/session registry.
3. Confirm the descriptor is parsed from `%LOCALAPPDATA%\SketchUpMCP\sessions`.
4. Confirm Go completes protocol v1 hello + authentication.
5. Confirm `session.info` returns the same internal `session_id`, the SketchUp PID/version, model GUID/title/revision, and loopback endpoint.

Verified result on `26.0.429`: PASS.

## Authentication rejection

Against a real running extension:

- send a valid hello followed by an incorrect token: expect `AUTH_FAILED`, then connection close;
- send hello with an unsupported protocol version: expect connection close before model/session data is returned.

Verified result on `26.0.429`: PASS.

## Multi-instance discovery

1. Start two independent SketchUp processes, each with a model open.
2. Confirm two descriptor files exist.
3. Confirm descriptors have different internal session IDs and different ephemeral loopback ports.
4. Run the Go registry or MCP `sketchup.sessions.list`.
5. Confirm both healthy sessions are returned independently.

Verified result on `26.0.429`: PASS.

## Cleanup

1. Close one SketchUp instance normally.
2. Confirm that process exits and its descriptor is removed.
3. Call `sketchup.sessions.list` again.
4. Confirm only the remaining live session is returned.

Verified result on `26.0.429`: PASS.

## MCP stdio boundary

1. Build `sketchup-mcp.exe`.
2. Connect an MCP client with the official Go SDK `CommandTransport` over stdio.
3. Call `sketchup.sessions.list` with an empty object.
4. Confirm typed structured output contains live sessions and does not expose the per-session token.

Verified result on `26.0.429`: PASS.

## Responsiveness

Run repeated authenticated `session.info` probes while SketchUp is open and confirm the process remains responsive.

Verified run: 100 authenticated probes completed successfully; SketchUp still reported `Responding=True` afterward.

## Automated regression checks

- Ruby protocol tests
- Ruby main-thread dispatcher tests
- Ruby server integration tests
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`

All passed on the verification date above.

