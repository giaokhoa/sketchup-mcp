# Safe mutation real-SketchUp smoke

Target: Windows 11 x64 + SketchUp Desktop 2026 `26.x`.
Verified on **2026-09-16** with SketchUp **26.0.429**.

## Preconditions

1. Install/load the issue #7 extension payload.
2. Open a model containing at least one unlocked group/component instance.
3. Select that entity and call `selection.get` to obtain a current `EntityRef`.
4. Record the current `model_guid`, revision, transform/bounds, and target PID.
5. Use a fresh `operation_id` for each intended logical write.

## Translation and idempotency

1. Call `entity.translate` with a visible known translation and current revision.
2. Confirm one successful response and a strictly newer model revision.
3. Confirm the returned `EntityRef` uses the same persistent ID and new revision.
4. Replay the exact same request including the same `operation_id`.
5. Confirm the cached result is returned and the entity does not move again.
6. Invoke one ordinary SketchUp Undo entry and confirm placement is restored.

## Stale write

1. Change the model or complete another mutation so revision advances.
2. Submit a new `entity.translate` operation using the older revision/ref.
3. Expect `STALE_REVISION`.
4. Confirm transform/bounds are unchanged by the rejected request.

## Box creation and undo

1. Read the current revision.
2. Call `geometry.create_box` with positive width/depth/height and explicit origin.
3. Confirm the result contains a positive persistent ID and current model GUID.
4. Confirm exactly one grouped rectangular prism appears.
5. Call `changes.undo` with the current revision and a fresh operation ID.
6. Confirm revision advances and the created group is removed.

## Safety/responsiveness checks

- A locked target returns `LOCKED_ENTITY_OR_CONTEXT` without model change.
- Reusing an operation ID with different parameters returns `OPERATION_ID_REUSE`.
- No write leaves a SketchUp operation open after an exception.
- Repeated reads/writes do not freeze the SketchUp UI.
- The bridge remains authenticated and responsive after each mutation.

## Automated regression gate

Before recording the real smoke result, all must pass:

- `go vet ./...`
- `go test -race ./...`
- Ruby syntax check for all files under `sketchup/` and `test/`
- `test/protocol_test.rb`
- `test/dispatcher_test.rb`
- `test/model_context_test.rb`
- `test/server_integration_test.rb`
- `test/mutation_engine_test.rb`
- `git diff --check`

## Verified result

PASS on SketchUp 26.0.429.

- Test model: a temporary copy of SketchUp's shipped simple template.
- Existing selected entity: `ComponentInstance`, persistent ID `45058`.
- Translation: +12 internal inches on X.
- Transform X translation changed from `-14.4330720058` to `-2.4330720058`.
- First mutation advanced revision `0 -> 1`.
- `changes.undo` restored the exact original transform and advanced `1 -> 2`.
- Replaying the original operation ID returned the recorded result but left the
  live model at revision `2`; the entity did not move again.
- A fresh write using stale revision `0` returned `STALE_REVISION`; revision
  remained `2`.
- Box creation at origin `[36, 0, 0]` with dimensions `[12, 10, 8]` returned
  persistent ID `48880`, bounds `[36,0,0] -> [48,10,8]`, and revision `3`.
- Undoing box creation advanced `3 -> 4`; inspecting the removed persistent ID
  returned `ENTITY_NOT_FOUND`; top-level entity count returned to `1`.
- Final SketchUp process reported `Responding=True`.

The smoke caught a pre-existing dispatcher bug where string-keyed structured
domain errors were read with symbol keys and collapsed to `INTERNAL`. The
dispatcher was fixed and a regression test was added before the smoke was rerun
from a clean SketchUp session. No bridge token was recorded.
