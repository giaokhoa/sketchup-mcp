# Live SketchUp → LayOut E2E harness

This harness turns the template-first live flow into a repository-owned acceptance test.
It drives the real packaged MCP executable over stdio and a real SketchUp Desktop session.
It is intentionally not part of normal CI because GitHub-hosted runners do not provide SketchUp Desktop.

## What it proves

The generic runner executes this fixture-driven flow:

```text
discover target session
  -> model.summary / model.bounds
  -> list existing scenes / section planes and create only missing fixture state
  -> run the same presentation preparation a second time and require reuse-only / unchanged revision
  -> model.file.save_copy
  -> layout.template.inspect
  -> layout.document.create(template_path)
  -> layout.viewport.add(require_fit=true)
  -> layout.dimension.add
  -> annotations / marks / notes
  -> layout.panel.validate for every populated panel
  -> layout.export PNG + native ImageContent validation
  -> layout.export PDF
  -> SketchUp responsiveness check
```

Cabinet coordinates, dimensions, captions, scene choices, panel IDs, and notes live in
`testdata/e2e/cabinet/spec.json`; the runner contains no cabinet workflow.

## Preconditions

1. Use Windows with SketchUp 2026 and Go 1.25.x.
2. Download the `sketchup-mcp-windows-demo` artifact from the CI run being accepted.
3. Install the RBZ from that same artifact into SketchUp.
4. Open the source `.skp` named by `-SourceModel` and leave SketchUp responsive.
5. Keep the reference `.layout` template available on the same machine.

The final acceptance must use the downloaded CI executable, not a developer-local build.
The harness records the workflow run ID, artifact ID, EXE/RBZ SHA-256 hashes, SketchUp
version, session/model identity, output paths, panel summaries, and dimension connection count.

## One-command live acceptance

From a fresh checkout, run:

```powershell
.\scripts\run_live_layout_e2e.ps1 `
  -McpExe C:\acceptance\sketchup-mcp-windows-amd64.exe `
  -Rbz C:\acceptance\giaokhoa_sketchup_mcp.rbz `
  -Pid 15036 `
  -SourceModel C:\fixtures\cabinet-layout-source.skp `
  -Template C:\fixtures\furniture-shopdrawing-a2.layout `
  -OutputDir C:\acceptance\cabinet-live `
  -Fixture .\testdata\e2e\cabinet\spec.json `
  -WorkflowRunId 123456789 `
  -ArtifactId 987654321
```

Use `-Session <session-id>` instead of `-Pid` when selecting by MCP session ID.
The wrapper requires exactly one selector.

The output directory receives:

```text
<fixture output base>.skp
<fixture output base>.layout
<fixture output base>.png
<fixture output base>.pdf
live-layout-e2e-report.json
```

A passing run prints `LIVE_LAYOUT_E2E_PASS`. Any failed assertion exits non-zero and the
report identifies the failed stage. The report also records first/second presentation create
and reuse counts; the second pass must create zero items and reuse every fixture presentation
role without advancing the SketchUp model revision.

## Generic-fixture check

`testdata/e2e/cabinet-minimal/spec.json` uses the same runner with one standard-view
viewport, one connected dimension, two labels, and one populated panel. It deliberately
changes composition only through fixture data. Run it by changing only `-Fixture` and
`-OutputDir` in the command above.

## Fresh-checkout developer checks

Before live acceptance:

```powershell
go test ./...
python scripts/package_demo.py
```

`package_demo.py` remains the local packaging/reproducibility check. For final live
acceptance, substitute the artifact downloaded from the CI run under review.
