# LayOut automation architecture

Verified: **2026-09-16**

Issue: #23

## Repositories studied

### Vaalasar/SketchUp-SDK-2024

Relevant official SDK samples:

- `GenerateLayOutFromSkp`
- `LayOutExporter`
- `RubyExampleCreateLayOut`
- `WritingToALayOutFile`

Key findings:

- standalone LayOut C API applications are valid, but they require the Desktop
  SDK build boundary and multiple LayOut runtime libraries;
- `RubyExampleCreateLayOut` demonstrates the important dimension pattern:
  model-space point + persistent-id path -> connection point -> connected linear
  dimension;
- dimension text should be measured by LayOut, not supplied as a fixed string;
- PDF and image export are first-class LayOut operations.

### jhhsia/sketchup_converter

This repository is a standalone SketchUp C API consumer.

Useful pattern:

- treat the Desktop SDK as a real native build dependency;
- initialize and terminate the API explicitly;
- load saved .skp files from disk;
- check API return codes;
- manage native object lifetime explicitly.

It confirms a standalone worker is feasible, but also confirms that it adds a
native toolchain/runtime packaging boundary.

## Current official Ruby API

The current LayOut Ruby API already exposes everything needed for the first
correct documentation tool:

- `Layout::SketchUpModel` for saved .skp viewports;
- scene selection and orthographic scale;
- `model_to_paper_point`;
- `Layout::ConnectionPoint.new(viewport, point3d, persistent_id_path)`;
- `Layout::LinearDimension#connect`;
- `custom_text = false` for measured, automatically updating dimensions;
- `Layout::Document#save`;
- direct PDF export;
- direct PNG export.

The LayOut Ruby API runs inside SketchUp.

## Minimal decision for #23

Do **not** add a standalone native worker in this PR.

Use the existing SketchUp MCP bridge and one tool:

`layout.a3_sheet.create`

Pipeline:

```text
saved live SketchUp model
  -> inspect real hierarchy and world bounds
  -> derive section positions and dimension endpoints
  -> create six documentation scenes
  -> save documentation .skp snapshot
  -> create A3 LayOut document through the built-in Ruby API
  -> create six SketchUp viewports
  -> create PID-connected measured dimensions
  -> save editable .layout
  -> export PDF
  -> export PNG directly
  -> visual review of that PNG
```

This is the smallest implementation because SketchUp is already open for the MCP
modeling workflow and no additional SDK/compiler/runtime distribution is
required.

LayOut.exe is not automated. It remains the optional human editor for the
resulting .layout file.

## Model-derived data versus template data

Allowed fixed values:

- A3 paper size;
- page margins;
- six viewport rectangles;
- line weights;
- fonts;
- fixed presentation grid.

Must come from the model:

- overall dimensions;
- door/drawer subdivision points;
- gaps;
- panel thicknesses;
- shelf levels;
- section positions;
- persistent-id paths;
- dimension measurement text.

No furniture measurement is written as custom dimension text.

## Associative dimension pattern

For every dimension:

```text
real model-space start/end points
  -> viewport.model_to_paper_point
  -> Layout::LinearDimension
  -> Layout::ConnectionPoint(viewport, point3d, persistent_id_path)
  -> dimension.connect(start_connection, end_connection)
  -> custom_text = false
```

The connection must succeed for both endpoints. The dimension then displays the
actual measured value from the model.

## QA gate

Generation is not complete merely because files exist.

The tool must return:

- .skp snapshot;
- editable .layout;
- PDF;
- direct PNG;
- 6 viewports;
- dimension count;
- connected dimension count.

Acceptance requires:

- A3 landscape;
- six expected views;
- every generated dimension connected to model geometry;
- no hard-coded measurement text;
- non-empty PDF and PNG;
- visual review of the directly exported PNG against the supplied reference;
- no merge until visual review passes.

Browser/PDF-viewer screenshots are not acceptance evidence.

## MCP preview transport

The final local flow returns the directly exported full-sheet PNG as standard
MCP `ImageContent` in the same `layout.a3_sheet.create` response that carries
the typed structured result.

The Go server reads the generated PNG bytes and returns:

```go
&mcp.ImageContent{Data: previewBytes, MIMEType: "image/png"}
```

This avoids a separate Base64/file-transfer loop for direct MCP clients.

Desktop Commander can also read the generated PDF and returns text plus image
content blocks, but when Desktop Commander is itself called through an
additional orchestration wrapper those image blocks may be flattened into text.
That wrapper limitation is not part of the SketchUp MCP protocol and is not
used as the production preview transport.

## Recovery behavior

The repeatable input is the explicitly saved source .skp. When SketchUp offers a
recovered/autosaved version during testing, choose **No**. Recovery state is not
part of the documentation pipeline.
