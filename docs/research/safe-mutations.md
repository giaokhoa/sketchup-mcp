# Safe mutation engine research

Verified: **2026-09-16**

Issue #7 implements only translation, grouped rectangular box creation, and undo.
All SketchUp API calls continue to execute through the existing main-thread dispatcher.

## Verified SketchUp API behavior

Authoritative references:
- https://ruby.sketchup.com/Sketchup/Model.html#start_operation-instance_method
- https://ruby.sketchup.com/Sketchup/Model.html#commit_operation-instance_method
- https://ruby.sketchup.com/Sketchup/Model.html#abort_operation-instance_method
- https://ruby.sketchup.com/Sketchup/Group.html#transform!-instance_method
- https://ruby.sketchup.com/Sketchup/ComponentInstance.html#transform!-instance_method
- https://ruby.sketchup.com/Geom/Transformation.html#translation-class_method
- https://ruby.sketchup.com/Sketchup/Entity.html#valid?-instance_method
- https://ruby.sketchup.com/Sketchup/Group.html#locked?-instance_method
- https://ruby.sketchup.com/Sketchup/ComponentInstance.html#locked?-instance_method
- https://ruby.sketchup.com/Sketchup/Model.html#active_entities-instance_method
- https://ruby.sketchup.com/Sketchup/Entities.html#add_group-instance_method
- https://ruby.sketchup.com/Sketchup/Entities.html#add_face-instance_method
- https://ruby.sketchup.com/Sketchup/Face.html#pushpull-instance_method
- https://ruby.sketchup.com/Sketchup.html#undo-class_method

SketchUp documents operations as sequential and non-nestable. Every engine-owned
write therefore uses exactly one `start_operation(label, true)` followed by
`commit_operation`; exceptions after start call `abort_operation` from the same
main-thread call path. Transparent operations are not used.

Translation uses `Group#transform!` / `ComponentInstance#transform!` with
`Geom::Transformation.translation`. It intentionally does not use `move!`,
because SketchUp documents `move!` as not recording to the undo stack.
Only translation is exposed; rotation and scale are deferred.

Target resolution uses the #6 persistent-id path and then checks `Entity#valid?`.
Groups and component instances are rejected when `locked?`. The current
`Model#active_path` is also checked for locked instances before mutation.

Box creation uses `Model#active_entities`, creates an empty group with
`Entities#add_group`, adds a four-point face with `Entities#add_face`, normalizes
the ground-plane face direction when necessary, then calls `Face#pushpull`.
This keeps all primitive geometry inside one returned durable group.

Undo uses the documented synchronous `Sketchup.undo` API:
https://ruby.sketchup.com/Sketchup.html#undo-class_method

`Sketchup.send_action("viewUndo:")` is not used because `send_action` is
deprecated and asynchronous. Undo is not wrapped in a new SketchUp operation;
the revision observer confirms that the undo actually changed the model.

## Internal bridge mutation envelope

The public MCP write inputs are flat typed structs. Before crossing the private
Go-to-Ruby bridge, they are converted to the ADR #2 envelope:

    "mutation": {
      "operation_id": "...",
      "expected_model_guid": "...",
      "expected_revision": 42,
      "undo_label": "SketchUp MCP: ..."
    }

The bridge request itself already carries and authenticates `session_id`.
Entity translation additionally carries the durable `EntityRef`.

## Idempotency and failure semantics

Idempotency state is in-memory and scoped to the bridge session/model identity
epoch. A model GUID change clears the cache.

- completed + same ID/fingerprint: return the recorded result, do not execute;
- same ID + different fingerprint: `OPERATION_ID_REUSE`;
- running + same fingerprint: `OPERATION_IN_PROGRESS`;
- stale revision with a new ID: `STALE_REVISION` before `start_operation`;
- exception after operation start: abort, record `SKETCHUP_OPERATION_FAILED`,
  and replay that same recorded failure for the same ID/fingerprint;
- validation/precondition failures before execution starts are not recorded,
  because no mutation has become uncertain or partially started.

The fingerprint excludes only `operation_id`; it includes model/revision,
operation parameters, entity reference, and fixed undo label.

The dispatcher serializes main-thread commands. Therefore two distinct writes
submitted against the same revision cannot both succeed: the first committed
transaction advances revision before the second request reaches its stale check.
