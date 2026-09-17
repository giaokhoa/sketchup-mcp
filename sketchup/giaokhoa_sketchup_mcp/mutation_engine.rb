# frozen_string_literal: true

require 'digest'
require 'json'

module Giaokhoa
  module SketchupMcp
    class MutationEngine
      SUPPORTED_ENTITY_TYPES = ['Group', 'ComponentInstance'].freeze
      MUTATION_KEYS = %w[operation_id expected_model_guid expected_revision undo_label].freeze
      Context = Struct.new(:model, :snapshot, :operation_id, :fingerprint, keyword_init: true)

      class OperationFailure < StandardError; end

      def initialize(session_id:, model_state:)
        @session_id = session_id
        @model_state = model_state
        @cache_model_guid = nil
        @records = {}
      end

      def translate(payload)
        context, result = prepare_request(
          'entity.translate',
          payload,
          %w[mutation entity_ref translation_inches],
          'SketchUp MCP: Translate'
        )
        return result if result

        entity, entity_error = resolve_entity(context, payload['entity_ref'])
        return entity_error if entity_error

        translation = vector3(payload['translation_inches'])
        return error('INVALID_TRANSFORM', 'translation_inches must contain finite x, y, z values') unless translation
        if translation.all?(&:zero?)
          return error('INVALID_TRANSFORM', 'translation must move the entity by a non-zero distance')
        end

        perform_operation(context, 'SketchUp MCP: Translate') do
          transform = Geom::Transformation.translation(translation)
          transformed = entity.transform!(transform)
          raise OperationFailure, 'entity transform failed' if transformed == false || transformed.nil?

          lambda do |post_snapshot|
            {'entity_ref' => reference_for(entity, post_snapshot)}
          end
        end
      end

      def delete(payload)
        context, result = prepare_request(
          'entity.delete',
          payload,
          %w[mutation entity_ref],
          'SketchUp MCP: Delete Entity'
        )
        return result if result

        entity, entity_error = resolve_entity(context, payload['entity_ref'])
        return entity_error if entity_error
        if active_path_contains?(context.model, entity)
          return error(
            'LOCKED_ENTITY_OR_CONTEXT',
            'target entity is part of the active edit context'
          )
        end

        persistent_id = entity.persistent_id
        perform_operation(context, 'SketchUp MCP: Delete Entity') do
          entity.erase!

          lambda do |_post_snapshot|
            {'deleted_persistent_id' => persistent_id}
          end
        end
      end

      def set_material(payload)
        context, result = prepare_request(
          'entity.material.set',
          payload,
          %w[mutation entity_ref material],
          'SketchUp MCP: Set Material'
        )
        return result if result

        entity, entity_error = resolve_entity(context, payload['entity_ref'])
        return entity_error if entity_error

        material_spec = parse_material(payload['material'])
        return invalid('material must contain a bounded name and RGB channels from 0 through 255') unless material_spec

        name, red, green, blue = material_spec
        materials = context.model.materials
        existing = materials[name]
        if existing
          if existing.respond_to?(:texture) && existing.texture
            return error(
              'MATERIAL_NAME_CONFLICT',
              'existing material with this name is textured'
            )
          end
          color = existing.color
          unless color && color.red == red && color.green == green && color.blue == blue
            return error(
              'MATERIAL_NAME_CONFLICT',
              'existing material with this name has a different color'
            )
          end
        end

        perform_operation(context, 'SketchUp MCP: Set Material') do
          material = existing || materials.add(name)
          raise OperationFailure, 'failed to create material' unless material

          material.color = Sketchup::Color.new(red, green, blue) unless existing
          assigned = entity.material = material
          raise OperationFailure, 'failed to assign material' if assigned.nil?

          lambda do |post_snapshot|
            {
              'entity_ref' => reference_for(entity, post_snapshot),
              'material' => {
                'name' => material.name.to_s,
                'color' => {'r' => red, 'g' => green, 'b' => blue}
              }
            }
          end
        end
      end

      def set_name(payload)
        context, result = prepare_request(
          'entity.name.set',
          payload,
          %w[mutation entity_ref name],
          'SketchUp MCP: Set Entity Name'
        )
        return result if result

        entity, entity_error = resolve_entity(context, payload['entity_ref'])
        return entity_error if entity_error

        name = payload['name']
        unless name.is_a?(String) && !name.strip.empty? && name.bytesize <= 128
          return invalid('name must be a non-empty string up to 128 bytes')
        end
        if entity.respond_to?(:name) && entity.name.to_s == name
          return invalid('name must differ from the current entity name')
        end

        perform_operation(context, 'SketchUp MCP: Set Entity Name') do
          entity.name = name
          unless entity.respond_to?(:name) && entity.name.to_s == name
            raise OperationFailure, 'entity name was not applied'
          end

          lambda do |post_snapshot|
            {
              'entity_ref' => reference_for(entity, post_snapshot),
              'name' => entity.name.to_s
            }
          end
        end
      end

      def create_assembly(payload)
        context, result = prepare_request(
          'assembly.create',
          payload,
          %w[mutation name children],
          'SketchUp MCP: Create Assembly'
        )
        return result if result

        name = payload['name']
        unless name.is_a?(String) && !name.strip.empty? && name.bytesize <= 128
          return invalid('name must be a non-empty string up to 128 bytes')
        end

        child_refs = payload['children']
        unless child_refs.is_a?(Array) && child_refs.length.between?(2, 100)
          return invalid('children must contain between 2 and 100 entity references')
        end

        entities = []
        persistent_ids = []
        child_refs.each do |ref|
          entity, entity_error = resolve_entity(context, ref)
          return entity_error if entity_error

          pid = entity.persistent_id
          return invalid('children persistent_id values must be unique') if persistent_ids.include?(pid)

          entities << entity
          persistent_ids << pid
        end

        parent = entities.first.parent
        unless parent && entities.all? { |entity| entity.parent.equal?(parent) }
          return error(
            'ASSEMBLY_PARENT_MISMATCH',
            'all assembly children must share the same SketchUp parent'
          )
        end

        if parent.respond_to?(:instances) && parent.instances.respond_to?(:length) &&
           parent.instances.length > 1
          return error(
            'ASSEMBLY_SHARED_DEFINITION',
            'cannot regroup entities inside a shared component definition'
          )
        end

        if entities.any? { |entity| active_path_contains?(context.model, entity) }
          return error(
            'LOCKED_ENTITY_OR_CONTEXT',
            'assembly child is part of the active edit context'
          )
        end

        parent_entities = parent.respond_to?(:entities) ? parent.entities : nil
        return error('ASSEMBLY_PARENT_MISMATCH', 'assembly parent does not expose an Entities collection') unless parent_entities

        perform_operation(context, 'SketchUp MCP: Create Assembly') do
          group = parent_entities.add_group(entities)
          raise OperationFailure, 'failed to create assembly group' unless group

          group.name = name
          unless group.respond_to?(:name) && group.name.to_s == name
            raise OperationFailure, 'assembly name was not applied'
          end

          lambda do |post_snapshot|
            refreshed_children = persistent_ids.map do |persistent_id|
              child = context.model.find_entity_by_persistent_id(persistent_id)
              raise OperationFailure, "assembly child #{persistent_id} no longer exists" unless child

              reference_for(child, post_snapshot)
            end

            {
              'name' => group.name.to_s,
              'assembly_ref' => reference_for(group, post_snapshot),
              'children' => refreshed_children
            }
          end
        end
      end

      def create_box(payload)
        context, result = prepare_request(
          'geometry.create_box',
          payload,
          %w[mutation origin_inches dimensions_inches],
          'SketchUp MCP: Create Box'
        )
        return result if result
        return context_locked if active_context_locked?(context.model)

        origin = vector3(payload['origin_inches'])
        return error('INVALID_DIMENSIONS', 'origin_inches must contain finite x, y, z values') unless origin

        dimensions = dimensions3(payload['dimensions_inches'])
        unless dimensions
          return error('INVALID_DIMENSIONS', 'dimensions_inches must contain positive finite width, depth, height values')
        end
        width, depth, height = dimensions

        perform_operation(context, 'SketchUp MCP: Create Box') do
          group = context.model.active_entities.add_group
          raise OperationFailure, 'failed to create group' unless group

          points = [
            [0.0, 0.0, 0.0],
            [width, 0.0, 0.0],
            [width, depth, 0.0],
            [0.0, depth, 0.0]
          ]
          face = group.entities.add_face(points)
          raise OperationFailure, 'failed to create box base face' unless face

          face.reverse! if face.normal.respond_to?(:z) && face.normal.z.negative?
          face.pushpull(height)
          unless origin.all?(&:zero?)
            group.transform!(Geom::Transformation.translation(origin))
          end

          lambda do |post_snapshot|
            {'entity_ref' => reference_for(group, post_snapshot)}
          end
        end
      end

      def undo(payload)
        context, result = prepare_request(
          'changes.undo',
          payload,
          %w[mutation],
          'SketchUp MCP: Undo'
        )
        return result if result

        mark_running(context)
        begin
          Sketchup.undo
          post_snapshot = @model_state.snapshot
          unless revision_advanced?(context.snapshot, post_snapshot)
            result = error(
              'SKETCHUP_OPERATION_FAILED',
              'undo did not change the model revision',
              'operation' => 'changes.undo'
            )
            return record_terminal(context, :failed, result)
          end

          result = success_result(context, post_snapshot, {})
          record_terminal(context, :completed, result)
        rescue StandardError => e
          result = operation_failure('changes.undo', e)
          record_terminal(context, :failed, result)
        end
      end

      private

      def prepare_request(operation, payload, expected_keys, expected_undo_label)
        return [nil, invalid('payload must be an object')] unless payload.is_a?(Hash)
        unless payload.keys.sort == expected_keys.sort
          return [nil, invalid("payload fields must be exactly: #{expected_keys.sort.join(', ')}")]
        end

        mutation = payload['mutation']
        unless mutation.is_a?(Hash) && mutation.keys.sort == MUTATION_KEYS.sort
          return [nil, invalid("mutation fields must be exactly: #{MUTATION_KEYS.sort.join(', ')}")]
        end

        operation_id = mutation['operation_id']
        unless operation_id.is_a?(String) && !operation_id.strip.empty? && operation_id.bytesize <= 128
          return [nil, invalid('operation_id must be a non-empty string up to 128 bytes')]
        end

        expected_model_guid = mutation['expected_model_guid']
        unless expected_model_guid.is_a?(String) && !expected_model_guid.empty?
          return [nil, invalid('expected_model_guid is required')]
        end

        expected_revision = mutation['expected_revision']
        unless expected_revision.is_a?(Integer) && expected_revision >= 0
          return [nil, invalid('expected_revision must be a non-negative integer')]
        end

        unless mutation['undo_label'] == expected_undo_label
          return [nil, invalid("undo_label must be #{expected_undo_label.inspect}")]
        end

        model, snapshot = @model_state.capture
        sync_cache_epoch(snapshot.fetch(:guid))
        if expected_model_guid != snapshot.fetch(:guid)
          return [
            nil,
            error(
              'MODEL_CHANGED',
              'expected model identity no longer matches the active model',
              'expected_model_guid' => expected_model_guid,
              'current_model_guid' => snapshot.fetch(:guid),
              'current_revision' => snapshot.fetch(:revision)
            )
          ]
        end

        fingerprint = fingerprint_for(operation, payload)
        existing = @records[operation_id]
        if existing
          if existing.fetch(:fingerprint) != fingerprint
            return [
              nil,
              error(
                'OPERATION_ID_REUSE',
                'operation_id was already used with different parameters',
                'operation_id' => operation_id
              )
            ]
          end

          case existing.fetch(:state)
          when :running
            return [
              nil,
              error(
                'OPERATION_IN_PROGRESS',
                'operation with this operation_id is still in progress',
                'operation_id' => operation_id
              )
            ]
          when :completed, :failed
            return [nil, existing.fetch(:result)]
          end
        end

        if expected_revision != snapshot.fetch(:revision)
          return [
            nil,
            error(
              'STALE_REVISION',
              'model revision changed; refresh context before mutating',
              'expected_revision' => expected_revision,
              'actual_revision' => snapshot.fetch(:revision)
            )
          ]
        end

        [
          Context.new(
            model: model,
            snapshot: snapshot,
            operation_id: operation_id,
            fingerprint: fingerprint
          ),
          nil
        ]
      end

      def perform_operation(context, undo_label)
        mark_running(context)
        started = false
        begin
          started = context.model.start_operation(undo_label, true)
          raise OperationFailure, 'SketchUp refused to start the operation' unless started

          result_builder = yield
          committed = context.model.commit_operation
          raise OperationFailure, 'SketchUp refused to commit the operation' unless committed
          started = false

          post_snapshot = @model_state.snapshot
          unless revision_advanced?(context.snapshot, post_snapshot)
            result = error(
              'SKETCHUP_OPERATION_FAILED',
              'mutation committed but model revision did not advance',
              'operation' => undo_label
            )
            return record_terminal(context, :failed, result)
          end

          payload = result_builder.respond_to?(:call) ? result_builder.call(post_snapshot) : result_builder
          result = success_result(context, post_snapshot, payload)
          record_terminal(context, :completed, result)
        rescue StandardError => e
          abort_quietly(context.model) if started
          result = operation_failure(undo_label, e)
          record_terminal(context, :failed, result)
        end
      end

      def resolve_entity(context, ref)
        unless ref.is_a?(Hash) && ref.keys.sort == %w[model_guid persistent_id revision session_id].sort
          return [nil, error('INVALID_ENTITY_REFERENCE', 'entity_ref fields are invalid')]
        end
        unless ref['session_id'] == @session_id &&
               ref['model_guid'] == context.snapshot.fetch(:guid) &&
               ref['revision'] == context.snapshot.fetch(:revision)
          return [
            nil,
            error(
              'INVALID_ENTITY_REFERENCE',
              'entity_ref must match the mutation session, model, and expected revision'
            )
          ]
        end
        unless ref['persistent_id'].is_a?(Integer) && ref['persistent_id'].positive?
          return [nil, error('INVALID_ENTITY_REFERENCE', 'entity_ref persistent_id must be positive')]
        end

        entity = context.model.find_entity_by_persistent_id(ref['persistent_id'])
        unless entity && (!entity.respond_to?(:valid?) || entity.valid?)
          return [nil, error('ENTITY_DELETED', 'target entity no longer exists')]
        end

        type = entity.typename.to_s
        unless SUPPORTED_ENTITY_TYPES.include?(type)
          return [
            nil,
            error(
              'ENTITY_TYPE_NOT_SUPPORTED',
              "entity type #{type} is not supported by mutations",
              'entity_type' => type,
              'supported_types' => SUPPORTED_ENTITY_TYPES
            )
          ]
        end
        if entity.respond_to?(:locked?) && entity.locked?
          return [nil, error('LOCKED_ENTITY_OR_CONTEXT', 'target entity is locked')]
        end
        return [nil, context_locked] if active_context_locked?(context.model)

        [entity, nil]
      end

      def active_context_locked?(model)
        path = model.respond_to?(:active_path) ? (model.active_path || []) : []
        path.any? { |entity| entity.respond_to?(:locked?) && entity.locked? }
      end

      def context_locked
        error('LOCKED_ENTITY_OR_CONTEXT', 'active edit context is locked')
      end

      def reference_for(entity, snapshot)
        persistent_id = entity.respond_to?(:persistent_id) ? entity.persistent_id : nil
        unless persistent_id.is_a?(Integer) && persistent_id.positive?
          raise OperationFailure, 'created or mutated entity has no persistent_id'
        end

        {
          'session_id' => @session_id,
          'model_guid' => snapshot.fetch(:guid),
          'persistent_id' => persistent_id,
          'revision' => snapshot.fetch(:revision)
        }
      end

      def mark_running(context)
        @records[context.operation_id] = {
          fingerprint: context.fingerprint,
          state: :running
        }
      end

      def record_terminal(context, state, result)
        @records[context.operation_id] = {
          fingerprint: context.fingerprint,
          state: state,
          result: result
        }
        result
      end

      def success_result(context, snapshot, payload)
        ok(
          {
            'operation_id' => context.operation_id,
            'model_guid' => snapshot.fetch(:guid),
            'revision' => snapshot.fetch(:revision)
          }.merge(payload)
        )
      end

      def revision_advanced?(before_snapshot, after_snapshot)
        before_snapshot.fetch(:guid) == after_snapshot.fetch(:guid) &&
          after_snapshot.fetch(:revision) > before_snapshot.fetch(:revision)
      end

      def abort_quietly(model)
        model.abort_operation
      rescue StandardError
        nil
      end

      def operation_failure(operation, exception)
        details = {'operation' => operation}
        details['reason'] = exception.message.to_s unless exception.message.to_s.empty?
        error('SKETCHUP_OPERATION_FAILED', 'SketchUp operation failed', details)
      end

      def sync_cache_epoch(model_guid)
        return if @cache_model_guid == model_guid

        @cache_model_guid = model_guid
        @records.clear
      end

      def fingerprint_for(operation, payload)
        material = payload.dup
        mutation = payload.fetch('mutation').dup
        mutation.delete('operation_id')
        material['mutation'] = mutation
        Digest::SHA256.hexdigest(JSON.generate([operation, canonicalize(material)]))
      end

      def canonicalize(value)
        case value
        when Hash
          value.keys.sort.each_with_object({}) do |key, result|
            result[key] = canonicalize(value.fetch(key))
          end
        when Array
          value.map { |item| canonicalize(item) }
        else
          value
        end
      end

      def active_path_contains?(model, entity)
        path = model.respond_to?(:active_path) ? (model.active_path || []) : []
        path.include?(entity)
      end

      def parse_material(value)
        return nil unless value.is_a?(Hash) && value.keys.sort == %w[color name]
        name = value['name']
        return nil unless name.is_a?(String) && !name.strip.empty? && name.bytesize <= 128

        color = value['color']
        return nil unless color.is_a?(Hash) && color.keys.sort == %w[b g r]

        channels = %w[r g b].map { |key| color[key] }
        return nil unless channels.all? { |channel| channel.is_a?(Integer) && channel.between?(0, 255) }

        [name, *channels]
      end

      def vector3(value)
        return nil unless value.is_a?(Hash) && value.keys.sort == %w[x y z]

        numbers = %w[x y z].map { |axis| finite_number(value[axis]) }
        numbers.all? ? numbers : nil
      end

      def dimensions3(value)
        return nil unless value.is_a?(Hash) && value.keys.sort == %w[depth height width]

        numbers = %w[width depth height].map { |axis| finite_number(value[axis]) }
        return nil unless numbers.all? && numbers.all?(&:positive?)

        numbers
      end

      def finite_number(value)
        return nil unless value.is_a?(Numeric)

        number = value.to_f
        number if number.finite?
      end

      def invalid(message)
        error('INVALID_REQUEST', message)
      end

      def ok(payload)
        {ok: true, payload: payload}
      end

      def error(code, message, details = nil)
        value = {'code' => code, 'message' => message}
        value['details'] = details if details
        {ok: false, error: value}
      end
    end
  end
end
