# Assembly hierarchy research gate

Verified: **2026-09-16**

Issue: #21

Target: SketchUp 2026 / 26.0.429.

## Grouping existing entities

SketchUp documents `Sketchup::Entities#add_group(entities)` for creating a new
Group from existing entities in one Entities collection.

The API documentation warns about crashes before SketchUp 8 when passing
existing entities. That historical caveat does not apply to the verified
SketchUp 2026 target.

Chosen mutation:

```ruby
assembly = parent.entities.add_group(children)
assembly.name = name
```

The MCP requires all child references to resolve to supported Group or
ComponentInstance entities with one shared parent. It also rejects grouping
inside a shared component definition so an instance-scoped request cannot
silently restructure every instance of a shared definition.

## Durable identity after nesting

SketchUp documents persistent IDs as durable entity identities and
`Model#find_entity_by_persistent_id` as a model-wide lookup.

The assembly mutation therefore stores child persistent IDs before grouping and
re-resolves every child after commit before returning refreshed EntityRefs.
The real SketchUp acceptance test must prove that every original cabinet part
remains addressable after nesting.

## Reading hierarchy

`Sketchup::Group#entities` exposes the entities inside a Group.
For ComponentInstance, child entities are read from its definition.

`entity.children.list` is deliberately bounded to the first 100 immediate
supported Group / ComponentInstance children. It also reports raw immediate
entity count and total supported child count so truncation is explicit.

## Authoritative references

- https://ruby.sketchup.com/Sketchup/Entities.html#add_group-instance_method
- https://ruby.sketchup.com/Sketchup/Group.html#entities-instance_method
- https://ruby.sketchup.com/Sketchup/Model.html#find_entity_by_persistent_id-instance_method
- https://ruby.sketchup.com/Sketchup/Entity.html#persistent_id-instance_method
