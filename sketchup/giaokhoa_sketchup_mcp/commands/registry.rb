# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    module Commands
      class Registry
        OPERATIONS = [
          'system.ping',
          'session.info',
          'model.summary',
          'model.bounds',
          'selection.get',
          'entity.inspect',
          'entity.children.list',
          'entity.translate',
          'entity.delete',
          'entity.material.set',
          'entity.name.set',
          'assembly.create',
          'geometry.create_box',
          'section_plane.create',
          'scene.create',
          'model.file.save_copy',
          'layout.document.create',
          'layout.viewport.add',
          'layout.dimension.add',
          'layout.text.add',
          'layout.line.add',
          'layout.rectangle.add',
          'layout.export',
          'changes.undo'
        ].freeze
        MAX_SELECTION_ENTITIES = 100
        MAX_CHILD_ENTITIES = 100
        SUPPORTED_ENTITY_TYPES = ['Group', 'ComponentInstance'].freeze
        LENGTH_UNITS = {
          0 => 'inches', 1 => 'feet', 2 => 'millimeters',
          3 => 'centimeters', 4 => 'meters', 5 => 'yards'
        }.freeze
        LENGTH_FORMATS = {
          0 => 'decimal', 1 => 'architectural', 2 => 'engineering', 3 => 'fractional'
        }.freeze

        def initialize(session_id:, model_state:, mutation_engine: nil)
          @session_id = session_id
          @model_state = model_state
          @mutation_engine = mutation_engine || MutationEngine.new(
            session_id: session_id,
            model_state: model_state
          )
        end

        def call(operation, payload)
          return invalid('payload must be an object') unless payload.is_a?(Hash)

          case operation
          when 'system.ping'
            return invalid('payload must be an empty object') unless payload.empty?
            ok('pong' => true)
          when 'session.info'
            return invalid('payload must be an empty object') unless payload.empty?
            session_info
          when 'model.summary'
            return invalid('payload must be an empty object') unless payload.empty?
            model_summary
          when 'model.bounds'
            return invalid('payload must be an empty object') unless payload.empty?
            @mutation_engine.model_bounds(payload)
          when 'selection.get'
            return invalid('payload must be an empty object') unless payload.empty?
            selection_get
          when 'entity.inspect'
            entity_inspect(payload)
          when 'entity.children.list'
            entity_children_list(payload)
          when 'entity.translate'
            @mutation_engine.translate(payload)
          when 'entity.delete'
            @mutation_engine.delete(payload)
          when 'entity.material.set'
            @mutation_engine.set_material(payload)
          when 'entity.name.set'
            @mutation_engine.set_name(payload)
          when 'assembly.create'
            @mutation_engine.create_assembly(payload)
          when 'geometry.create_box'
            @mutation_engine.create_box(payload)
          when 'section_plane.create'
            @mutation_engine.create_section_plane(payload)
          when 'scene.create'
            @mutation_engine.create_scene(payload)
          when 'model.file.save_copy'
            @mutation_engine.save_model_copy(payload)
          when 'layout.document.create'
            @mutation_engine.create_layout_document(payload)
          when 'layout.viewport.add'
            @mutation_engine.add_layout_viewport(payload)
          when 'layout.dimension.add'
            @mutation_engine.add_layout_dimension(payload)
          when 'layout.text.add'
            @mutation_engine.add_layout_text(payload)
          when 'layout.line.add'
            @mutation_engine.add_layout_line(payload)
          when 'layout.rectangle.add'
            @mutation_engine.add_layout_rectangle(payload)
          when 'layout.export'
            @mutation_engine.export_layout_document(payload)
          when 'changes.undo'
            @mutation_engine.undo(payload)
          else
            invalid("unsupported operation: #{operation}")
          end
        end

        private

        def session_info
          snapshot = @model_state.snapshot
          ok(
            'session_id' => @session_id,
            'pid' => Process.pid,
            'sketchup_version' => Sketchup.version,
            'model' => {
              'guid' => snapshot.fetch(:guid),
              'title' => snapshot.fetch(:title),
              'revision' => snapshot.fetch(:revision)
            }
          )
        end

        def model_summary
          model, snapshot = @model_state.capture
          path = model.active_path || []
          edit_path = path.map do |entity|
            ref, = reference_for(entity, snapshot)
            {
              'ref' => ref,
              'type' => entity.typename.to_s,
              'name' => entity_name(entity)
            }
          end

          units = model.options['UnitsOptions']
          unit_code = integer_option(units, 'LengthUnit')
          format_code = integer_option(units, 'LengthFormat')
          precision = integer_option(units, 'LengthPrecision')

          ok(
            'session_id' => @session_id,
            'model_guid' => snapshot.fetch(:guid),
            'revision' => snapshot.fetch(:revision),
            'title' => snapshot.fetch(:title),
            'path' => snapshot.fetch(:path),
            'units' => {
              'length_unit' => LENGTH_UNITS.fetch(unit_code, "unknown(#{unit_code})"),
              'length_unit_code' => unit_code,
              'length_format' => LENGTH_FORMATS.fetch(format_code, "unknown(#{format_code})"),
              'length_format_code' => format_code,
              'length_precision' => precision
            },
            'active_edit_context' => {
              'depth' => path.length,
              'path' => edit_path,
              'edit_transform' => transformation_array(model.edit_transform)
            },
            'counts' => {
              'top_level_entities' => model.entities.length,
              'definitions' => model.definitions.length,
              'materials' => model.materials.length,
              'tags' => model.layers.length,
              'selection' => model.selection.length
            }
          )
        end

        def selection_get
          model, snapshot = @model_state.capture
          entities = []
          model.selection.each do |entity|
            break if entities.length >= MAX_SELECTION_ENTITIES

            ref, ref_error = reference_for(entity, snapshot)
            entities << {
              'ref' => ref,
              'type' => entity.typename.to_s,
              'name' => entity_name(entity),
              'bounds' => bounds_for(entity),
              'error' => ref_error
            }
          end
          selected_count = model.selection.length
          ok(
            'session_id' => @session_id,
            'model_guid' => snapshot.fetch(:guid),
            'revision' => snapshot.fetch(:revision),
            'selected_count' => selected_count,
            'returned_count' => entities.length,
            'truncated' => selected_count > entities.length,
            'entities' => entities
          )
        end

        def entity_inspect(payload)
          required = %w[session_id model_guid persistent_id revision]
          return invalid('EntityRef fields are invalid') unless payload.keys.sort == required.sort
          return error('SESSION_NOT_FOUND', 'EntityRef session does not match this bridge session') unless payload['session_id'] == @session_id
          return invalid('persistent_id must be a positive integer') unless payload['persistent_id'].is_a?(Integer) && payload['persistent_id'].positive?
          return invalid('revision must be a non-negative integer') unless payload['revision'].is_a?(Integer) && payload['revision'] >= 0

          model, snapshot = @model_state.capture
          if payload['model_guid'] != snapshot.fetch(:guid)
            return error(
              'MODEL_CHANGED',
              'EntityRef model_guid no longer matches the active model identity epoch',
              'referenced_model_guid' => payload['model_guid'],
              'current_model_guid' => snapshot.fetch(:guid),
              'current_revision' => snapshot.fetch(:revision)
            )
          end

          entity = model.find_entity_by_persistent_id(payload['persistent_id'])
          return error('ENTITY_NOT_FOUND', 'No entity exists for the requested persistent_id') unless entity
          return unsupported_type(entity) unless supported_entity?(entity)

          ref, ref_error = reference_for(entity, snapshot)
          return {ok: false, error: ref_error} if ref_error

          details = {
            'ref' => ref,
            'type' => entity.typename.to_s,
            'name' => entity_name(entity),
            'bounds' => bounds_for(entity),
            'transformation' => transformation_array(entity.transformation),
            'material' => entity.material&.name.to_s,
            'tag' => entity.layer&.name.to_s,
            'definition' => definition_for(entity)
          }

          ok(
            'current_revision' => snapshot.fetch(:revision),
            'revision_mismatch' => payload['revision'] != snapshot.fetch(:revision),
            'entity' => details
          )
        end

        def entity_children_list(payload)
          required = %w[session_id model_guid persistent_id revision]
          return invalid('EntityRef fields are invalid') unless payload.keys.sort == required.sort
          return error('SESSION_NOT_FOUND', 'EntityRef session does not match this bridge session') unless payload['session_id'] == @session_id
          return invalid('persistent_id must be a positive integer') unless payload['persistent_id'].is_a?(Integer) && payload['persistent_id'].positive?
          return invalid('revision must be a non-negative integer') unless payload['revision'].is_a?(Integer) && payload['revision'] >= 0

          model, snapshot = @model_state.capture
          if payload['model_guid'] != snapshot.fetch(:guid)
            return error(
              'MODEL_CHANGED',
              'EntityRef model_guid no longer matches the active model identity epoch',
              'referenced_model_guid' => payload['model_guid'],
              'current_model_guid' => snapshot.fetch(:guid),
              'current_revision' => snapshot.fetch(:revision)
            )
          end

          entity = model.find_entity_by_persistent_id(payload['persistent_id'])
          return error('ENTITY_NOT_FOUND', 'No entity exists for the requested persistent_id') unless entity
          return unsupported_type(entity) unless supported_entity?(entity)

          child_entities = if entity.typename.to_s == 'Group'
                             entity.entities
                           elsif entity.respond_to?(:definition) && entity.definition
                             entity.definition.entities
                           end
          return error('ENTITY_TYPE_NOT_SUPPORTED', 'Entity does not expose child entities') unless child_entities

          children = []
          supported_count = 0
          child_entities.each do |child|
            next unless supported_entity?(child)

            supported_count += 1
            next if children.length >= MAX_CHILD_ENTITIES

            ref, ref_error = reference_for(child, snapshot)
            children << {
              'ref' => ref,
              'type' => child.typename.to_s,
              'name' => entity_name(child),
              'material' => child.respond_to?(:material) ? child.material&.name.to_s : '',
              'error' => ref_error
            }
          end

          ok(
            'current_revision' => snapshot.fetch(:revision),
            'revision_mismatch' => payload['revision'] != snapshot.fetch(:revision),
            'raw_entity_count' => child_entities.length,
            'supported_child_count' => supported_count,
            'returned_count' => children.length,
            'truncated' => supported_count > children.length,
            'children' => children
          )
        end

        def reference_for(entity, snapshot)
          return [nil, unsupported_type_error(entity)] unless supported_entity?(entity)

          persistent_id = supported_persistent_id(entity)
          unless persistent_id
            return [
              nil,
              error_payload(
                'ENTITY_HAS_NO_SUPPORTED_PERSISTENT_ID',
                'Entity does not expose a positive persistent_id'
              )
            ]
          end

          [
            {
              'session_id' => @session_id,
              'model_guid' => snapshot.fetch(:guid),
              'persistent_id' => persistent_id,
              'revision' => snapshot.fetch(:revision)
            },
            nil
          ]
        end

        def supported_entity?(entity)
          SUPPORTED_ENTITY_TYPES.include?(entity.typename.to_s)
        end

        def supported_persistent_id(entity)
          return nil unless entity.respond_to?(:persistent_id)

          value = entity.persistent_id
          value if value.is_a?(Integer) && value.positive?
        rescue StandardError
          nil
        end

        def unsupported_type(entity)
          {ok: false, error: unsupported_type_error(entity)}
        end

        def unsupported_type_error(entity)
          error_payload(
            'ENTITY_TYPE_NOT_SUPPORTED',
            "Entity type #{entity.typename} is not supported by entity.inspect",
            'entity_type' => entity.typename.to_s,
            'supported_types' => SUPPORTED_ENTITY_TYPES
          )
        end

        def entity_name(entity)
          name = entity.respond_to?(:name) ? entity.name.to_s : ''
          return name unless name.empty?
          return entity.definition.name.to_s if entity.respond_to?(:definition) && entity.definition

          ''
        end

        def definition_for(entity)
          return nil unless entity.respond_to?(:definition) && entity.definition

          {
            'name' => entity.definition.name.to_s,
            'entity_count' => entity.definition.entities.length
          }
        end

        def bounds_for(entity)
          return nil unless entity.respond_to?(:bounds)

          box = entity.bounds
          {
            'min' => point_array(box.min),
            'max' => point_array(box.max)
          }
        rescue StandardError
          nil
        end

        def point_array(point)
          point.to_a.map(&:to_f)
        end

        def transformation_array(transform)
          transform.to_a.map(&:to_f)
        end

        def integer_option(provider, key)
          provider[key].to_i
        end

        def ok(payload)
          {ok: true, payload: payload}
        end

        def invalid(message)
          error('INVALID_REQUEST', message)
        end

        def error(code, message, details = nil)
          {ok: false, error: error_payload(code, message, details)}
        end

        def error_payload(code, message, details = nil)
          value = {'code' => code, 'message' => message}
          value['details'] = details if details
          value
        end
      end
    end
  end
end
