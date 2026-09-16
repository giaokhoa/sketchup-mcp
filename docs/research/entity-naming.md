# Entity naming research gate

Verified: **2026-09-16**

Issue: #19

Target: SketchUp 2026 / 26.0.429.

## API choice

SketchUp documents instance-level name setters for both supported structured
entity types:

- `Sketchup::Group#name=`
- `Sketchup::ComponentInstance#name=`

The MCP tool deliberately writes the Group / ComponentInstance instance name.
It does **not** rename a ComponentDefinition, because changing a shared
definition name would have wider effects than the explicitly referenced
instance.

Chosen API:

```ruby
entity.name = name
```

The name is bounded to a non-empty UTF-8 string of at most 128 bytes.

## Mutation behavior

`entity.name.set` preserves the existing mutation envelope:

- session ID;
- operation ID;
- expected model GUID;
- expected revision;
- durable EntityRef;
- idempotent replay;
- stale-revision rejection;
- one SketchUp undo operation;
- structured errors.

A no-op request that supplies the already-current entity name is rejected
instead of opening an operation that may not advance the model revision.

## Real SketchUp acceptance

A fresh SketchUp 2026 instance using the packaged PR artifact rebuilt the
1800 mm cabinet as 34 independently editable Groups.

Observed through MCP:

- 10 tools discovered;
- rename replay returned the cached result without advancing revision;
- undo restored the previous instance name;
- stale rename returned `STALE_REVISION`;
- all 34 final Groups received distinct human-readable names;
- `entity.inspect` read all 34 names back;
- after selecting the complete cabinet, `selection.get` returned 34/34 named
  Groups with 34 unique names;
- overall width remained 1800.000 mm;
- SketchUp remained responsive.

## Authoritative references

- https://ruby.sketchup.com/Sketchup/Group.html
- https://ruby.sketchup.com/Sketchup/ComponentInstance.html
