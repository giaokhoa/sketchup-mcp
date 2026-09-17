# SketchUp MCP local demo

Verified: **2026-09-17**

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
| Local MCP tools | 27 |
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

<!-- PUBLIC_TOOL_LIST_START -->
```text
sketchup.sessions.list
model.summary
model.bounds
selection.get
entity.inspect
entity.children.list
entity.translate
entity.delete
entity.material.set
entity.name.set
assembly.create
geometry.create_box
section_plane.list
section_plane.create
scene.list
scene.create
model.file.save_copy
changes.undo
layout.document.create
layout.template.inspect
layout.panel.validate
layout.viewport.add
layout.dimension.add
layout.text.add
layout.line.add
layout.rectangle.add
layout.export
```
<!-- PUBLIC_TOOL_LIST_END -->

Use `sketchup.sessions.list` first. Before a write, read the current model
GUID/revision, use durable entity references, and give each intended mutation a
fresh `operation_id`. If a write returns `STALE_REVISION`, refresh state and
retry the intended write with a new operation ID.

## Canonical SketchUp -> LayOut documentation workflow

Use SketchUp scenes as the presentation source for camera and section state. A
LayOut viewport should reference a named documentation scene when one exists
instead of independently inventing camera state. Orthographic documentation
views require an explicit scale denominator.

```text
sketchup.sessions.list
  -> model.summary / model.bounds
  -> section_plane.list / scene.list
  -> reuse matching presentation state; create only missing state
  -> model.file.save_copy
  -> layout.template.inspect
  -> layout.document.create(template_path)
  -> layout.viewport.add(require_fit=true)
  -> layout.dimension.add
  -> layout.text / layout.line / layout.rectangle as needed
  -> layout.panel.validate for every populated panel
  -> layout.export
```

Paper-space coordinates are millimeters. `fit_model_bounds_mm` is SketchUp
model-space bounds in millimeters, not paper-space bounds. Runtime viewports,
dimensions, and annotations should carry the template `panel_id` so
`layout.panel.validate` can reject containment violations before accepted
export. Associative dimensions report `connected`; accepted dimensions should
return `connected=true`.

For official drawing generation, normally keep `require_fit=true` and fix scale,
slot, or model-bound inputs when the fit check fails rather than disabling the
gate.

### One-command live acceptance

Issue #38 checked in the generic live harness. From a fresh checkout, with the
matching CI EXE/RBZ installed and the target SketchUp model already open:

```powershell
.\scripts\run_live_layout_e2e.ps1 `
  -McpExe C:\acceptance\sketchup-mcp-windows-amd64.exe `
  -Rbz C:\acceptance\giaokhoa_sketchup_mcp.rbz `
  -Pid <SketchUp-PID> `
  -SourceModel C:\fixtures\cabinet-layout-source.skp `
  -Template C:\fixtures\furniture-shopdrawing-a2.layout `
  -OutputDir C:\acceptance\cabinet-live `
  -Fixture .\testdata\e2e\cabinet\spec.json `
  -WorkflowRunId <run-id> `
  -ArtifactId <artifact-id>
```

See `docs/testing/live-layout-e2e.md` for preconditions and report fields. The
final #38 acceptance used CI run `35174845489`, artifact `10477553790`, SketchUp
26.0.429, and passed 52/52 connected dimensions, 6/6 populated panels with zero
containment violations, native PNG `ImageContent` (166,756 bytes), PDF export,
and the SketchUp responsiveness gate.

## Proven local end-to-end scenarios

All scenarios below were executed against real SketchUp 26.0.429 through the
Go MCP stdio server.

### Read/discovery

- current public contract exposes exactly 27 MCP tools;
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

## LayOut template-first acceptance - issues #27, #28, and #38

The packaged MCP exposes SketchUp presentation and LayOut operations as
independent primitives. There is no public tool that decides a cabinet or sheet
workflow. The final live flow uses an A2 landscape reference template and
composes the primitives in the canonical order documented above.

The public coordinate contract is millimeters. Conversion to SketchUp/LayOut
internal inches happens only at the Ruby API boundary. Template inspection
provides reusable panel, viewport-slot, layer, style, and Auto-Text metadata;
`panel_id` is then carried by runtime entities so containment can be validated
before export.

The cabinet remains fixture data only. The generic #38 runner also passes a
second minimal fixture with one viewport and one connected dimension without
source-code changes.

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

**Viewport fit check fails**

Keep `require_fit=true`. Re-read `model.bounds`, confirm `fit_model_bounds_mm` is
model-space millimeter bounds, then adjust the scene, orthographic scale, slot,
or fit margin. Do not disable the fit gate to make an accepted drawing pass.

**Panel containment validation fails**

Run `layout.panel.validate` for the reported `panel_id`, inspect its violations,
and move/resize the associated viewport, dimension, or annotation back inside
the template panel. Re-run validation before export.

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
