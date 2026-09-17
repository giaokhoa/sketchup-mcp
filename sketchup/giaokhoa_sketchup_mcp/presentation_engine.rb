# frozen_string_literal: true

require 'fileutils'

module Giaokhoa
  module SketchupMcp
    class MutationEngine
      PRESENTATION_MM_PER_INCH = 25.4

      def model_bounds(_payload)
        model, snapshot = @model_state.capture
        bounds = model.bounds
        if bounds.empty?
          zero = {'x' => 0.0, 'y' => 0.0, 'z' => 0.0}
          return ok(
            'session_id' => @session_id,
            'model_guid' => snapshot.fetch(:guid),
            'revision' => snapshot.fetch(:revision),
            'empty' => true,
            'min_mm' => zero,
            'max_mm' => zero,
            'size_mm' => zero,
            'center_mm' => zero
          )
        end

        min = bounds.min
        max = bounds.max
        center = bounds.center
        ok(
          'session_id' => @session_id,
          'model_guid' => snapshot.fetch(:guid),
          'revision' => snapshot.fetch(:revision),
          'empty' => false,
          'min_mm' => point_mm_hash(min),
          'max_mm' => point_mm_hash(max),
          'size_mm' => {
            'x' => inches_to_mm(max.x - min.x),
            'y' => inches_to_mm(max.y - min.y),
            'z' => inches_to_mm(max.z - min.z)
          },
          'center_mm' => point_mm_hash(center)
        )
      end

      def create_section_plane(payload)
        context, result = prepare_request(
          'section_plane.create',
          payload,
          %w[mutation point_mm normal name symbol],
          'SketchUp MCP: Create Section Plane'
        )
        return result if result

        point = point3d_from_mm(payload['point_mm'])
        normal = direction3(payload['normal'])
        return invalid('point_mm values are invalid') unless point
        return invalid('normal values are invalid') unless normal

        name = payload['name']
        symbol = payload['symbol']
        unless name.is_a?(String) && !name.strip.empty? && name.bytesize <= 128
          return invalid('name must be a non-empty string up to 128 bytes')
        end
        unless symbol.is_a?(String) && symbol.bytesize <= 3
          return invalid('symbol must be a string up to 3 bytes')
        end

        perform_operation(context, 'SketchUp MCP: Create Section Plane') do
          section_plane = context.model.entities.add_section_plane(point, normal)
          raise OperationFailure, 'failed to create section plane' unless section_plane

          section_plane.name = name
          section_plane.symbol = symbol unless symbol.empty?

          lambda do |post_snapshot|
            {
              'entity_ref' => reference_for(section_plane, post_snapshot),
              'name' => section_plane.name.to_s,
              'symbol' => section_plane.symbol.to_s
            }
          end
        end
      end

      def create_scene(payload)
        context, result = prepare_request(
          'scene.create',
          payload,
          %w[mutation name eye_mm target_mm up perspective orthographic_height_mm fov_degrees section_plane_ref display_section_plane],
          'SketchUp MCP: Create Scene'
        )
        return result if result

        name = payload['name']
        return invalid('name must be a non-empty string up to 128 bytes') unless name.is_a?(String) && !name.strip.empty? && name.bytesize <= 128

        eye = point3d_from_mm(payload['eye_mm'])
        target = point3d_from_mm(payload['target_mm'])
        up = direction3(payload['up'])
        return invalid('eye_mm values are invalid') unless eye
        return invalid('target_mm values are invalid') unless target
        return invalid('up values are invalid') unless up
        return invalid('eye_mm and target_mm must differ') if eye == target

        perspective = payload['perspective'] == true
        ortho_height = finite_number(payload['orthographic_height_mm'])
        fov = finite_number(payload['fov_degrees'])
        if perspective
          return invalid('fov_degrees must be between 1 and 120') unless fov && fov.between?(1.0, 120.0)
          return invalid('orthographic_height_mm must be 0 for perspective') unless ortho_height == 0.0
        else
          return invalid('orthographic_height_mm must be positive') unless ortho_height&.positive?
          return invalid('fov_degrees must be 0 for orthographic') unless fov == 0.0
        end

        section_plane = nil
        if payload['section_plane_ref']
          section_plane, section_error = resolve_section_plane(context, payload['section_plane_ref'])
          return section_error if section_error
        end

        perform_operation(context, 'SketchUp MCP: Create Scene') do
          entities = context.model.entities
          previous_section = entities.active_section_plane
          page = nil
          begin
            entities.active_section_plane = section_plane
            flags = PAGE_USE_CAMERA | PAGE_USE_SECTION_PLANES | PAGE_USE_RENDERING_OPTIONS
            page = context.model.pages.add(name, flags)
            raise OperationFailure, 'failed to create scene' unless page

            camera = page.camera
            camera.set(eye, target, up)
            camera.perspective = perspective
            if perspective
              camera.fov = fov
            else
              camera.height = presentation_mm_to_inches(ortho_height)
            end

            page.use_camera = true
            page.use_section_planes = true
            page.use_rendering_options = true
            rendering = page.rendering_options
            rendering['DisplaySectionCuts'] = !section_plane.nil?
            rendering['DisplaySectionPlanes'] = payload['display_section_plane'] == true
          ensure
            entities.active_section_plane = previous_section
          end

          lambda do |_post_snapshot|
            {
              'name' => page.name.to_s,
              'index' => context.model.pages.to_a.index(page)
            }
          end
        end
      end

      def save_model_copy(payload)
        context, result = prepare_request(
          'model.file.save_copy',
          payload,
          %w[mutation output_path],
          'SketchUp MCP: Save Model Copy'
        )
        return result if result

        output_path = payload['output_path']
        unless output_path.is_a?(String) && !output_path.strip.empty? && File.extname(output_path).downcase == '.skp'
          return invalid('output_path must be a .skp path')
        end
        return error('FILE_EXISTS', 'output_path already exists') if File.exist?(output_path)

        perform_model_file_operation(context, 'model.file.save_copy') do
          FileUtils.mkdir_p(File.dirname(output_path))
          saved = context.model.save_copy(output_path)
          raise OperationFailure, 'SketchUp save_copy failed' unless saved && File.exist?(output_path)

          {'output_path' => output_path}
        end
      end

      private

      def resolve_section_plane(context, ref)
        required = %w[session_id model_guid persistent_id revision]
        unless ref.is_a?(Hash) && ref.keys.sort == required.sort
          return [nil, error('INVALID_ENTITY_REFERENCE', 'section_plane_ref fields are invalid')]
        end
        unless ref['session_id'] == @session_id &&
               ref['model_guid'] == context.snapshot.fetch(:guid) &&
               ref['revision'] == context.snapshot.fetch(:revision)
          return [nil, error('INVALID_ENTITY_REFERENCE', 'section_plane_ref must match the current mutation revision')]
        end

        entity = context.model.find_entity_by_persistent_id(ref['persistent_id'])
        unless entity && entity.is_a?(Sketchup::SectionPlane)
          return [nil, error('ENTITY_TYPE_NOT_SUPPORTED', 'section_plane_ref must reference a SectionPlane')]
        end
        [entity, nil]
      end

      def perform_model_file_operation(context, operation)
        mark_running(context)
        begin
          payload = yield
          post_snapshot = @model_state.snapshot
          unless post_snapshot.fetch(:guid) == context.snapshot.fetch(:guid) &&
                 post_snapshot.fetch(:revision) == context.snapshot.fetch(:revision)
            result = error(
              'STALE_REVISION',
              'SketchUp model changed during file operation',
              'operation' => operation,
              'expected_revision' => context.snapshot.fetch(:revision),
              'actual_revision' => post_snapshot.fetch(:revision)
            )
            return record_terminal(context, :failed, result)
          end
          record_terminal(context, :completed, success_result(context, post_snapshot, payload))
        rescue StandardError => e
          result = error(
            'SKETCHUP_OPERATION_FAILED',
            'SketchUp file operation failed',
            'operation' => operation,
            'reason' => e.message.to_s
          )
          record_terminal(context, :failed, result)
        end
      end

      def point3d_from_mm(value)
        return nil unless value.is_a?(Hash) && value.keys.sort == %w[x y z]

        numbers = %w[x y z].map { |key| finite_number(value[key]) }
        return nil unless numbers.all?

        Geom::Point3d.new(*numbers.map { |number| presentation_mm_to_inches(number) })
      end

      def direction3(value)
        return nil unless value.is_a?(Hash) && value.keys.sort == %w[x y z]

        numbers = %w[x y z].map { |key| finite_number(value[key]) }
        return nil unless numbers.all?
        return nil if numbers.all?(&:zero?)

        Geom::Vector3d.new(*numbers)
      end

      def point_mm_hash(point)
        {
          'x' => inches_to_mm(point.x),
          'y' => inches_to_mm(point.y),
          'z' => inches_to_mm(point.z)
        }
      end

      def presentation_mm_to_inches(value)
        value.to_f / PRESENTATION_MM_PER_INCH
      end

      def inches_to_mm(value)
        value.to_f * PRESENTATION_MM_PER_INCH
      end
    end
  end
end
