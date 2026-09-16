# frozen_string_literal: true

require 'minitest/autorun'

module Sketchup
  class ModelObserver; end

  class << self
    attr_accessor :active_model, :undo_proc

    def version
      '26.0.429'
    end

    def undo
      undo_proc&.call
      nil
    end
  end
end

module Geom
  class Transformation
    attr_reader :vector

    def self.translation(vector)
      new(vector)
    end

    def initialize(vector)
      @vector = vector
    end
  end
end

require_relative '../sketchup/giaokhoa_sketchup_mcp/model_state'
require_relative '../sketchup/giaokhoa_sketchup_mcp/mutation_engine'

class MutationEngineTest < Minitest::Test
  ModelState = Giaokhoa::SketchupMcp::ModelState
  MutationEngine = Giaokhoa::SketchupMcp::MutationEngine
  SESSION_ID = '11111111-1111-4111-8111-111111111111'

  Normal = Struct.new(:z)
  class FakeFace
    attr_reader :normal, :pushpull_distance

    def initialize(model)
      @model = model
      @normal = Normal.new(1)
    end

    def reverse!
      @normal.z = -@normal.z
      self
    end

    def pushpull(distance)
      @pushpull_distance = distance
      @model.mark_changed
      nil
    end
  end

  class FakeChildEntities
    attr_reader :last_face

    def initialize(model)
      @model = model
    end

    def add_face(_points)
      @last_face = FakeFace.new(@model)
    end
  end

  class FakeEntity
    attr_reader :persistent_id, :transform_count, :entities
    attr_accessor :raise_on_transform

    def initialize(model:, persistent_id:, type: 'Group', locked: false, valid: true)
      @model = model
      @persistent_id = persistent_id
      @type = type
      @locked = locked
      @valid = valid
      @transform_count = 0
      @entities = FakeChildEntities.new(model)
      @raise_on_transform = false
    end

    def typename
      @type
    end

    def locked?
      @locked
    end

    def valid?
      @valid
    end

    def transform!(_transform)
      raise 'transform exploded' if @raise_on_transform

      @transform_count += 1
      @model.mark_changed
      self
    end
  end
  class FakeEntities
    attr_reader :groups

    def initialize(model)
      @model = model
      @groups = []
    end

    def add_group
      group = FakeEntity.new(model: @model, persistent_id: @model.next_persistent_id)
      @groups << group
      @model.register(group)
      @model.mark_changed
      group
    end
  end

  class FakeModel
    attr_accessor :guid
    attr_reader :observer, :start_count, :commit_count, :abort_count, :active_entities

    def initialize(guid: 'guid-a')
      @guid = guid
      @lookup = {}
      @next_persistent_id = 100
      @active_path = []
      @active_entities = FakeEntities.new(self)
      @changed = false
      @operation_open = false
      @start_count = 0
      @commit_count = 0
      @abort_count = 0
    end

    def add_observer(observer)
      @observer = observer
    end

    def remove_observer(observer)
      @observer = nil if @observer.equal?(observer)
    end

    def title
      'Mutation demo'
    end

    def path
      'C:/mutation-demo.skp'
    end

    def active_path
      @active_path
    end

    def register(entity)
      @lookup[entity.persistent_id] = entity
      entity
    end

    def find_entity_by_persistent_id(id)
      @lookup[id]
    end

    def next_persistent_id
      value = @next_persistent_id
      @next_persistent_id += 1
      value
    end
    def mark_changed
      @changed = true
    end

    def start_operation(_label, disable_ui)
      raise 'nested operation' if @operation_open
      raise 'disable_ui must be true' unless disable_ui

      @operation_open = true
      @changed = false
      @start_count += 1
      true
    end

    def commit_operation
      raise 'no operation open' unless @operation_open

      @operation_open = false
      @commit_count += 1
      @observer.onTransactionCommit(self) if @changed
      true
    end

    def abort_operation
      @operation_open = false
      @changed = false
      @abort_count += 1
      true
    end

    def undo_last
      @observer.onTransactionUndo(self)
    end
  end

  def setup
    @model = FakeModel.new
    Sketchup.active_model = @model
    @state = ModelState.new
    @state.track(@model)
    @engine = MutationEngine.new(session_id: SESSION_ID, model_state: @state)
    @entity = @model.register(FakeEntity.new(model: @model, persistent_id: 42))
    Sketchup.undo_proc = -> { @model.undo_last }
  end

  def teardown
    Sketchup.undo_proc = nil
    @state.stop
    Sketchup.active_model = nil
  end

  def envelope(operation_id:, revision: 0, undo_label: nil)
    {
      'mutation' => {
        'operation_id' => operation_id,
        'expected_model_guid' => 'guid-a',
        'expected_revision' => revision,
        'undo_label' => undo_label
      }
    }
  end
  def entity_ref(entity = @entity, revision: 0)
    {
      'session_id' => SESSION_ID,
      'model_guid' => 'guid-a',
      'persistent_id' => entity.persistent_id,
      'revision' => revision
    }
  end

  def translate_payload(operation_id:, revision: 0, entity: @entity, vector: [1, 2, 3])
    envelope(
      operation_id: operation_id,
      revision: revision,
      undo_label: 'SketchUp MCP: Translate'
    ).merge(
      'entity_ref' => entity_ref(entity, revision: revision),
      'translation_inches' => {
        'x' => vector[0],
        'y' => vector[1],
        'z' => vector[2]
      }
    )
  end

  def box_payload(operation_id:, revision: 0)
    envelope(
      operation_id: operation_id,
      revision: revision,
      undo_label: 'SketchUp MCP: Create Box'
    ).merge(
      'origin_inches' => {'x' => 10, 'y' => 20, 'z' => 30},
      'dimensions_inches' => {'width' => 4, 'depth' => 5, 'height' => 6}
    )
  end

  def test_duplicate_operation_id_applies_exactly_once
    payload = translate_payload(operation_id: 'translate-once')

    first = @engine.translate(payload)
    second = @engine.translate(payload)

    assert first.fetch(:ok)
    assert second.fetch(:ok)
    assert_equal first, second
    assert_equal 1, @entity.transform_count
    assert_equal 1, @model.start_count
    assert_equal 1, @model.commit_count
    assert_equal 1, first.fetch(:payload).fetch('revision')
  end

  def test_duplicate_operation_id_with_different_input_is_rejected
    first = @engine.translate(translate_payload(operation_id: 'same-id', vector: [1, 0, 0]))
    conflict = @engine.translate(translate_payload(operation_id: 'same-id', vector: [2, 0, 0]))

    assert first.fetch(:ok)
    refute conflict.fetch(:ok)
    assert_equal 'OPERATION_ID_REUSE', conflict.fetch(:error).fetch('code')
    assert_equal 1, @entity.transform_count
  end
  def test_stale_revision_is_rejected_before_mutation
    @model.observer.onTransactionCommit(@model)

    result = @engine.translate(translate_payload(operation_id: 'stale-write'))

    refute result.fetch(:ok)
    assert_equal 'STALE_REVISION', result.fetch(:error).fetch('code')
    assert_equal 0, @entity.transform_count
    assert_equal 0, @model.start_count
  end

  def test_successful_mutation_increments_revision_and_refreshes_entity_ref
    result = @engine.translate(translate_payload(operation_id: 'successful-write'))

    assert result.fetch(:ok)
    output = result.fetch(:payload)
    assert_equal 1, output.fetch('revision')
    assert_equal 1, output.fetch('entity_ref').fetch('revision')
    assert_equal 42, output.fetch('entity_ref').fetch('persistent_id')
    assert_equal 1, @state.snapshot.fetch(:revision)
  end

  def test_failed_started_mutation_aborts_and_replay_returns_same_failure
    @entity.raise_on_transform = true
    payload = translate_payload(operation_id: 'failing-write')

    first = @engine.translate(payload)
    second = @engine.translate(payload)

    refute first.fetch(:ok)
    assert_equal 'SKETCHUP_OPERATION_FAILED', first.fetch(:error).fetch('code')
    assert_equal first, second
    assert_equal 1, @model.start_count
    assert_equal 1, @model.abort_count
    assert_equal 0, @model.commit_count
    assert_equal 0, @state.snapshot.fetch(:revision)
  end

  def test_box_creation_returns_persistent_identity
    result = @engine.create_box(box_payload(operation_id: 'box-1'))

    assert result.fetch(:ok)
    output = result.fetch(:payload)
    ref = output.fetch('entity_ref')
    assert_equal SESSION_ID, ref.fetch('session_id')
    assert_equal 'guid-a', ref.fetch('model_guid')
    assert_operator ref.fetch('persistent_id'), :>, 0
    assert_equal 1, ref.fetch('revision')
    assert_equal 1, @model.active_entities.groups.length
  end

  def test_two_writes_against_same_revision_cannot_both_succeed
    other = @model.register(FakeEntity.new(model: @model, persistent_id: 43))

    first = @engine.translate(translate_payload(operation_id: 'write-a', entity: @entity))
    second = @engine.translate(translate_payload(operation_id: 'write-b', entity: other))

    assert first.fetch(:ok)
    refute second.fetch(:ok)
    assert_equal 'STALE_REVISION', second.fetch(:error).fetch('code')
    assert_equal 1, @entity.transform_count
    assert_equal 0, other.transform_count
  end

  def test_undo_uses_sketchup_undo_without_opening_nested_operation
    mutation = @engine.translate(translate_payload(operation_id: 'before-undo'))
    assert mutation.fetch(:ok)

    undo_payload = envelope(
      operation_id: 'undo-1', revision: 1, undo_label: 'SketchUp MCP: Undo'
    )
    result = @engine.undo(undo_payload)
    replay = @engine.undo(undo_payload)

    assert result.fetch(:ok)
    assert_equal 2, result.fetch(:payload).fetch('revision')
    assert_equal result, replay
    assert_equal 1, @model.start_count
    assert_equal 1, @model.commit_count
  end

  def test_deleted_and_locked_targets_return_stable_errors_without_opening_operation
    deleted = translate_payload(operation_id: 'deleted').merge(
      'entity_ref' => entity_ref.merge('persistent_id' => 999)
    )
    deleted_result = @engine.translate(deleted)
    refute deleted_result.fetch(:ok)
    assert_equal 'ENTITY_DELETED', deleted_result.fetch(:error).fetch('code')

    locked = @model.register(FakeEntity.new(model: @model, persistent_id: 44, locked: true))
    locked_result = @engine.translate(
      translate_payload(operation_id: 'locked', entity: locked)
    )
    refute locked_result.fetch(:ok)
    assert_equal 'LOCKED_ENTITY_OR_CONTEXT', locked_result.fetch(:error).fetch('code')
    assert_equal 0, @model.start_count
  end

  def test_invalid_box_dimensions_are_rejected_before_start_operation
    payload = box_payload(operation_id: 'bad-box')
    payload['dimensions_inches']['height'] = 0

    result = @engine.create_box(payload)

    refute result.fetch(:ok)
    assert_equal 'INVALID_DIMENSIONS', result.fetch(:error).fetch('code')
    assert_equal 0, @model.start_count
  end
end
