# SketchUp MCP demo

Verified: **2026-09-16**

This is the reproducible demo guide for issue #8. It covers the seven-tool MCP
surface, real SketchUp smoke results, client configuration, and the exact
validation status. A fake bridge is never treated as evidence for real SketchUp,
and a missing MCP client is never marked as a pass.

## Verified environment and research gate

| Item | Version / status |
| --- | --- |
| Windows target | Windows 11 Enterprise, build 26200, x64 |
| SketchUp Desktop | 26.0.429 (SketchUp 2026), verified live |
| sketchup-mcp | 0.1.0-dev |
| Go toolchain | go1.25.0; automated Go suite verified on Linux arm64 |
| MCP Go SDK | github.com/modelcontextprotocol/go-sdk v1.8.0 |
| MCP protocol | current SDK line supports 2026-07-28 plus earlier supported revisions |
| Codex on Windows smoke host | **Not installed / unavailable; real Codex call not claimed** |
| Claude Desktop on Windows smoke host | **Not installed / unavailable; real Claude call not claimed** |

Research was re-checked immediately before integration against:

- OpenAI Codex MCP docs: https://developers.openai.com/codex/mcp/
- MCP Go SDK v1.8.0: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0
- Anthropic local MCP guidance: https://support.claude.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop
- MCP SDK real-host guide: https://py.sdk.modelcontextprotocol.io/get-started/real-host/

The Windows smoke machine had a running SketchUp process and live bridge, but
did not have codex, claude, go, or git in PATH. Real SketchUp scenarios below
were therefore driven through the authenticated local bridge. Automated tests
cover MCP stdio discovery and the full MCP -> session registry -> fake bridge
pipeline. Real Codex/Claude client validation remains an external-environment
gate and is not fabricated here.

## Prerequisites

- Windows 11 x64.
- SketchUp Desktop 2026 26.x; build 26.0.429 is the verified target.
- Python 3 to package the RBZ.
- Go 1.25.x to build the MCP host.
- A local checkout of this repository.

## Build and install

From the repository root:

~~~powershell
python scripts/package_rbz.py
go build -o build\sketchup-mcp.exe .\cmd\sketchup-mcp
~~~

The RBZ is written to dist\giaokhoa_sketchup_mcp.rbz.

In SketchUp, open **Extension Manager > Install Extension**, select that RBZ,
then open a model. A healthy extension writes one per-process descriptor under:

~~~text
%LOCALAPPDATA%\SketchUpMCP\sessions\
~~~

Descriptors contain an ephemeral loopback endpoint and a per-start secret.
Never copy the token into logs, prompts, issues, or MCP results.

## Final MCP surface

The server intentionally exposes only:

~~~text
sketchup.sessions.list
model.summary
selection.get
entity.inspect
entity.translate
geometry.create_box
changes.undo
~~~

Use sketchup.sessions.list first. Before every intended write, read current
model state and use its model GUID/revision. Use durable entity references
returned by the read/write tools. Give each intended write a unique
operation_id. After STALE_REVISION, re-read state and retry with a new operation
ID.

## Codex configuration

Current Codex supports local STDIO MCP servers and reads the server instructions
field. ChatGPT desktop, Codex CLI, and the Codex IDE extension share MCP
configuration on the same Codex host.

Fast setup:

~~~powershell
codex mcp add sketchup -- C:\absolute\path\to\build\sketchup-mcp.exe
codex mcp list
~~~

Equivalent ~/.codex/config.toml entry:

~~~toml
[mcp_servers.sketchup]
command = "C:\\absolute\\path\\to\\build\\sketchup-mcp.exe"
startup_timeout_sec = 10
tool_timeout_sec = 60
enabled = true
required = true
default_tools_approval_mode = "writes"
~~~

In Codex TUI use /mcp. In the ChatGPT desktop app use
**Settings > MCP servers** and restart after saving. The writes approval mode
prompts for tools that are not annotated read-only.

**Validation status on 2026-09-16:** configuration was re-checked against
current official OpenAI docs, but no Codex binary/client was present on the
target Windows machine, so no real Codex tool call is claimed.

## Claude Desktop configuration

Anthropic currently recommends Desktop Extensions/MCPB for packaged local MCP
servers. This repository does **not** add an MCPB/DXT wrapper merely to satisfy
this demo: the existing binary is already a valid local stdio MCP server, and
issue #8 explicitly rejects workaround layers that compromise the architecture.

For local developer MCP, the current MCP real-host documentation supports an
absolute stdio command in:

~~~text
%APPDATA%\Claude\claude_desktop_config.json
~~~

Example:

~~~json
{
  "mcpServers": {
    "sketchup": {
      "command": "C:\\absolute\\path\\to\\build\\sketchup-mcp.exe",
      "args": []
    }
  }
}
~~~

Fully quit Claude Desktop and reopen it after editing. Claude Desktop server
logs are under %APPDATA%\Claude\logs.

