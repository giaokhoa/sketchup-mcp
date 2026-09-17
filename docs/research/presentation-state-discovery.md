# SketchUp presentation-state discovery research

Issue: #40. Verified against the SketchUp 2026 Ruby API before implementation.

## API choices

- Scenes are enumerated through `Model#pages` / `Sketchup::Pages#each`. `Pages#selected_page` identifies the active scene.
- `Sketchup::Page#camera` provides read-only camera state without activating the page. Page persistent IDs are reported as supplemental identity; the bridge continues to expose a stable zero-based page index and scene name for presentation reuse.
- SketchUp 2026 `Page#active_section_planes` exposes the section planes captured by a scene. The public list remains bounded and reports the first captured plane as a durable `EntityRef`, which is sufficient for the root-level presentation workflow used by the current bridge.
- Root section planes are enumerated from `model.entities.grep(Sketchup::SectionPlane)`. `SectionPlane#persistent_id` supplies durable identity, while `name`, `symbol`, `get_plane`, and `active?` supply bounded discovery metadata.
- `SectionPlane#get_plane` returns `[A, B, C, D]` for `Ax + By + Cz + D = 0`. The bridge converts this to a model-coordinate point nearest the model origin plus a unit normal; coordinates are returned in millimeters.
- `Entities#active_section_plane` is readable and writable, and `Pages#erase` can remove a page. No lifecycle delete tool is added in this issue because the repeated-run fixture can reuse exact matching state and therefore does not demonstrate a concrete replacement need.

## Bounded public contract

Both `scene.list` and `section_plane.list` return at most 100 entries plus `total_count`, `returned_count`, and `truncated`. They expose only the camera/plane state required for reuse decisions rather than dumping unbounded rendering/style data.

## Repeated-run policy

The live E2E client lists presentation state before writes. Exact fixture matches are reused. Duplicate names or stale mismatches are treated as deterministic acceptance failures rather than creating another scene or section plane. A second presentation-preparation pass must perform zero creates and leave the model revision unchanged.

## Official references

- https://ruby.sketchup.com/Sketchup/Pages.html
- https://ruby.sketchup.com/Sketchup/Page.html
- https://ruby.sketchup.com/Sketchup/SectionPlane.html
- https://ruby.sketchup.com/Sketchup/Entities.html
