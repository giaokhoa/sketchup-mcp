# frozen_string_literal: true

require 'fileutils'

module Giaokhoa
  module SketchupMcp
    class DocumentationSnapshotBuilder
      Node = Struct.new(
        :entity, :name, :path, :pid_path, :bounds, :parent_transform,
        keyword_init: true
      )

      A3_WIDTH_MM = 420.0
      A3_HEIGHT_MM = 297.0
      MM_PER_INCH = 25.4

      def initialize(model:, output_directory:, base_name:)
        @model = model
        @output_directory = output_directory
        @base_name = base_name
        @root = nil
        @nodes = []
        @leaves = []
        @scenes = {}
      end

      def prepare!
        validate_source!
        FileUtils.mkdir_p(@output_directory)
        snapshot_path = File.join(@output_directory, "#{@base_name}.skp")
        raise "snapshot already exists: #{snapshot_path}" if File.exist?(snapshot_path)

        collect_model!
        essential = essential_parts
        create_documentation_scenes!(essential)

        {
          'snapshot_path' => snapshot_path,
          'spec' => build_spec(snapshot_path, essential)
        }
      end

      def save_snapshot!(prepared)
        saved = @model.save_copy(prepared.fetch('snapshot_path'))
        raise 'SketchUp save_copy failed for documentation snapshot' unless saved
        raise 'documentation snapshot file is missing' unless File.exist?(prepared.fetch('snapshot_path'))

        prepared
      end

      private

      def validate_source!
        raise 'documentation requires a saved SketchUp model' if @model.path.to_s.empty?

        supported = @model.entities.select { |entity| supported_entity?(entity) }
        raise 'documentation requires at least one top-level Group or ComponentInstance' if supported.empty?
      end

      def collect_model!
        roots = @model.entities.select { |entity| supported_entity?(entity) }
        @root = roots.find { |entity| entity_name(entity) == 'Cabinet - 1800' }
        @root ||= roots.max_by { |entity| bounds_volume(entity.bounds) }
        raise 'could not resolve documentation root assembly' unless @root

        @nodes = []
        @leaves = []
        walk(@root, [], Geom::Transformation.new)
        raise 'documentation root contains no supported leaf parts' if @leaves.empty?
      end

      def walk(entity, parent_path, parent_transform)
        path = parent_path + [entity]
        node = Node.new(
          entity: entity,
          name: entity_name(entity),
          path: path,
          pid_path: persistent_id_path(path),
          bounds: transformed_bounds(entity.bounds, parent_transform),
          parent_transform: parent_transform
        )
        @nodes << node

        children = supported_children(entity)
        if children.empty?
          @leaves << node
          return
        end

        world_transform = parent_transform * entity.transformation
        children.each { |child| walk(child, path, world_transform) }
      end

      def supported_children(entity)
        entities = if entity.typename.to_s == 'Group'
                     entity.entities
                   elsif entity.respond_to?(:definition) && entity.definition
                     entity.definition.entities
                   end
        return [] unless entities

        entities.select { |child| supported_entity?(child) }
      end

      def essential_parts
        top = require_leaf('Carcass - Top Slab')
        side_left = require_leaf('Carcass - Side Left')
        side_right = require_leaf('Carcass - Side Right')
        bottom = require_leaf('Carcass - Bottom')
        back = require_leaf('Carcass - Back Panel')
        doors = @leaves.select { |node| node.name.match?(/\ADoor - \d+\z/) }.sort_by { |node| min_x(node) }
        raise "expected four lower doors, found #{doors.length}" unless doors.length == 4

        drawer_fronts = @leaves.select { |node| node.name.match?(/\ADrawer (Left|Right) - Front Face/) }
                               .sort_by { |node| min_x(node) }
        raise "expected two drawer fronts, found #{drawer_fronts.length}" unless drawer_fronts.length == 2

        shelves = @leaves.select { |node| node.name.start_with?('Adjustable Shelf -') }.sort_by { |node| min_x(node) }
        raise "expected two adjustable shelves, found #{shelves.length}" unless shelves.length == 2

        legs = @leaves.select { |node| node.name.start_with?('Leg -') }
        raise "expected four legs, found #{legs.length}" unless legs.length == 4

        drawer_box_front = @leaves.find { |node| node.name.match?(/\ADrawer Left - Box Front/) }
        drawer_side = @leaves.find { |node| node.name.match?(/\ADrawer Left - Side Left/) }
        raise 'left drawer box boards are missing' unless drawer_box_front && drawer_side

        {
          root: @nodes.find { |node| node.entity.equal?(@root) },
          top: top,
          side_left: side_left,
          side_right: side_right,
          bottom: bottom,
          back: back,
          doors: doors,
          drawer_fronts: drawer_fronts,
          shelves: shelves,
          legs: legs,
          drawer_box_front: drawer_box_front,
          drawer_side: drawer_side
        }
      end

      def require_leaf(name)
        @leaves.find { |node| node.name == name } || raise("required part is missing: #{name}")
      end

      def create_documentation_scenes!(parts)
        remove_active_section
        rendering = @model.rendering_options
        rendering['DisplaySectionPlanes'] = false if rendering
        rendering['DisplaySectionCuts'] = true if rendering

        visual = parts.fetch(:root).bounds
        center = bounds_center(visual)
        width = extent_x(visual)
        depth = extent_y(visual)
        height = extent_z(visual)
        span = [width, depth, height].max

        @scenes[:plan] = create_scene(
          scene_name('PLAN'),
          [center.x, center.y, visual.max.z + span * 3.0],
          center.to_a,
          [0.0, 1.0, 0.0],
          false,
          [width, depth].max * 1.20
        )
        @scenes[:front] = create_scene(
          scene_name('FRONT'),
          [center.x, visual.min.y - span * 3.0, center.z],
          center.to_a,
          [0.0, 0.0, 1.0],
          false,
          height * 1.25
        )
        @scenes[:side] = create_scene(
          scene_name('SIDE'),
          [visual.max.x + span * 3.0, center.y, center.z],
          center.to_a,
          [0.0, 0.0, 1.0],
          false,
          height * 1.25
        )

        section_x = (min_x(parts.fetch(:drawer_fronts).first) + max_x(parts.fetch(:drawer_fronts).first)) / 2.0
        section_a = @model.entities.add_section_plane(
          [section_x, center.y, center.z],
          [-1.0, 0.0, 0.0]
        )
        raise 'failed to create section plane A-A' unless section_a
        section_a.name = "MCP-DOC-#{@base_name}-A-A" if section_a.respond_to?(:name=)
        section_a.activate
        @scenes[:section_a] = create_scene(
          scene_name('SECTION-A'),
          [visual.min.x - span * 3.0, center.y, center.z],
          [section_x, center.y, center.z],
          [0.0, 0.0, 1.0],
          false,
          height * 1.25,
          capture_section: true
        )

        remove_active_section
        shelf = parts.fetch(:shelves).first
        section_y = (min_y(shelf) + max_y(shelf)) / 2.0
        section_b = @model.entities.add_section_plane(
          [center.x, section_y, center.z],
          [0.0, -1.0, 0.0]
        )
        raise 'failed to create section plane B-B' unless section_b
        section_b.name = "MCP-DOC-#{@base_name}-B-B" if section_b.respond_to?(:name=)
        section_b.activate
        @scenes[:section_b] = create_scene(
          scene_name('SECTION-B'),
          [center.x, visual.min.y - span * 3.0, center.z],
          [center.x, section_y, center.z],
          [0.0, 0.0, 1.0],
          false,
          height * 1.25,
          capture_section: true
        )

        remove_active_section
        @scenes[:iso] = create_scene(
          scene_name('ISO'),
          [visual.max.x + width * 1.8, visual.min.y - depth * 2.4, visual.max.z + height * 1.5],
          center.to_a,
          [0.0, 0.0, 1.0],
          true,
          nil
        )
      end

      def create_scene(name, eye, target, up, perspective, camera_height, capture_section: false)
        page = @model.pages.add(name)
        raise "failed to create scene #{name}" unless page

        camera = page.camera
        camera.set(
          Geom::Point3d.new(*eye),
          Geom::Point3d.new(*target),
          Geom::Vector3d.new(*up)
        )
        camera.perspective = perspective
        camera.height = camera_height if !perspective && camera_height
        page.use_camera = true
        page.use_section_planes = capture_section
        page.use_rendering_options = true if page.respond_to?(:use_rendering_options=)

        flags = PAGE_USE_CAMERA
        flags |= PAGE_USE_SECTION_PLANES if capture_section
        flags |= PAGE_USE_RENDERING_OPTIONS if defined?(PAGE_USE_RENDERING_OPTIONS)
        page.update(flags)

        @model.pages.to_a.index(page) + 1
      end

      def build_spec(snapshot_path, parts)
        {
          'schema_version' => 1,
          'snapshot_path' => snapshot_path,
          'output_directory' => @output_directory,
          'base_name' => @base_name,
          'page_width_mm' => A3_WIDTH_MM,
          'page_height_mm' => A3_HEIGHT_MM,
          'root_bounds' => bounds_hash(parts.fetch(:root).bounds),
          'views' => paper_views,
          'dimensions' => dimension_specs(parts),
          'notes' => note_lines(parts)
        }
      end

      def paper_views
        [
          view('plan', 'MẶT BẰNG', @scenes.fetch(:plan), [0.55, 0.60, 4.70, 3.25], 1.0 / 15.0, false),
          view('front', 'MẶT ĐỨNG CHÍNH', @scenes.fetch(:front), [5.82, 0.55, 5.48, 3.40], 1.0 / 15.0, false),
          view('section_a', 'MẶT CẮT A-A', @scenes.fetch(:section_a), [11.92, 0.55, 4.10, 3.45], 1.0 / 10.0, false),
          view('section_b', 'MẶT CẮT B-B', @scenes.fetch(:section_b), [0.55, 5.95, 6.35, 3.75], 1.0 / 15.0, false),
          view('side', 'MẶT BÊN', @scenes.fetch(:side), [7.42, 5.95, 2.95, 3.80], 1.0 / 10.0, false),
          view('iso', 'PHỐI CẢNH', @scenes.fetch(:iso), [10.88, 5.80, 5.15, 4.05], 0.0, true)
        ]
      end

      def view(id, title, scene_index, rect, scale, perspective)
        {
          'id' => id,
          'title' => title,
          'scene_index' => scene_index,
          'rect' => {
            'left' => rect[0],
            'top' => rect[1],
            'right' => rect[0] + rect[2],
            'bottom' => rect[1] + rect[3]
          },
          'scale' => scale,
          'perspective' => perspective
        }
      end

      def dimension_specs(parts)
        dimensions = []
        top = parts.fetch(:top)
        doors = parts.fetch(:doors)
        drawers = parts.fetch(:drawer_fronts)
        shelf_left = parts.fetch(:shelves).first
        bottom = parts.fetch(:bottom)
        back = parts.fetch(:back)
        leg_left = parts.fetch(:legs).min_by { |node| min_x(node) }
        drawer_front = drawers.first
        drawer_box_front = parts.fetch(:drawer_box_front)

        top_front_z = max_z(top)
        top_front_y = min_y(top)
        mid_x = (min_x(top) + max_x(top)) / 2.0

        # Plan: overall + center split + depth.
        dimensions << dim('plan-width', 'plan', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, -mm(120), 0.0])
        dimensions << dim('plan-half-left', 'plan', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [mid_x, top_front_y, top_front_z],
                          [0.0, -mm(65), 0.0])
        dimensions << dim('plan-half-right', 'plan', top,
                          [mid_x, top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, -mm(65), 0.0])
        dimensions << dim('plan-depth', 'plan', top,
                          [min_x(top), min_y(top), top_front_z],
                          top, [min_x(top), max_y(top), top_front_z],
                          [-mm(120), 0.0, 0.0])

        # Front: overall + 900/900 style main split derived from actual top midpoint.
        dimensions << dim('front-width', 'front', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, 0.0, mm(120)])
        dimensions << dim('front-half-left', 'front', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [mid_x, top_front_y, top_front_z],
                          [0.0, 0.0, mm(65)])
        dimensions << dim('front-half-right', 'front', top,
                          [mid_x, top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, 0.0, mm(65)])
        dimensions.concat(horizontal_chain('front-door', 'front', top, doors, [0.0, 0.0, -mm(90)]))

        overall_x = min_x(top)
        dimensions << dim('front-height', 'front', leg_left,
                          [overall_x, min_y(leg_left), min_z(leg_left)],
                          top, [overall_x, top_front_y, max_z(top)],
                          [-mm(100), 0.0, 0.0])
        dimensions << vertical_size_dim('front-leg-height', 'front', leg_left, [mm(100), 0.0, 0.0])
        dimensions << vertical_size_dim('front-door-height', 'front', doors.first, [mm(100), 0.0, 0.0])
        dimensions << vertical_size_dim('front-drawer-height', 'front', drawer_front, [mm(100), 0.0, 0.0])
        dimensions << vertical_size_dim('front-top-thickness', 'front', top, [mm(100), 0.0, 0.0])

        # Section A: actual depth and construction thicknesses.
        section_x = (min_x(drawer_front) + max_x(drawer_front)) / 2.0
        dimensions << dim('section-a-depth', 'section_a', top,
                          [section_x, min_y(top), max_z(top)],
                          top, [section_x, max_y(top), max_z(top)],
                          [0.0, 0.0, mm(100)])
        dimensions << dim('section-a-height', 'section_a', leg_left,
                          [section_x, min_y(leg_left), min_z(leg_left)],
                          top, [section_x, min_y(top), max_z(top)],
                          [0.0, -mm(100), 0.0])
        dimensions << depth_size_dim('section-a-drawer-front-thickness', 'section_a', drawer_front, [0.0, 0.0, mm(60)])
        dimensions << depth_size_dim('section-a-drawer-board-thickness', 'section_a', drawer_box_front, [0.0, 0.0, mm(45)])
        dimensions << vertical_size_dim('section-a-shelf-thickness', 'section_a', shelf_left, [0.0, mm(90), 0.0])
        dimensions << depth_size_dim('section-a-back-thickness', 'section_a', back, [0.0, 0.0, mm(55)])
        dimensions << vertical_size_dim('section-a-top-thickness', 'section_a', top, [0.0, mm(90), 0.0])

        # Section B: width and all real front subdivisions.
        dimensions << dim('section-b-width', 'section_b', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, 0.0, mm(120)])
        dimensions << dim('section-b-half-left', 'section_b', top,
                          [min_x(top), top_front_y, top_front_z],
                          top, [mid_x, top_front_y, top_front_z],
                          [0.0, 0.0, mm(65)])
        dimensions << dim('section-b-half-right', 'section_b', top,
                          [mid_x, top_front_y, top_front_z],
                          top, [max_x(top), top_front_y, top_front_z],
                          [0.0, 0.0, mm(65)])
        dimensions.concat(horizontal_chain('section-b-door', 'section_b', top, doors, [0.0, 0.0, -mm(90)]))
        dimensions << dim('section-b-shelf-level', 'section_b', bottom,
                          [min_x(bottom), min_y(bottom), max_z(bottom)],
                          shelf_left, [min_x(shelf_left), min_y(shelf_left), max_z(shelf_left)],
                          [-mm(100), 0.0, 0.0])

        # Side: overall depth/height plus true side and panel thickness.
        dimensions << dim('side-depth', 'side', top,
                          [max_x(top), min_y(top), max_z(top)],
                          top, [max_x(top), max_y(top), max_z(top)],
                          [0.0, 0.0, mm(100)])
        dimensions << depth_size_dim('side-carcass-depth', 'side', parts.fetch(:side_right), [0.0, 0.0, mm(60)])
        dimensions << dim('side-height', 'side', leg_left,
                          [max_x(top), min_y(leg_left), min_z(leg_left)],
                          top, [max_x(top), min_y(top), max_z(top)],
                          [0.0, -mm(100), 0.0])
        dimensions << vertical_size_dim('side-top-thickness', 'side', top, [0.0, mm(90), 0.0])
        dimensions << vertical_size_dim('side-shelf-thickness', 'side', shelf_left, [0.0, mm(90), 0.0])

        dimensions
      end

      def horizontal_chain(prefix, view_id, top, doors, offset)
        result = []
        z = min_z(doors.first)
        y = min_y(doors.first)

        result << dim("#{prefix}-left-reveal", view_id, top,
                      [min_x(top), y, z],
                      doors.first, [min_x(doors.first), y, z],
                      offset)

        doors.each_with_index do |door, index|
          result << dim("#{prefix}-door-#{index + 1}", view_id, door,
                        [min_x(door), y, z],
                        door, [max_x(door), y, z],
                        offset)
          next if index == doors.length - 1

          following = doors[index + 1]
          result << dim("#{prefix}-gap-#{index + 1}", view_id, door,
                        [max_x(door), y, z],
                        following, [min_x(following), y, z],
                        offset)
        end

        result << dim("#{prefix}-right-reveal", view_id, doors.last,
                      [max_x(doors.last), y, z],
                      top, [max_x(top), y, z],
                      offset)
        result
      end

      def vertical_size_dim(id, view_id, node, offset)
        x = min_x(node)
        y = min_y(node)
        dim(id, view_id, node, [x, y, min_z(node)], node, [x, y, max_z(node)], offset)
      end

      def depth_size_dim(id, view_id, node, offset)
        x = min_x(node)
        z = max_z(node)
        dim(id, view_id, node, [x, min_y(node), z], node, [x, max_y(node), z], offset)
      end

      def dim(id, view_id, start_node, start_point, end_node, end_point, offset)
        {
          'id' => id,
          'view_id' => view_id,
          'start' => {
            'point' => point_hash(start_point),
            'persistent_id_path' => start_node.pid_path
          },
          'end' => {
            'point' => point_hash(end_point),
            'persistent_id_path' => end_node.pid_path
          },
          'offset' => point_hash(offset)
        }
      end

      def note_lines(parts)
        doors = parts.fetch(:doors)
        gaps = doors.each_cons(2).map { |left, right| (min_x(right) - max_x(left)) * MM_PER_INCH }
        drawer_thickness = extent_y(parts.fetch(:drawer_box_front)) * MM_PER_INCH
        shelf_thickness = extent_z(parts.fetch(:shelves).first) * MM_PER_INCH

        [
          'GHI CHÚ:',
          '- Kích thước lấy trực tiếp từ model, đơn vị mm.',
          format('- Khe giữa cánh: %s mm.', gaps.map { |value| format('%.1f', value) }.join(' / ')),
          format('- Ván hộp ngăn kéo: %.1f mm; đợt di động: %.1f mm.', drawer_thickness, shelf_thickness)
        ]
      end

      def scene_name(suffix)
        "MCP-DOC-#{@base_name}-#{suffix}"
      end

      def remove_active_section
        @model.entities.active_section_plane = nil if @model.entities.respond_to?(:active_section_plane=)
      rescue StandardError
        nil
      end

      def persistent_id_path(path)
        instance_path = Sketchup::InstancePath.new(path)
        value = instance_path.persistent_id_path.to_s
        raise "persistent-id path is empty for #{entity_name(path.last)}" if value.empty?

        value
      end

      def transformed_bounds(box, transform)
        points = [
          [box.min.x, box.min.y, box.min.z],
          [box.max.x, box.min.y, box.min.z],
          [box.min.x, box.max.y, box.min.z],
          [box.max.x, box.max.y, box.min.z],
          [box.min.x, box.min.y, box.max.z],
          [box.max.x, box.min.y, box.max.z],
          [box.min.x, box.max.y, box.max.z],
          [box.max.x, box.max.y, box.max.z]
        ].map { |coords| Geom::Point3d.new(*coords).transform(transform) }

        result = Geom::BoundingBox.new
        points.each { |point| result.add(point) }
        result
      end

      def bounds_hash(box)
        {
          'min' => point_hash(box.min.to_a),
          'max' => point_hash(box.max.to_a)
        }
      end

      def point_hash(values)
        {'x' => values[0].to_f, 'y' => values[1].to_f, 'z' => values[2].to_f}
      end

      def bounds_center(box)
        Geom::Point3d.new(
          (box.min.x + box.max.x) / 2.0,
          (box.min.y + box.max.y) / 2.0,
          (box.min.z + box.max.z) / 2.0
        )
      end

      def supported_entity?(entity)
        %w[Group ComponentInstance].include?(entity.typename.to_s)
      end

      def entity_name(entity)
        name = entity.respond_to?(:name) ? entity.name.to_s : ''
        return name unless name.empty?
        return entity.definition.name.to_s if entity.respond_to?(:definition) && entity.definition

        ''
      end

      def bounds_volume(box)
        extent_x(box) * extent_y(box) * extent_z(box)
      end

      def extent_x(value)
        box = value.respond_to?(:bounds) ? value.bounds : value
        box.max.x - box.min.x
      end

      def extent_y(value)
        box = value.respond_to?(:bounds) ? value.bounds : value
        box.max.y - box.min.y
      end

      def extent_z(value)
        box = value.respond_to?(:bounds) ? value.bounds : value
        box.max.z - box.min.z
      end

      def min_x(node) = node.bounds.min.x
      def max_x(node) = node.bounds.max.x
      def min_y(node) = node.bounds.min.y
      def max_y(node) = node.bounds.max.y
      def min_z(node) = node.bounds.min.z
      def max_z(node) = node.bounds.max.z

      def mm(value)
        value.to_f / MM_PER_INCH
      end
    end
  end
end
