# frozen_string_literal: true

require 'fileutils'

module Giaokhoa
  module SketchupMcp
    class MutationEngine
      LAYOUT_ATTR_DICTIONARY = 'giaokhoa.sketchup_mcp'
      LAYOUT_ATTR_ENTITY_ID = 'entity_id'
      TEMPLATE_ATTR_DICTIONARY = 'giaokhoa.layout_template'
      MM_PER_INCH = 25.4

      def create_layout_document(payload)
        context, result = prepare_request(
          'layout.document.create',
          payload,
          %w[mutation layout_path template_path page_width_mm page_height_mm],
          'SketchUp MCP: Create LayOut Document'
        )
        return result if result

        path = payload['layout_path']
        template_path = payload['template_path'].to_s.strip
        width = finite_number(payload['page_width_mm'])
        height = finite_number(payload['page_height_mm'])
        return invalid('layout_path must end in .layout') unless path.is_a?(String) && path.downcase.end_with?('.layout')
        return error('LAYOUT_FILE_EXISTS', 'layout_path already exists') if File.exist?(path)

        if template_path.empty?
          return invalid('page_width_mm and page_height_mm must be positive') unless width&.positive? && height&.positive?
        else
          return invalid('template_path must end in .layout') unless template_path.downcase.end_with?('.layout')
          return error('LAYOUT_TEMPLATE_NOT_FOUND', 'template_path does not exist') unless File.exist?(template_path)
          return invalid('page dimensions must be 0 when template_path is provided') unless width == 0.0 && height == 0.0
        end

        perform_layout_file_operation(context, 'layout.document.create') do
          FileUtils.mkdir_p(File.dirname(path))
          doc = template_path.empty? ? Layout::Document.new : Layout::Document.new(template_path)
          if template_path.empty?
            doc.page_info.width = mm_to_inches(width)
            doc.page_info.height = mm_to_inches(height)
            doc.units = Layout::Document::DECIMAL_MILLIMETERS
          end
          doc.save(path)
          raise OperationFailure, 'LayOut document save failed' unless File.exist?(path)

          {
            'layout_path' => path,
            'page_width_mm' => inches_to_layout_mm(doc.page_info.width),
            'page_height_mm' => inches_to_layout_mm(doc.page_info.height),
            'page_count' => doc.pages.length,
            'template_path' => template_path.empty? ? nil : template_path
          }
        end
      end

      def inspect_layout_template(payload)
        required = %w[template_path]
        return invalid('payload fields are invalid') unless payload.is_a?(Hash) && payload.keys.sort == required

        path = payload['template_path']
        return invalid('template_path must end in .layout') unless path.is_a?(String) && path.downcase.end_with?('.layout')
        return error('LAYOUT_TEMPLATE_NOT_FOUND', 'template_path does not exist') unless File.exist?(path)

        doc = Layout::Document.open(path)
        page_width_mm = inches_to_layout_mm(doc.page_info.width)
        page_height_mm = inches_to_layout_mm(doc.page_info.height)

        pages = doc.pages.each_with_index.map do |page, index|
          {
            'index' => index,
            'name' => page.name.to_s,
            'width_mm' => page_width_mm,
            'height_mm' => page_height_mm
          }
        end

        layers = doc.layers.map do |layer|
          {
            'name' => layer.name.to_s,
            'shared' => layer.shared?,
            'locked' => layer.locked?
          }
        end

        slots = []
        styles = []
        doc.pages.each_with_index do |page, page_index|
          page.entities.each do |entity|
            role = entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'role', nil)
            next unless role

            if role == 'viewport_slot'
              bounds = entity.bounds
              upper_left = bounds.upper_left
              slots << {
                'slot_id' => entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'slot_id', '').to_s,
                'page_index' => page_index,
                'bounds_mm' => {
                  'x' => inches_to_layout_mm(upper_left.x),
                  'y' => inches_to_layout_mm(upper_left.y),
                  'width' => inches_to_layout_mm(bounds.width),
                  'height' => inches_to_layout_mm(bounds.height)
                },
                'default_scale_denominator' => entity.get_attribute(
                  TEMPLATE_ATTR_DICTIONARY,
                  'default_scale_denominator',
                  0.0
                ).to_f,
                'perspective' => entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'perspective', false) == true
              }
            elsif role == 'style_sample'
              styles << {
                'style_id' => entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'style_id', '').to_s,
                'page_index' => page_index
              }
            end
          end
        end

        auto_text_types = doc.auto_text_definitions.map do |definition|
          layout_auto_text_type_name(definition.type)
        end.compact.uniq.sort

        ok(
          'template_path' => path,
          'schema_version' => doc.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'schema_version', 0).to_i,
          'template_id' => doc.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'template_id', '').to_s,
          'template_kind' => doc.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'template_kind', '').to_s,
          'pages' => pages,
          'layers' => layers,
          'slots' => slots,
          'styles' => styles,
          'auto_text_types' => auto_text_types
        )
      rescue StandardError => e
        error(
          'LAYOUT_TEMPLATE_INSPECTION_FAILED',
          'LayOut template inspection failed',
          'reason' => e.message.to_s
        )
      end

      def add_layout_viewport(payload)
        context, result = prepare_request(
          'layout.viewport.add',
          payload,
          %w[mutation layout_path skp_path page_index layer_name bounds_mm scene_name standard_view perspective scale_denominator render_mode],
          'SketchUp MCP: Add LayOut Viewport'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.viewport.add') do
          doc = open_layout_document(payload['layout_path'])
          page = layout_page(doc, payload['page_index'])
          bounds = layout_bounds(payload['bounds_mm'])
          skp_path = payload['skp_path']
          raise OperationFailure, 'skp_path does not exist' unless skp_path.is_a?(String) && File.exist?(skp_path)

          viewport = Layout::SketchUpModel.new(skp_path, bounds)
          scene_name = payload['scene_name'].to_s.strip
          standard_view = payload['standard_view'].to_s.strip.downcase
          if !scene_name.empty?
            scene_index = viewport.scenes.index(scene_name)
            raise OperationFailure, "scene not found: #{scene_name}" unless scene_index
            viewport.current_scene = scene_index
          elsif !standard_view.empty?
            viewport.view = layout_standard_view(standard_view)
          else
            raise OperationFailure, 'scene_name or standard_view is required'
          end

          perspective = payload['perspective'] == true
          viewport.perspective = perspective
          viewport.preserve_scale_on_resize = true
          unless perspective
            denominator = finite_number(payload['scale_denominator'])
            raise OperationFailure, 'scale_denominator must be positive' unless denominator&.positive?
            viewport.scale = 1.0 / denominator
          end
          viewport.render_mode = layout_render_mode(payload['render_mode'])
          viewport.display_background = false
          doc.add_entity(viewport, resolve_layout_layer(doc, payload['layer_name']), page)
          tag_layout_entity(viewport, context.operation_id)
          viewport.render
          doc.save

          {
            'layout_path' => payload['layout_path'],
            'entity_ref' => layout_entity_ref(payload['layout_path'], payload['page_index'], context.operation_id)
          }
        end
      end

      def add_layout_dimension(payload)
        context, result = prepare_request(
          'layout.dimension.add',
          payload,
          %w[mutation layout_path page_index layer_name viewport_ref start_point_mm end_point_mm start_pid_path end_pid_path offset_mm alignment style_id],
          'SketchUp MCP: Add LayOut Dimension'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.dimension.add') do
          doc = open_layout_document(payload['layout_path'])
          page = layout_page(doc, payload['page_index'])
          viewport = resolve_layout_entity(doc, payload['viewport_ref'], Layout::SketchUpModel)

          start_3d = layout_point3d_mm(payload['start_point_mm'])
          end_3d = layout_point3d_mm(payload['end_point_mm'])
          start_2d = viewport.model_to_paper_point(start_3d)
          end_2d = viewport.model_to_paper_point(end_3d)
          offset = finite_number(payload['offset_mm'])
          raise OperationFailure, 'offset_mm must be non-zero' unless offset && !offset.zero?

          dimension = Layout::LinearDimension.new(
            start_2d,
            end_2d,
            mm_to_inches(offset),
            layout_dimension_alignment(payload['alignment'], start_2d, end_2d)
          )
          dimension.custom_text = false
          dimension.auto_scale = true
          style = dimension.style
          style.set_dimension_units(Layout::Style::DECIMAL_MILLIMETERS, 0.1)
          style.suppress_dimension_units = true
          dimension.style = style
          apply_layout_template_style!(doc, dimension, payload['style_id'])
          doc.add_entity(dimension, resolve_layout_layer(doc, payload['layer_name']), page)

          start_pid = payload['start_pid_path'].to_s
          end_pid = payload['end_pid_path'].to_s
          start_connection = start_pid.empty? ? Layout::ConnectionPoint.new(viewport, start_3d) : Layout::ConnectionPoint.new(viewport, start_3d, start_pid)
          end_connection = end_pid.empty? ? Layout::ConnectionPoint.new(viewport, end_3d) : Layout::ConnectionPoint.new(viewport, end_3d, end_pid)
          dimension.connect(start_connection, end_connection)
          tag_layout_entity(dimension, context.operation_id)
          doc.save

          {
            'layout_path' => payload['layout_path'],
            'entity_ref' => layout_entity_ref(payload['layout_path'], payload['page_index'], context.operation_id),
            'connected' => !dimension.custom_text?
          }
        end
      end

      def add_layout_text(payload)
        context, result = prepare_request(
          'layout.text.add',
          payload,
          %w[mutation layout_path page_index layer_name bounds_mm text font_size_pt bold alignment style_id],
          'SketchUp MCP: Add LayOut Text'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.text.add') do
          doc = open_layout_document(payload['layout_path'])
          page = layout_page(doc, payload['page_index'])
          text = payload['text'].to_s
          raise OperationFailure, 'text is required' if text.strip.empty?

          entity = Layout::FormattedText.new(text, layout_bounds(payload['bounds_mm']))
          style = entity.style
          style.font_size = finite_number(payload['font_size_pt'])
          style.text_bold = payload['bold'] == true
          style.text_alignment = layout_text_alignment(payload['alignment'])
          entity.style = style
          apply_layout_template_style!(doc, entity, payload['style_id'])
          doc.add_entity(entity, resolve_layout_layer(doc, payload['layer_name']), page)
          tag_layout_entity(entity, context.operation_id)
          doc.save

          {
            'layout_path' => payload['layout_path'],
            'entity_ref' => layout_entity_ref(payload['layout_path'], payload['page_index'], context.operation_id)
          }
        end
      end

      def add_layout_line(payload)
        context, result = prepare_request(
          'layout.line.add',
          payload,
          %w[mutation layout_path page_index layer_name start_mm end_mm stroke_width],
          'SketchUp MCP: Add LayOut Line'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.line.add') do
          doc = open_layout_document(payload['layout_path'])
          page = layout_page(doc, payload['page_index'])
          start_point = layout_point2d_mm(payload['start_mm'])
          end_point = layout_point2d_mm(payload['end_mm'])
          entity = Layout::Path.new(start_point, end_point)
          style = entity.style
          style.stroke_width = finite_number(payload['stroke_width'])
          entity.style = style
          doc.add_entity(entity, resolve_layout_layer(doc, payload['layer_name']), page)
          tag_layout_entity(entity, context.operation_id)
          doc.save

          {
            'layout_path' => payload['layout_path'],
            'entity_ref' => layout_entity_ref(payload['layout_path'], payload['page_index'], context.operation_id)
          }
        end
      end

      def add_layout_rectangle(payload)
        context, result = prepare_request(
          'layout.rectangle.add',
          payload,
          %w[mutation layout_path page_index layer_name bounds_mm stroke_width],
          'SketchUp MCP: Add LayOut Rectangle'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.rectangle.add') do
          doc = open_layout_document(payload['layout_path'])
          page = layout_page(doc, payload['page_index'])
          entity = Layout::Rectangle.new(layout_bounds(payload['bounds_mm']))
          style = entity.style
          style.stroke_width = finite_number(payload['stroke_width'])
          style.solid_filled = false
          style.pattern_filled = false
          entity.style = style
          doc.add_entity(entity, resolve_layout_layer(doc, payload['layer_name']), page)
          tag_layout_entity(entity, context.operation_id)
          doc.save

          {
            'layout_path' => payload['layout_path'],
            'entity_ref' => layout_entity_ref(payload['layout_path'], payload['page_index'], context.operation_id)
          }
        end
      end

      def export_layout_document(payload)
        context, result = prepare_request(
          'layout.export',
          payload,
          %w[mutation layout_path output_path dpi],
          'SketchUp MCP: Export LayOut Document'
        )
        return result if result

        perform_layout_file_operation(context, 'layout.export') do
          doc = open_layout_document(payload['layout_path'])
          output_path = payload['output_path'].to_s
          raise OperationFailure, 'output_path already exists' if File.exist?(output_path)
          FileUtils.mkdir_p(File.dirname(output_path))

          ext = File.extname(output_path).downcase
          options = {}
          options[:dpi] = payload['dpi'].to_i if %w[.png .jpg .jpeg].include?(ext)
          doc.export(output_path, options.empty? ? nil : options)
          exported = resolve_layout_export(output_path)
          raise OperationFailure, 'LayOut export failed' unless File.exist?(exported)

          {
            'layout_path' => payload['layout_path'],
            'output_path' => exported,
            'mime_type' => layout_mime_type(ext)
          }
        end
      end

      private

      def perform_layout_file_operation(context, operation)
        mark_running(context)
        begin
          payload = yield
          post_snapshot = @model_state.snapshot
          unless post_snapshot.fetch(:guid) == context.snapshot.fetch(:guid) &&
                 post_snapshot.fetch(:revision) == context.snapshot.fetch(:revision)
            result = error(
              'STALE_REVISION',
              'SketchUp model changed during LayOut operation',
              'operation' => operation,
              'expected_revision' => context.snapshot.fetch(:revision),
              'actual_revision' => post_snapshot.fetch(:revision)
            )
            return record_terminal(context, :failed, result)
          end

          record_terminal(context, :completed, success_result(context, post_snapshot, payload))
        rescue StandardError => e
          result = error(
            'LAYOUT_OPERATION_FAILED',
            'LayOut operation failed',
            'operation' => operation,
            'reason' => e.message.to_s
          )
          record_terminal(context, :failed, result)
        end
      end

      def open_layout_document(path)
        raise OperationFailure, 'layout_path does not exist' unless path.is_a?(String) && File.exist?(path)
        Layout::Document.open(path)
      end

      def layout_page(doc, index)
        raise OperationFailure, 'page_index must be a non-negative integer' unless index.is_a?(Integer) && index >= 0
        doc.pages[index]
      rescue IndexError
        raise OperationFailure, "page_index out of range: #{index}"
      end

      def layout_bounds(value)
        raise OperationFailure, 'bounds_mm must be an object' unless value.is_a?(Hash)
        x = finite_number(value['x'])
        y = finite_number(value['y'])
        width = finite_number(value['width'])
        height = finite_number(value['height'])
        raise OperationFailure, 'bounds_mm values are invalid' unless x && y && width&.positive? && height&.positive?

        Geom::Bounds2d.new(mm_to_inches(x), mm_to_inches(y), mm_to_inches(width), mm_to_inches(height))
      end

      def layout_point2d_mm(value)
        raise OperationFailure, 'paper point must be an object' unless value.is_a?(Hash)
        x = finite_number(value['x'])
        y = finite_number(value['y'])
        raise OperationFailure, 'paper point values are invalid' unless x && y
        Geom::Point2d.new(mm_to_inches(x), mm_to_inches(y))
      end

      def layout_point3d_mm(value)
        raise OperationFailure, 'model point must be an object' unless value.is_a?(Hash)
        x = finite_number(value['x'])
        y = finite_number(value['y'])
        z = finite_number(value['z'])
        raise OperationFailure, 'model point values are invalid' unless x && y && z
        Geom::Point3d.new(mm_to_inches(x), mm_to_inches(y), mm_to_inches(z))
      end

      def resolve_layout_layer(doc, name)
        requested = name.to_s.strip
        return doc.layers.first if requested.empty?

        layer = doc.layers.find { |candidate| candidate.name.to_s == requested }
        raise OperationFailure, "LayOut layer not found: #{requested}" unless layer
        layer
      end

      def apply_layout_template_style!(doc, target, style_id)
        requested = style_id.to_s.strip
        return if requested.empty?

        source = nil
        doc.pages.each do |page|
          page.entities.each do |entity|
            next unless entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'role', nil) == 'style_sample'
            next unless entity.get_attribute(TEMPLATE_ATTR_DICTIONARY, 'style_id', '').to_s == requested
            source = entity
            break
          end
          break if source
        end
        raise OperationFailure, "LayOut template style not found: #{requested}" unless source

        source_style = source.style
        raise OperationFailure, "LayOut template style has no style object: #{requested}" unless source_style
        target.style = source_style
      end

      def layout_standard_view(value)
        {
          'top' => Layout::SketchUpModel::TOP_VIEW,
          'bottom' => Layout::SketchUpModel::BOTTOM_VIEW,
          'front' => Layout::SketchUpModel::FRONT_VIEW,
          'back' => Layout::SketchUpModel::BACK_VIEW,
          'left' => Layout::SketchUpModel::LEFT_VIEW,
          'right' => Layout::SketchUpModel::RIGHT_VIEW,
          'iso' => Layout::SketchUpModel::ISO_VIEW
        }.fetch(value) { raise OperationFailure, "unsupported standard_view: #{value}" }
      end

      def layout_render_mode(value)
        {
          'raster' => Layout::SketchUpModel::RASTER_RENDER,
          'hybrid' => Layout::SketchUpModel::HYBRID_RENDER,
          'vector' => Layout::SketchUpModel::VECTOR_RENDER
        }.fetch(value.to_s.downcase) { raise OperationFailure, "unsupported render_mode: #{value}" }
      end

      def layout_text_alignment(value)
        {
          'left' => Layout::Style::ALIGN_LEFT,
          'center' => Layout::Style::ALIGN_CENTER,
          'right' => Layout::Style::ALIGN_RIGHT
        }.fetch(value.to_s.downcase) { raise OperationFailure, "unsupported text alignment: #{value}" }
      end

      def layout_dimension_alignment(value, start_point, end_point)
        normalized = value.to_s.downcase
        if normalized == 'auto'
          dx = (end_point.x - start_point.x).abs
          dy = (end_point.y - start_point.y).abs
          normalized = dx >= dy ? 'horizontal' : 'vertical'
        end
        {
          'horizontal' => Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL,
          'vertical' => Layout::LinearDimension::DIMENSION_LINE_VERTICAL,
          'aligned' => Layout::LinearDimension::DIMENSION_LINE_ALIGNED
        }.fetch(normalized) { raise OperationFailure, "unsupported dimension alignment: #{value}" }
      end

      def tag_layout_entity(entity, entity_id)
        entity.set_attribute(LAYOUT_ATTR_DICTIONARY, LAYOUT_ATTR_ENTITY_ID, entity_id)
      end

      def resolve_layout_entity(doc, ref, klass)
        raise OperationFailure, 'viewport_ref must be an object' unless ref.is_a?(Hash)
        page = layout_page(doc, ref['page_index'])
        entity_id = ref['entity_id'].to_s
        found = nil
        page.entities.each do |entity|
          next unless entity.get_attribute(LAYOUT_ATTR_DICTIONARY, LAYOUT_ATTR_ENTITY_ID) == entity_id
          found = entity
          break
        end
        raise OperationFailure, "LayOut entity not found: #{entity_id}" unless found
        raise OperationFailure, "LayOut entity has wrong type: #{found.class}" unless found.is_a?(klass)
        found
      end

      def layout_entity_ref(layout_path, page_index, entity_id)
        {
          'layout_path' => layout_path,
          'page_index' => page_index,
          'entity_id' => entity_id
        }
      end

      def resolve_layout_export(requested)
        return requested if File.exist?(requested)
        ext = File.extname(requested)
        return requested unless %w[.png .jpg .jpeg].include?(ext.downcase)

        directory = File.dirname(requested)
        base = File.basename(requested, ext)
        matches = Dir.glob(File.join(directory, "#{base}*#{ext}"))
        matches.max_by { |path| File.mtime(path) } || requested
      end

      def layout_mime_type(ext)
        case ext
        when '.pdf' then 'application/pdf'
        when '.png' then 'image/png'
        when '.jpg', '.jpeg' then 'image/jpeg'
        else 'application/octet-stream'
        end
      end

      def inches_to_layout_mm(value)
        value.to_f * MM_PER_INCH
      end

      def layout_auto_text_type_name(type)
        mapping = {
          Layout::AutoTextDefinition::TYPE_MODEL_SCENE_NAME => 'model_scene_name',
          Layout::AutoTextDefinition::TYPE_MODEL_SCALE => 'model_scale',
          Layout::AutoTextDefinition::TYPE_MODEL_SECTION_NAME => 'model_section_name',
          Layout::AutoTextDefinition::TYPE_MODEL_SECTION_SYMBOL => 'model_section_symbol',
          Layout::AutoTextDefinition::TYPE_PAGE_NAME => 'page_name',
          Layout::AutoTextDefinition::TYPE_PAGE_NUMBER => 'page_number',
          Layout::AutoTextDefinition::TYPE_PAGE_COUNT => 'page_count',
          Layout::AutoTextDefinition::TYPE_FILE => 'file'
        }
        mapping[type]
      end

      def mm_to_inches(value)
        value.to_f / MM_PER_INCH
      end
    end
  end
end
