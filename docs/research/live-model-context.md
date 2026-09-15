# Live model context and identity semantics

Issue #6 adds a deliberately small, read-only model surface: `model.summary`,
`selection.get`, and `entity.inspect`. No MCP resources mirror these tools because
the current clients already interoperate with typed tool output and resources
would duplicate the same volatile live state.

## Verified SketchUp API behavior

The implementation uses `Model#guid` for the model identity epoch and
`Entity#persistent_id` plus `Model#find_entity_by_persistent_id` for entity
identity. Public output never relies on `entityID`, which SketchUp documents as
non-persistent between sessions.

SketchUp documents that `Model#guid` changes after a modified model is saved.
Therefore a GUID change is treated as a new model identity epoch even if the Ruby
`Model` object itself is unchanged. Existing `EntityRef` values are invalidated
with `MODEL_CHANGED`, and the per-session revision rebases to zero.

The revision starts at zero when a model/identity epoch is attached. It increases
once for each `ModelObserver#onTransactionCommit` callback, and once for each
undo or redo callback. `onTransactionCommit` is documented to fire only for
transactions that changed the model; empty transactions receive
`onTransactionEmpty` instead and do not increment the revision. Observer
callbacks only update bookkeeping.

`Model#active_path` and `Model#edit_transform` describe the current edit context.
Selection enumeration uses `Selection#each`, and the public response is capped at
100 items with explicit `selected_count`, `returned_count`, and `truncated`
fields. No query recursively traverses the model graph.

## EntityRef and stale reads

`EntityRef` contains exactly:

- `session_id`
- `model_guid`
- `persistent_id`
- `revision`

No instance path is needed for the supported demo entities (`Group` and
`ComponentInstance`) because their persistent IDs are model-wide lookup keys for
`Model#find_entity_by_persistent_id`.

`entity.inspect` requires the session and model identity to still match. A missing
entity returns `ENTITY_NOT_FOUND`; unsupported entity kinds return
`ENTITY_TYPE_NOT_SUPPORTED`; and a supported entity without a positive persistent
ID returns `ENTITY_HAS_NO_SUPPORTED_PERSISTENT_ID`.

A revision mismatch alone is not an error for this read-only operation.
Inspection resolves the entity at the current revision and returns
`revision_mismatch: true` plus a refreshed `EntityRef`. Later mutation tools must
perform stricter stale-write checks.

## MCP SDK decision

The Go host uses the official SDK generic `mcp.AddTool` API. Input and output
schemas are inferred from Go structs, arguments and results are validated, and
typed output is automatically placed in `structuredContent` (with the JSON text
compatibility content supplied by the SDK). All three tools are annotated
read-only and closed-world.

Domain failures are returned as typed structured output with `isError: true`.
Transport/internal failures remain ordinary MCP tool failures.
