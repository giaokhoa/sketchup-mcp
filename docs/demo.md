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
| Local MCP tools | 13 |
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
entity.name.set
entity.children.list
assembly.create
geometry.create_box
layout.a3_sheet.create
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

- exactly thirteen MCP tools discovered;
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

## Structured cabinet naming smoke - issue #19

The practical cabinet was rebuilt in a fresh SketchUp instance with the
packaged #19 artifact as **34 independently editable Groups**. The parts include
individual carcass boards, shelves, four legs, four doors, six drawer-box boards
per drawer, and six handles.

Every Group was named through `entity.name.set`. Examples include:

```text
Carcass - Side Left
Carcass - Center Divider
Door - 01
Drawer Left - Side Left
Drawer Left - Bottom
Leg - Front Right
Handle - Door 04
```

Live mutation safety checks passed before the final model was built:

- exact rename replay did not advance revision twice;
- `changes.undo` restored the previous Group name;
- a rename using the pre-undo revision returned `STALE_REVISION`.

Final readback:

- `entity.inspect`: 34/34 expected names matched;
- `selection.get` after selecting the cabinet: 34 selected, 34 returned,
  34 unique non-empty Group names;
- overall cabinet width: **1800.000 mm**;
- SketchUp remained responsive.

The names are instance/group names intended for practical Outliner navigation;
the MCP does not rename shared ComponentDefinition objects.

## Nested furniture assembly smoke - issue #21

The named 34-part cabinet was rebuilt in a fresh SketchUp 2026 instance using
the packaged #21 artifact and then converted from a flat model into a real
assembly hierarchy:

```text
Cabinet - 1800
├─ Carcass
│  └─ 8 named structural boards
├─ Legs
│  └─ 4 named legs
├─ Door Assembly - 01
│  ├─ Door - 01
│  └─ Handle - Door 01
├─ Door Assembly - 02
├─ Door Assembly - 03
├─ Door Assembly - 04
├─ Drawer Assembly - Left
│  └─ 6 drawer boards + drawer handle
└─ Drawer Assembly - Right
   └─ 6 drawer boards + drawer handle
```

Live MCP validation:

- exactly 12 tools discovered;
- an assembly replay returned the cached result without regrouping;
- `changes.undo` restored two probe children from an assembly;
- a stale assembly request returned `STALE_REVISION`;
- final model has exactly one top-level Group named `Cabinet - 1800`;
- `entity.children.list` returns 8 immediate root subassemblies;
- recursive child listing returns all original 34 named parts;
- every original part persistent ID remains addressable after two nesting levels;
- all original names and materials remain attached to the original parts;
- overall cabinet width remains **1800.000 mm**;
- SketchUp remained responsive.

### World-position validation

A nested entity's `bounds` are expressed in its current parent context, so
comparing raw child bounds before and after regrouping is not a valid world-space
movement test.

The live acceptance therefore inspected the transformation at all three levels
and composed:

```text
Cabinet transform
  + Subassembly transform
  + Part transform
  = original part world translation
```

All 34 composed world translations matched their pre-grouping millimeter
origins within floating-point tolerance. SketchUp chose a non-zero transform for
the root assembly, but compensated the child transforms so the physical model
did not move.

## A3 LayOut shop-drawing smoke - issue #23

The saved 1800 mm cabinet model was documented through one MCP call:

```text
layout.a3_sheet.create
```

The tool does not automate LayOut.exe. It uses the LayOut Ruby API available
inside the existing SketchUp extension and produces:

```text
documentation .skp snapshot
editable .layout
PDF
direct PNG
```

The drawing spec is model-derived. Furniture measurements are not supplied as
custom dimension text. Every generated dimension uses real model-space
endpoints and persistent-id paths, then connects to the matching
`Layout::SketchUpModel` viewport with `Layout::ConnectionPoint`.

Live packaged-artifact acceptance on SketchUp 2026:

- exactly 13 MCP tools discovered;
- one A3 landscape page: **420 x 297 mm**;
- 6 viewports:
  - MẶT BẰNG;
  - MẶT ĐỨNG CHÍNH;
  - MẶT CẮT A-A;
  - MẶT CẮT B-B;
  - MẶT BÊN;
  - PHỐI CẢNH;
- **46 dimensions created / 46 dimensions connected**;
- no generated dimension uses custom measurement text;
- preflight rejects dimension endpoints outside their referenced part bounds;
- preflight rejects non-axis-aligned shop-drawing dimensions;
- A3 paper preflight rejects overlapping/out-of-page viewports and labels;
- PDF semantic readback reported the expected model measurements:
  - width 1800.0 mm;
  - 900.0 / 900.0 main split;
  - four 438.5 mm lower doors;
  - three 2.0 mm door gaps;
  - 12.0 mm drawer-box boards;
  - 18.0 mm adjustable shelf;
  - 220.0 mm shelf level;
- the output PDF contains one page and all six expected view labels;
- the MCP result returns structured output and one native `image/png`
  `ImageContent` block in the same call;
- live native preview payload size was **258,874 bytes**;
- SketchUp remained responsive.

The native preview is the intended review transport for a direct MCP client.
The server reads the LayOut-exported PNG and returns raw image bytes as standard
MCP `ImageContent`; callers do not need to parse Base64 or open a PDF viewer.

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
