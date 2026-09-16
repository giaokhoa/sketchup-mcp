# frozen_string_literal: true

require 'minitest/autorun'

module Sketchup
  class ModelObserver; end

  class << self
    attr_accessor :active_model

    def version
      '26.0.429'
    end
  end
end

require_relative '../sketchup/giaokhoa_sketchup_mcp/model_state'
require_relative '../sketchup/giaokhoa_sketchup_mcp/mutation_engine'
require_relative '../sketchup/giaokhoa_sketchup_mcp/commands/registry'

class ModelContextTest < Minitest::Test
  ModelState = Giaokhoa::SketchupMcp::ModelState
  Registry = Giaokhoa::SketchupMcp::Commands::Registry
  SESSION_ID = '11111111-1111-4111-8111-111111111111'

  Point = Struct.new(:x, :y, :z) do
    def to_a
      [x, y, z]
    end
  end

  Bounds = Struct.new(:min, :max)

  class Transform
    def initialize(values = nil)
      @values = values || [
        1, 0, 0, 0,
        0, 1, 0, 0,
        0, 0, 1, 0,
        0, 0, 0, 1
      ]
    end

    def to_a
      @values
    end
  end

  Named = Struct.new(:name)
  Definition = Struct.new(:name, :entities)

  class FakeEntity
    attr_reader :persistent_id, :name, :bounds, :transformation, :material, :layer, :definition

    def initialize(type: 'Group', persistent_id: 1, name: 'Entity', definition: nil)
      @type = type
      @persistent_id = persistent_id
      @name = name
      @bounds = Bounds.new(Point.new(0, 0, 0), Point.new(1, 2, 3))
      @transformation = Transform.new
      @material = Named.new('Material')
      @layer = Named.new('Untagged')
      @definition = definition || Definition.new('Definition', [Object.new])
    end

    def typename
      @type
    end
  end

  class FakeSelection
    include Enumerable

    def initialize(entities)
      @entities = entities
    end

    def each(&block)
      @entities.each(&block)
    end

    def length
      @entities.length
    end
  end

  class FakeModel
    attr_accessor :guid
    attr_reader :title, :path, :selection, :entities, :definitions, :materials, :layers,
                :edit_transform, :observer

    def initialize(guid:, selection: [], title: 'Demo')
      @guid = guid
      @title = title
      @path = 'C:/demo.skp'
      @selection = FakeSelection.new(selection)
      @entities = Array.new(3)
      @definitions = Array.new(2)
      @materials = Array.new(1)
      @layers = Array.new(1)
      @edit_transform = Transform.new
      @active_path = []
      @lookup = selection.each_with_object({}) do |entity, result|
        result[entity.persistent_id] = entity if entity.persistent_id.positive?
      end
    end

    def add_observer(observer)
      @observer = observer
    end

    def remove_observer(observer)
      @observer = nil if @observer.equal?(observer)
    end

    def active_path
      @active_path
    end

    def options
      {
        'UnitsOptions' => {
          'LengthUnit' => 2,
          'LengthFormat' => 0,
          'LengthPrecision' => 2
        }
      }
    end

    def find_entity_by_persistent_id(persistent_id)
      @lookup[persistent_id]
    end
  end

  def setup
    @entity = FakeEntity.new(persistent_id: 42, name: 'Selected group')
    @model = FakeModel.new(guid: 'guid-a', selection: [@entity])
    Sketchup.active_model = @model
    @state = ModelState.new
    @state.track(@model)
    @registry = Registry.new(session_id: SESSION_ID, model_state: @state)
  end

  def teardown
    @state.stop
    Sketchup.active_model = nil
  end

  def test_revision_increments_for_commit_undo_and_redo_bookkeeping
    assert_equal 0, @state.snapshot.fetch(:revision)

    @model.observer.onTransactionCommit(@model)
    @model.observer.onTransactionUndo(@model)
    @model.observer.onTransactionRedo(@model)

    assert_equal 3, @state.snapshot.fetch(:revision)
  end

  def test_revision_does_not_depend_on_ruby_model_wrapper_identity
    callback_model = FakeModel.new(guid: 'guid-a')
    @model.observer.onTransactionCommit(callback_model)

    Sketchup.active_model = FakeModel.new(guid: 'guid-a')

    assert_equal 1, @state.snapshot.fetch(:revision)
  end

  def test_guid_change_rebases_revision_and_invalidates_old_reference_epoch
    @model.observer.onTransactionCommit(@model)
    assert_equal 1, @state.snapshot.fetch(:revision)

    @model.guid = 'guid-b'

    snapshot = @state.snapshot
    assert_equal 'guid-b', snapshot.fetch(:guid)
    assert_equal 0, snapshot.fetch(:revision)
  end

  def test_model_replacement_rebases_revision_and_invalidates_old_reference
    @model.observer.onTransactionCommit(@model)
    replacement = FakeModel.new(guid: 'guid-c')
    Sketchup.active_model = replacement

    snapshot = @state.snapshot
    assert_equal 'guid-c', snapshot.fetch(:guid)
    assert_equal 0, snapshot.fetch(:revision)

    result = @registry.call(
      'entity.inspect',
      'session_id' => SESSION_ID,
      'model_guid' => 'guid-a',
      'persistent_id' => 42,
      'revision' => 1
    )

    refute result.fetch(:ok)
    assert_equal 'MODEL_CHANGED', result.fetch(:error).fetch('code')
    assert_equal 'guid-c', result.fetch(:error).fetch('details').fetch('current_model_guid')
  end

  def test_selection_is_bounded_and_returns_durable_refs
    entities = Array.new(101) { |i| FakeEntity.new(persistent_id: i + 1, name: "Group #{i + 1}") }
    model = FakeModel.new(guid: 'guid-many', selection: entities)
    Sketchup.active_model = model

    result = @registry.call('selection.get', {})
    assert result.fetch(:ok)
    payload = result.fetch(:payload)
    assert_equal 101, payload.fetch('selected_count')
    assert_equal 100, payload.fetch('returned_count')
    assert payload.fetch('truncated')
    first_ref = payload.fetch('entities').first.fetch('ref')
    assert_equal SESSION_ID, first_ref.fetch('session_id')
    assert_equal 'guid-many', first_ref.fetch('model_guid')
    assert_equal 1, first_ref.fetch('persistent_id')
    refute first_ref.key?('entityID')
  end

  def test_inspect_stale_revision_returns_current_entity_and_mismatch
    @model.observer.onTransactionCommit(@model)

    result = @registry.call(
      'entity.inspect',
      'session_id' => SESSION_ID,
      'model_guid' => 'guid-a',
      'persistent_id' => 42,
      'revision' => 0
    )

    assert result.fetch(:ok)
    payload = result.fetch(:payload)
    assert_equal 1, payload.fetch('current_revision')
    assert payload.fetch('revision_mismatch')
    assert_equal 1, payload.fetch('entity').fetch('ref').fetch('revision')
  end

  def test_inspect_old_model_epoch_returns_model_changed
    result = @registry.call(
      'entity.inspect',
      'session_id' => SESSION_ID,
      'model_guid' => 'old-guid',
      'persistent_id' => 42,
      'revision' => 0
    )

    refute result.fetch(:ok)
    error = result.fetch(:error)
    assert_equal 'MODEL_CHANGED', error.fetch('code')
    assert_equal 'guid-a', error.fetch('details').fetch('current_model_guid')
  end

  def test_unsupported_and_no_pid_entities_are_reported
    face = FakeEntity.new(type: 'Face', persistent_id: 7)
    no_pid = FakeEntity.new(type: 'Group', persistent_id: 0)
    model = FakeModel.new(guid: 'guid-errors', selection: [face, no_pid])
    Sketchup.active_model = model

    selection = @registry.call('selection.get', {}).fetch(:payload).fetch('entities')
    assert_equal 'ENTITY_TYPE_NOT_SUPPORTED', selection[0].fetch('error').fetch('code')
    assert_equal 'ENTITY_HAS_NO_SUPPORTED_PERSISTENT_ID', selection[1].fetch('error').fetch('code')
  end
end
