# Delete and material mutation research gate

Verified: **2026-09-16**

Issue: #16

Target: SketchUp 2026 / 26.0.429.

## Entity deletion

SketchUp documents `Sketchup::Drawingelement#erase!` for erasing one drawing
element. `Group` inherits `Drawingelement`, and `ComponentInstance` supports
the same drawing-element behavior used by the existing mutation surface.

The API raises `ArgumentError` if the drawing element is an instance used by
`Model#active_path`. SketchUp also documents historical crash behavior before
2023 for that case. The target is 2026, but the MCP tool still rejects deletion
when the target is part of the active edit path instead of relying on an
exception.

Chosen API:

```ruby
entity.erase!
```

A single-target tool does not need the bulk `Entities#erase_entities` API.

## Material/color assignment

SketchUp documents:

- `Model#materials` for the model material collection;
- `Materials#[](name)` for deterministic lookup;
- `Materials#add(name)` to create a material;
- `Material#color=` to set RGB color;
- `Drawingelement#material=` to assign a material to the target.

Chosen flow:

```ruby
material = model.materials[name] || model.materials.add(name)
material.color = Sketchup::Color.new(r, g, b)
entity.material = material
```

The tool accepts only a bounded material name and integer RGB values 0..255.
No texture, PBR, UV, transparency, or remote asset loading is added.

If a named material already exists, the tool deterministically reuses it and
sets the requested color. That behavior is itself part of the undoable SketchUp
operation.

## Mutation semantics

Both additions preserve the existing safe mutation envelope:

- session ID;
- operation ID;
- expected model GUID;
- expected revision;
- durable entity reference;
- exact replay cache;
- stale revision rejection;
- one SketchUp undo operation;
- structured domain errors.

A delete result returns the deleted persistent ID rather than a refreshed
EntityRef because the entity no longer exists after the commit.

A material result returns a refreshed EntityRef and the applied material name
and RGB values.

## Authoritative references

- https://ruby.sketchup.com/Sketchup/Drawingelement.html
- https://ruby.sketchup.com/Sketchup/Materials.html
- https://ruby.sketchup.com/Sketchup/Material.html
- https://ruby.sketchup.com/Sketchup/Group.html
- https://ruby.sketchup.com/Sketchup/ComponentInstance.html
