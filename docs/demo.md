# SketchUp MCP local demo

Verified: **2026-09-16**

This guide covers the packaged local Windows demo only:

```text
MCP stdio client
  -> sketchup-mcp-windows-amd64.exe
  -> authenticated loopback session bridge
  -> SketchUp Desktop
```

Remote MCP and ChatGPT transport are intentionally outside this local baseline.

## Verified target

| Item | Verified value |
| --- | --- |
| Windows | Windows 11 Enterprise build 26200, x64 |
| SketchUp | SketchUp 2026 / 26.0.429 |
| Go | 1.25.0 |
| MCP Go SDK | github.com/modelcontextprotocol/go-sdk v1.8.0 |
| Local MCP tools | 9 |
| SketchUp responsiveness | PASS |
| Real MCP stdio -> SketchUp | PASS |

## Package from source

Developer prerequisites:

- Go 1.25.x
- Python 3

From the repository root:

```powershell
python scripts/package_demo.py
```

The command runs the Go tests and creates:

```text
dist/
  sketchup-mcp-windows-amd64.exe
  giaokhoa_sketchup_mcp.rbz
  SHA256SUMS.txt
```

The Windows executable is built with cgo disabled, `-trimpath`, and
`-buildvcs=false`. The RBZ is written with deterministic file ordering,
timestamps, modes, and stored ZIP entries.

CI builds the bundle twice and compares all three files byte-for-byte before
uploading the `sketchup-mcp-windows-demo` workflow artifact.

## Verify checksums

On Windows PowerShell:

```powershell
Get-Content .\SHA256SUMS.txt
Get-FileHash .\sketchup-mcp-windows-amd64.exe -Algorithm SHA256
Get-FileHash .\giaokhoa_sketchup_mcp.rbz -Algorithm SHA256
```

The two hashes must match the values in `SHA256SUMS.txt`.

## Install the SketchUp extension

1. Open SketchUp 2026.
2. Open **Extension Manager**.
3. Choose **Install Extension**.
4. Select `giaokhoa_sketchup_mcp.rbz`.
5. Open or create a model.

A healthy extension publishes one ephemeral descriptor per SketchUp process
under:

```text
%LOCALAPPDATA%\SketchUpMCP\sessions\
```

The descriptor contains a loopback endpoint and per-start secret. Do not copy
the token into logs, prompts, issues, or MCP output.

## Configure a local MCP stdio client

Point the client command directly at the packaged executable:

```text
C:\absolute\path\sketchup-mcp-windows-amd64.exe
```

The executable uses stdio for MCP. Protocol output stays on stdout and
diagnostics stay on stderr.

The expected tool list is exactly:

```text
sketchup.sessions.list
model.summary
selection.get
entity.inspect
entity.translate
entity.delete
entity.material.set
geometry.create_box
changes.undo
```

Use `sketchup.sessions.list` first. Before a write, read the current model
GUID/revision, use durable entity references, and give each intended mutation a
fresh `operation_id`. If a write returns `STALE_REVISION`, refresh state and
retry the intended write with a new operation ID.

## Proven local end-to-end scenarios

All scenarios below were executed against real SketchUp 26.0.429 through the
Go MCP stdio server.

### Read/discovery

- exactly nine MCP tools discovered;
- live SketchUp session listed;
- model summary and selection returned bounded structured output;
- durable group/component references inspected successfully.

### Mutation safety

- box creation advanced revision once;
- +1 inch X translation advanced revision once;
- replaying the same operation ID did not mutate twice;
- a fresh write using an old revision returned `STALE_REVISION`;
- undo restored translation;
- undo removed the test box;
- inspection after removal returned `ENTITY_NOT_FOUND`;
- entity deletion replay did not delete a second entity;
- stale entity deletion returned `STALE_REVISION`;
- undo restored a deleted entity;
- material assignment replay did not duplicate a mutation/material;
- stale material assignment returned `STALE_REVISION`;
- undo restored the previous unpainted appearance.

### Responsiveness

- 100 repeated `model.summary` calls completed through MCP;
- MCP disconnect/reconnect succeeded;
- SketchUp remained `Responding=True`.

## Practical furniture smoke test

Issue #9 also used the MCP as an actual modeling tool instead of only exercising
protocol cases.

A sideboard blockout was built through **18 real
`geometry.create_box` MCP calls**:

- cabinet body;
- top slab;
- four legs;
- two drawer fronts;
- four door fronts;
- six handles.

Requested overall width: **1800 mm**.

A subsequent `entity.inspect` measured the top slab as:

```text
70.866141732 inches
1800.000 mm
```

The model revision advanced from 8 to 26 and SketchUp remained responsive. The
cabinet was intentionally left in the open model for visual inspection.

The #9 smoke test exposed two practical gaps: deletion and material assignment.
Issue #16 added only those two capabilities and then repeated the furniture test
on a fresh SketchUp instance using the packaged PR artifact.

### Colored furniture smoke - issue #16

The fresh model's default scale figure was selected, addressed by durable
ComponentInstance reference, deleted through `entity.delete`, replayed without
a second deletion, restored through `changes.undo`, checked against a stale
revision, and then deleted for the final model.

The cabinet was rebuilt as 18 groups at **1800 mm** overall width. Material
assignment was exercised with replay, undo, and stale-revision checks before the
final appearance was applied:

- 12 cabinet parts: `cabinet-light-wood`, RGB **222, 203, 176**;
- 6 handles: `cabinet-bronze`, RGB **146, 94, 65**.

A final MCP selection/inspection pass reported:

- 18 top-level groups;
- 0 ComponentInstances, confirming the scale figure was removed;
- 12 groups using `cabinet-light-wood`;
- 6 groups using `cabinet-bronze`;
- top slab size **1800.0 x 450.0 x 30.0 mm**;
- SketchUp still `Responding=True`.

Do not assert that a fresh SketchUp template contains only two materials:
SketchUp may preload additional template materials. The acceptance check is the
material attached to the target entities, not the total material collection
size.

## Known local-demo limitations

The current surface intentionally does not provide:

- texture/PBR/UV workflows;
- arbitrary Ruby execution;
- remote MCP transport.

Solid RGB material assignment is intentionally bounded; it is not a general
appearance framework.

## Troubleshooting

**No sessions**

Ensure SketchUp has a model open, the extension is enabled, and a descriptor
exists under `%LOCALAPPDATA%\SketchUpMCP\sessions\`.

**SESSION_NOT_FOUND**

Run `sketchup.sessions.list` again and use the current session.

**STALE_REVISION**

Read `model.summary` again, refresh references if needed, and use a new
operation ID for the intended retry.

**Client sees no tools**

Use an absolute executable path and confirm the client launches the executable
as an MCP stdio process.

**SketchUp**

Use Ruby Console only for diagnostics. Do not bypass the MCP bridge to claim an
MCP acceptance pass.

## Cleanup

1. Disconnect/quit the MCP client.
2. Disable or uninstall **SketchUp MCP** in Extension Manager.
3. Close SketchUp.
4. Remove stale files under `%LOCALAPPDATA%\SketchUpMCP\sessions\` only
   after the corresponding SketchUp process has exited.
5. Delete the packaged EXE/RBZ if no longer needed.

The extension never silently saves or overwrites the open model.
