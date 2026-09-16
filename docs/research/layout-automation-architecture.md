# LayOut automation architecture

Verified: **2026-09-16**

Issue: #23

## Decision

The documentation pipeline is:

```text
live SketchUp model
  -> documentation.snapshot.create
  -> saved .skp snapshot + drawing spec
  -> standalone layout-worker.exe
  -> LayOut C API runtime
  -> editable .layout + PDF + PNG + QA JSON
```

LayOut is downstream from the SketchUp model. The worker never edits model
geometry and does not automate either SketchUp.exe or LayOut.exe.

## Why not the LayOut Ruby API

The LayOut Ruby API is useful, but Trimble documents it as available only from
inside SketchUp. The first prototype proved it could create a .layout and PDF,
but it forced the documentation renderer to live inside the modeling process.

That architecture caused the wrong lifecycle:

- SketchUp had to remain open for paper-space rendering;
- reloading documentation code encouraged SketchUp restarts;
- recovery prompts appeared after forced restarts;
- PDF generation and model editing shared one process;
- early dimension code accidentally hard-coded measurement text.

The production implementation therefore keeps Ruby on the model side only.

## Standalone C API proof on SketchUp 2026

The Windows test machine contains the LayOut 2026 runtime at:

```text
C:\Program Files\SketchUp\SketchUp 2026\SketchUp\
```

It does not contain SDK headers/import libraries. To avoid requiring a C++
toolchain on the user machine, the worker is a Go executable which dynamically
loads the documented C ABI from `LayOutAPI.dll`.

A live proof was executed with SketchUp and LayOut GUIs closed:

- `LOInitialize` succeeded;
- `LOGetAPIVersion` returned **11.0**;
- `LOSketchUpModelCreate` loaded the saved cabinet .skp;
- `LODocumentCreateEmpty` succeeded;
- A3 page width/height setters succeeded;
- the .skp viewport was added to the document;
- `LODocumentSaveToFile` produced a real .layout;
- `LODocumentExportToPDF` produced a real PDF.

This proves the standalone process boundary against the installed 2026 runtime,
not merely against headers or a mocked API.

## Build/runtime boundary

The implementation follows the public LayOut C API definitions and official SDK
sample patterns. The available public SDK mirror was used to inspect headers and
samples such as:

- `GenerateLayOutFromSkp`;
- `LayOutExporter`;
- `RubyExampleCreateLayOut`;
- `WritingToALayOutFile`.

The worker does not redistribute SDK headers or libraries. It resolves the
installed runtime DLL dynamically and checks the runtime API version before
generation.

## Model-side responsibility

The SketchUp bridge operation is intentionally bridge-only:

```text
documentation.snapshot.create
```

It is not exposed as a separate MCP tool. It performs only model/documentation
preparation:

1. require a saved source model;
2. locate the root structured assembly;
3. recursively extract named Group/ComponentInstance leaves;
4. compute world-space bounds from the actual hierarchy;
5. obtain `Sketchup::InstancePath#persistent_id_path` for dimension targets;
6. derive section positions from actual drawer/shelf geometry;
7. create the six saved scenes;
8. save a versioned .skp snapshot;
9. return a JSON drawing specification.

No paper-space objects are created inside SketchUp.

## Worker responsibility

`layout-worker.exe` receives only a saved .skp plus the drawing specification.

It:

1. initializes the standalone LayOut C API;
2. checks the runtime API version;
3. creates an A3 landscape document;
4. adds the fixed presentation frame/grid;
5. creates six SketchUp viewports from the saved scene indexes;
6. applies orthographic scales from the drawing spec;
7. creates real linear dimensions from model-space endpoints;
8. deep-connects both endpoints with
   `LOConnectionPointCreateFromPID`;
9. uses decimal-millimeter dimension style with units suppressed;
10. saves the editable .layout;
11. exports PDF;
12. exports PNG directly from the same LayOut document;
13. writes a machine-readable QA JSON report.

A3 page/grid coordinates are presentation-template data and may be fixed.
Furniture dimensions, split locations, gap sizes, panel thicknesses and section
locations must come from model geometry.

## Associative dimensions

The official `RubyExampleCreateLayOut` sample demonstrates the required
pattern:

```text
model-space point
  -> LOSketchUpModelConvertModelPointToPaperPoint
  -> LOLinearDimensionCreate
  -> LOConnectionPointCreateFromPID
  -> LOLinearDimensionConnectTo
```

The implementation follows that pattern. Dimension text is not supplied as a
hard-coded measurement string.

## QA gates

A generated sheet is not accepted merely because files exist.

The worker's structural gate requires:

- LayOut runtime API at the validated version boundary;
- A3 landscape page;
- six viewports;
- one or more PID-connected dimensions;
- non-empty .layout;
- non-empty PDF;
- non-empty direct PNG export.

The MCP tool returns an error if this QA report is not passing.

After that automated gate, acceptance for #23 additionally requires a visual
review of the direct PNG export against the user-supplied reference layout.
That review must check viewport composition, dimension collisions, labels and
readability before the PR is merged.

## Recovery behavior

The repeatable baseline is the explicitly saved source .skp. If SketchUp offers
to open an autosaved/recovered version during testing, choose **No**. Recovery
state is never used as input to the documentation pipeline.