**Validation status on 2026-09-16:** Claude Desktop was not installed/available
on the target machine, so no real Claude tool call is claimed.

## Copy/paste demo prompts

### A - inspect an existing model

~~~text
List live SketchUp sessions. Choose the session with my open model, read its
model summary and current selection, then inspect the selected group or
component using the durable entity reference. Do not modify the model.
~~~

Expected: one healthy session, bounded model/selection output, then entity
details for the selected supported entity.

### B - live edit and undo

~~~text
Read the current model revision and selected entity reference. Translate the
selected group/component exactly 1 inch along +X using a fresh operation ID.
Verify exactly one revision advance, inspect the result, then undo using the
new current revision and another fresh operation ID. Verify placement is back
to the original bounds.
~~~

Expected: one visible translation, one revision advance for the mutation,
another revision advance for undo, and original placement restored.

### C - create a primitive and undo

~~~text
Read the current model revision. Create one grouped box at [100,100,0] inches
with dimensions 2 x 3 x 4 inches using a fresh operation ID. Inspect the
returned durable reference, then undo with the current revision and a new
operation ID. Verify the box is gone.
~~~

Expected: exactly one box, durable persistent ID, then ENTITY_NOT_FOUND for that
ID after undo.

### D - mutation safety

Repeat the exact same geometry.create_box request with the same operation_id:
the cached result must return without a duplicate model change. Then submit a
different write with an older revision and a fresh operation ID: expect
STALE_REVISION and no model change.

### E - responsiveness

Run repeated bounded reads plus several write/undo pairs, disconnect and
reconnect the client, and confirm SketchUp remains responsive. No bridge socket
I/O should block SketchUp's UI thread.

## Real SketchUp result - 2026-09-16

Target: Windows 11 Enterprise build 26200, SketchUp 26.0.429.

- Scenario A: PASS through the live authenticated bridge. The open model
  returned revision 8; selection contained one ComponentInstance with a durable
  persistent ID.
- Scenario B: PASS. A +1 inch X translation advanced revision 8 -> 9; undo
  advanced 9 -> 10 and restored the original bounds.
- Scenario C: PASS. A 2 x 3 x 4 inch box returned a durable reference; undo
  removed it and later inspection returned ENTITY_NOT_FOUND.
- Scenario D: PASS. Replaying the same operation ID produced no duplicate
  mutation; a fresh write with an old revision returned STALE_REVISION without
  changing model state.
- Scenario E: PASS. 100 repeated model.summary reads plus disconnect/reconnect
  completed and SketchUp still reported Responding=True.

The box and translation were undone. Revision ended at 12 because successful
undo operations themselves advance the tracked revision; top-level model
content was restored. No bridge token was recorded.

These results prove the real SketchUp bridge/model behavior. They do **not**
claim that Codex or Claude Desktop drove the calls on this machine.

## Automated integration gate

Run:

~~~powershell
go test ./...
go vet ./...
go test -race ./...
~~~

The Go suite covers:

- exact seven-tool MCP discovery, descriptions, generated input/output schemas,
  annotations, and server instructions;
- stdio protocol cleanliness;
- structured domain-error mapping;
- authenticated MCP -> session registry -> fake bridge command flow;
- bridge authentication/protocol failures and disconnect/reconnect;
- stale session removal;
- mutation request validation and idempotency;
- race-free session discovery.

On 2026-09-16 all three Go commands passed with Go 1.25.0.

Ruby/SketchUp regression coverage remains under test/ and docs/testing/. This
issue changes no Ruby runtime code.

## Troubleshooting and useful logs

- **No sessions:** ensure SketchUp has a model open, the extension is enabled,
  and a descriptor exists under %LOCALAPPDATA%\SketchUpMCP\sessions\.
- **SESSION_NOT_FOUND:** call sketchup.sessions.list again and use the current
  session ID.
- **STALE_REVISION:** re-read model.summary/selection, refresh durable references
  as needed, and use a new operation ID for the intended retry.
- **Client sees no tools:** use an absolute executable path, restart the MCP
  client, and confirm the server writes diagnostics only to stderr.
- **Codex:** run codex mcp list or use /mcp.
- **Claude Desktop:** inspect %APPDATA%\Claude\logs\mcp.log and the per-server
  MCP log.
- **SketchUp:** use the Ruby Console only for diagnostics; do not invoke bridge
  internals from a background thread.

When sharing logs, include client/server versions and error codes, but redact
descriptor tokens.

## Cleanup / uninstall

1. Remove or disable the sketchup MCP entry in the client.
2. Fully quit the MCP client.
3. Disable/uninstall **SketchUp MCP** in SketchUp Extension Manager.
4. Close SketchUp. If stale descriptor files remain after the process exits,
   remove them from %LOCALAPPDATA%\SketchUpMCP\sessions\.
5. Delete local build\sketchup-mcp.exe and generated RBZ files if no longer
   needed.
