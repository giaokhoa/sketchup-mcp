# LayOut template-first contract

This note defines the machine-readable contract used by the LayOut MCP template workflow.

## API facts

- `Layout::Document.new(template_path)` creates a new unsaved document from an existing `.layout` file.
- LayOut templates preserve their pages and entities.
- `Layout::Layer` supports shared/non-shared layers.
- `Layout::Entity`, `Layout::Page`, and `Layout::Document` support attribute dictionaries in LayOut 2026.
- `Layout::Style` can be read from one entity and assigned to another.
- `Layout::AutoTextDefinition` includes model scene name, scale, section name, and section symbol definitions.
- `Layout::SketchUpModel` can select a SketchUp scene, use orthographic scale, and preserve scale on resize.

Official references:
- https://ruby.sketchup.com/Layout/Document.html
- https://ruby.sketchup.com/Layout/Layer.html
- https://ruby.sketchup.com/Layout/Entity.html
- https://ruby.sketchup.com/Layout/Style.html
- https://ruby.sketchup.com/Layout/AutoTextDefinition.html
- https://ruby.sketchup.com/Layout/SketchUpModel.html
- https://help.sketchup.com/en/layout/creating-template-layout
- https://help.sketchup.com/en/layout/automate-titleblocks

## Attribute dictionary

Use one dictionary:

`giaokhoa.layout_template`

### Document attributes

| key | type | meaning |
| --- | --- | --- |
| `schema_version` | Integer | template metadata schema, initially 1 |
| `template_id` | String | stable template identifier |
| `template_kind` | String | broad reusable class such as `furniture_shopdrawing` |

### Viewport slot entity attributes

A slot is represented by a template entity whose `bounds` define the paper-space allocation rectangle.

| key | type | meaning |
| --- | --- | --- |
| `role` | String | `viewport_slot` |
| `slot_id` | String | stable slot name |
| `default_scale_denominator` | Float | optional default orthographic scale denominator |
| `perspective` | Boolean | optional intended projection mode |

The first reference template uses:

- `plan`
- `front`
- `section_a`
- `section_b`
- `side`
- `iso`

These IDs describe drawing roles, not model-specific geometry.

### Style sample entity attributes

A tagged entity may act as a reusable style carrier.

| key | type | meaning |
| --- | --- | --- |
| `role` | String | `style_sample` |
| `style_id` | String | stable style role |

Initial style IDs:

- `border`
- `caption`
- `scale_label`
- `note`
- `section_marker`
- `dimension`

A client may request a style by ID and apply the sample entity's `Layout::Style` to a newly created compatible entity.

## Layer policy

The reference template uses separate layers by responsibility:

- `00 Shared Frame` — shared;
- `10 Viewports` — non-shared;
- `20 Dimensions` — non-shared;
- `30 Section Marks` — non-shared;
- `40 Annotations` — non-shared;
- `50 Notes` — non-shared.

Only reusable document-wide content belongs on shared layers.

## Auto-Text policy

Prefer LayOut Auto-Text where text is inherently derived from the referenced model:

- scene name;
- model scale;
- section name;
- section symbol.

Literal project notes remain normal text.

## Units

All MCP paper-space input/output values are millimeters.

LayOut's numeric paper coordinates are converted at the Ruby API boundary only.

## Expected `layout.template.inspect` shape

```json
{
  "template_id": "furniture-shopdrawing-a3-v1",
  "template_kind": "furniture_shopdrawing",
  "schema_version": 1,
  "pages": [
    {
      "index": 0,
      "name": "Sheet 1",
      "width_mm": 420,
      "height_mm": 297
    }
  ],
  "layers": [
    {"name": "00 Shared Frame", "shared": true, "locked": true},
    {"name": "10 Viewports", "shared": false, "locked": false}
  ],
  "slots": [
    {
      "slot_id": "front",
      "page_index": 0,
      "bounds_mm": {"x": 142, "y": 8, "width": 154, "height": 132},
      "default_scale_denominator": 10,
      "perspective": false
    }
  ],
  "styles": [
    {"style_id": "dimension", "page_index": 0},
    {"style_id": "caption", "page_index": 0}
  ],
  "auto_text_types": [
    "model_scene_name",
    "model_scale",
    "model_section_name",
    "model_section_symbol"
  ]
}
```

## Non-goals

The metadata contract does not encode:

- cabinet dimensions;
- cabinet entity names;
- number of doors/drawers;
- a required six-view workflow;
- rules for choosing which views a caller must create.

The template expresses reusable presentation structure. The caller decides how to populate it.
