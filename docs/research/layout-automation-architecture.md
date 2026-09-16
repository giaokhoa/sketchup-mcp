# LayOut automation architecture research

Verified: 2026-09-16

Issue: #23

This note intentionally re-evaluates the implementation before more LayOut code
is added.

## Repositories studied

### 1. Vaalasar/SketchUp-SDK-2024

This repository mirrors the official SketchUp Desktop SDK samples.

Relevant samples:

- `GenerateLayOutFromSkp`
- `LayOutExporter`
- `RubyExampleCreateLayOut`
- `WritingToALayOutFile`

Observed patterns:

- standalone C API programs call `LOInitialize()` once and `LOTerminate()`
  once;
- created references are explicitly released;
- `GenerateLayOutFromSkp` creates a `LOSketchUpModelRef` directly from a
  saved .skp, selects existing scenes, adds viewports to a LayOut document, and
  saves the .layout file;
- `LayOutExporter` is a separate CLI that opens an existing .layout and exports
  PDF/PNG/JPG;
- the Windows standalone project links `SketchUpAPI.lib` and `LayOutAPI.lib`
  and copies several LayOut runtime DLLs beside the executable;
- `RubyExampleCreateLayOut` demonstrates the most important documentation
  behavior for this project: a dimension is attached to a SketchUp viewport
  through `LOConnectionPointCreateFromPID` and
  `LOLinearDimensionConnectTo`, rather than by writing custom dimension text.

### 2. jhhsia/sketchup_converter

This is a standalone SketchUp C API consumer rather than a LayOut generator.

Useful architecture patterns:

- the Desktop SDK is treated as a build dependency of a small native
  executable;
- the process calls `SUInitialize()`, loads a saved model from disk with
  `SUModelCreateFromFile`, checks API return codes, traverses model data, and
  releases API objects;
- resource ownership is handled explicitly; for example SUString is wrapped in
  a small RAII class;
- the application works from .skp files without controlling the SketchUp GUI.

This confirms that an offline native worker is feasible, but it also confirms
that it adds a real native toolchain/runtime-distribution boundary.

## Current official documentation

The current SketchUp documentation states:

- the LayOut Ruby API is only available from inside SketchUp;
- `Layout::SketchUpModel` can reference a saved .skp, use saved scenes or
  standard Top/Front/Right/Iso views, use orthographic scale, and render
  Vector/Hybrid/Raster;
- `Layout::ConnectionPoint.new(sketchup_model, point3d, pid)` creates a deep
  connection into SketchUp geometry;
- `Layout::LinearDimension#connect` connects a real dimension to those
  connection points;
- with `custom_text = false`, the dimension displays the measured length and
  updates automatically;
- `Layout::Document#export` exports both PDF and PNG directly.

The current LayOut C API is also standalone-capable, but on Windows it requires
SDK headers/import libraries at build time and multiple LayOut runtime DLLs at
release time.

## State of the Windows test machine

SketchUp 2026 installs runtime DLLs including:

- `LayOutAPI.dll`
- `pdflib.dll`

but the machine does not contain the SDK headers or `LayOutAPI.lib`.

Therefore a new standalone native worker cannot be built correctly from the
installed application alone. The official Desktop SDK would need to be added as
a build dependency first.

## Minimal implementation decision

For #23, do **not** introduce a standalone C++ worker yet.

Use the built-in LayOut Ruby API from the existing SketchUp bridge, because:

1. SketchUp is already open for the MCP modeling workflow.
2. No second native toolchain or SDK packaging is required.
3. The Ruby API already exposes the required viewport, scene, scale, deep
   connection, dimension, .layout save, PDF export, and PNG export functions.
4. It is the smallest change that can produce technically correct associative
   dimensions.
5. The generated .layout can still be opened later in the separate LayOut.exe
   for human editing; LayOut.exe does not need to be automated.

A standalone C API worker remains a later packaging/decoupling option, not a
prerequisite for the first correct sheet.

## Minimal #23 scope

Keep one MCP tool:

`layout.a3_sheet.create`

Internally it should do only:

1. preflight a saved/current SketchUp model;
2. create or reuse the two required section scenes;
3. save a documentation .skp snapshot;
4. create one A3 landscape `Layout::Document`;
5. create six `Layout::SketchUpModel` viewports;
6. create dimensions from geometry-derived model-space points;
7. deep-connect dimension endpoints with persistent IDs wherever possible;
8. never use fixed measurement text;
9. save .layout;
10. export PDF and PNG;
11. return the three output paths plus simple counts.

## What must be removed from the current prototype

- hard-coded dimension strings such as `"1800"`, `"438.5"`, `"2"`,
  `"12"`, and `"18"`;
- paper-space dimensions that are not connected to model geometry;
- PDF QA performed through screenshots of a browser/PDF viewer;
- repeated SketchUp force-kill/restart as part of normal generation.

A3 page/grid coordinates may remain fixed because they are presentation
template data, not model geometry.

## QA

The same LayOut document should export both:

- final PDF;
- a direct 150-300 dpi PNG of the same page.

Automated acceptance should verify:

- A3 landscape page;
- six viewports;
- expected view/scene assignments and scales;
- no custom measurement text on model dimensions;
- all required dimensions are connected;
- PDF and PNG exist.

Visual review should use the directly exported PNG, not a screenshot of Edge or
LayOut.
