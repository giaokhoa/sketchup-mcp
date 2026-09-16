# frozen_string_literal: true

require 'fileutils'

module Giaokhoa
  module SketchupMcp
    class LayoutSheetBuilder
      A3_WIDTH_MM = 420.0
      A3_HEIGHT_MM = 297.0
      MM_PER_INCH = 25.4

      def initialize(spec:)
        @spec = spec
      end

      def build!
        validate_spec!
        paths = output_paths
        refuse_overwrite!(paths)

        doc = Layout::Document.new
        setup_document(doc)
        page = doc.pages.first
        layer = doc.layers.first

        add_sheet_frame(doc, layer, page)

        viewports = {}
        @spec.fetch('views').each do |view|
          viewport = add_viewport(doc, layer, page, view)
          validate_viewport_model_fit!(viewport, view)
          viewports[view.fetch('id')] = viewport
          add_view_label(doc, layer, page, view)
        end

        add_section_markers(doc, layer, page)
        add_notes(doc, layer, page)

        connected = 0
        @spec.fetch('dimensions').each do |dimension|
          viewport = viewports.fetch(dimension.fetch('view_id'))
          add_connected_dimension(doc, layer, page, viewport, dimension)
          connected += 1
        end

        doc.save(paths.fetch(:layout))
        raise 'LayOut document save failed' unless File.exist?(paths.fetch(:layout))

        doc.export(paths.fetch(:pdf), compress_images: true, compress_quality: 0.8)
        raise 'LayOut PDF export failed' unless File.exist?(paths.fetch(:pdf))

        requested_png = paths.fetch(:png)
        doc.export(requested_png, dpi: 200)
        png_path = resolve_exported_png(requested_png)

        {
          'skp_path' => @spec.fetch('snapshot_path'),
          'layout_path' => paths.fetch(:layout),
          'pdf_path' => paths.fetch(:pdf),
          'png_path' => png_path,
          'page_width_mm' => A3_WIDTH_MM,
          'page_height_mm' => A3_HEIGHT_MM,
          'viewport_count' => viewports.length,
          'dimension_count' => @spec.fetch('dimensions').length,
          'connected_dimension_count' => connected,
          'scenes' => @spec.fetch('views').map { |view| view.fetch('id') }
        }
      end

      private

      def validate_spec!
        raise 'drawing spec schema_version must be 1' unless @spec['schema_version'] == 1
        raise 'documentation snapshot is missing' unless File.exist?(@spec.fetch('snapshot_path'))
        unless (@spec.fetch('page_width_mm').to_f - A3_WIDTH_MM).abs < 0.001 &&
               (@spec.fetch('page_height_mm').to_f - A3_HEIGHT_MM).abs < 0.001
          raise 'drawing spec must target A3 landscape'
        end
        raise 'drawing spec must contain exactly six views' unless @spec.fetch('views').length == 6
        raise 'drawing spec has no dimensions' if @spec.fetch('dimensions').empty?
        validate_paper_layout_spec!
      end

      def validate_paper_layout_spec!
        page_width = A3_WIDTH_MM / MM_PER_INCH
        page_height = A3_HEIGHT_MM / MM_PER_INCH
        views = @spec.fetch('views')

        views.each do |view|
          rect = view.fetch('rect')
          left = rect.fetch('left').to_f
          top = rect.fetch('top').to_f
          right = rect.fetch('right').to_f
          bottom = rect.fetch('bottom').to_f

          raise "view #{view.fetch('id')} has invalid paper bounds" unless right > left && bottom > top
          unless left >= 0.0 && top >= 0.0 && right <= page_width && bottom <= page_height
            raise "view #{view.fetch('id')} exceeds A3 page bounds"
          end

          label_bottom = bottom + 0.08 + 0.41
          raise "view #{view.fetch('id')} label exceeds A3 page bounds" if label_bottom > page_height
        end

        views.combination(2) do |a, b|
          next unless paper_rectangles_overlap?(a.fetch('rect'), b.fetch('rect'))

          raise "views #{a.fetch('id')} and #{b.fetch('id')} overlap"
        end

        notes_bottom = 10.88 + ((@spec.fetch('notes').length - 1) * 0.15) + 0.14
        raise 'notes exceed A3 page bounds' if notes_bottom > page_height
      end

      def paper_rectangles_overlap?(a, b)
        tolerance = 0.001
        a.fetch('left') < b.fetch('right') - tolerance &&
          a.fetch('right') > b.fetch('left') + tolerance &&
          a.fetch('top') < b.fetch('bottom') - tolerance &&
          a.fetch('bottom') > b.fetch('top') + tolerance
      end

      def output_paths
        directory = @spec.fetch('output_directory')
        FileUtils.mkdir_p(directory)
        base = @spec.fetch('base_name')
        {
          layout: File.join(directory, "#{base}.layout"),
          pdf: File.join(directory, "#{base}.pdf"),
          png: File.join(directory, "#{base}.png")
        }
      end

      def refuse_overwrite!(paths)
        paths.each_value do |path|
          raise "output already exists: #{path}" if File.exist?(path)
        end
        directory = File.dirname(paths.fetch(:png))
        base = File.basename(paths.fetch(:png), '.png')
        raise "PNG export already exists for #{base}" unless Dir.glob(File.join(directory, "#{base}*.png")).empty?
      end

      def setup_document(doc)
        info = doc.page_info
        info.width = A3_WIDTH_MM / MM_PER_INCH
        info.height = A3_HEIGHT_MM / MM_PER_INCH
        info.top_margin = 0.25
        info.bottom_margin = 0.25
        info.left_margin = 0.25
        info.right_margin = 0.25
        info.show_margins = false
        info.print_margins = false
        info.output_resolution = Layout::PageInfo::RESOLUTION_HIGH
        doc.units = Layout::Document::DECIMAL_MILLIMETERS
        doc.render_mode_override = Layout::SketchUpModel::HYBRID_RENDER
      end

      def add_viewport(doc, layer, page, view)
        rect = view.fetch('rect')
        bounds = Geom::Bounds2d.new(
          rect.fetch('left'),
          rect.fetch('top'),
          rect.fetch('right') - rect.fetch('left'),
          rect.fetch('bottom') - rect.fetch('top')
        )
        viewport = Layout::SketchUpModel.new(@spec.fetch('snapshot_path'), bounds)
        viewport.current_scene = view.fetch('scene_index')
        viewport.perspective = view.fetch('perspective')
        viewport.display_background = false
        viewport.preserve_scale_on_resize = true
        viewport.scale = view.fetch('scale') unless view.fetch('perspective')
        viewport.render_mode = Layout::SketchUpModel::HYBRID_RENDER
        viewport.line_weight = 0.5
        doc.add_entity(viewport, layer, page)
        viewport.render
        viewport
      end

      def validate_viewport_model_fit!(viewport, view)
        root = @spec.fetch('root_bounds')
        min = root.fetch('min')
        max = root.fetch('max')
        corners = [
          [min.fetch('x'), min.fetch('y'), min.fetch('z')],
          [max.fetch('x'), min.fetch('y'), min.fetch('z')],
          [min.fetch('x'), max.fetch('y'), min.fetch('z')],
          [max.fetch('x'), max.fetch('y'), min.fetch('z')],
          [min.fetch('x'), min.fetch('y'), max.fetch('z')],
          [max.fetch('x'), min.fetch('y'), max.fetch('z')],
          [min.fetch('x'), max.fetch('y'), max.fetch('z')],
          [max.fetch('x'), max.fetch('y'), max.fetch('z')]
        ]

        rect = view.fetch('rect')
        margin = 0.03
        left = rect.fetch('left') + margin
        top = rect.fetch('top') + margin
        right = rect.fetch('right') - margin
        bottom = rect.fetch('bottom') - margin

        projected = corners.map do |coords|
          viewport.model_to_paper_point(Geom::Point3d.new(*coords))
        end
        min_x = projected.map(&:x).min
        max_x = projected.map(&:x).max
        min_y = projected.map(&:y).min
        max_y = projected.map(&:y).max
        fits = min_x >= left && max_x <= right && min_y >= top && max_y <= bottom
        return if fits

        raise(
          "view #{view.fetch('id')} clips model bounds: " \
          "projected x=#{min_x.round(3)}..#{max_x.round(3)} " \
          "y=#{min_y.round(3)}..#{max_y.round(3)}; " \
          "viewport x=#{left.round(3)}..#{right.round(3)} " \
          "y=#{top.round(3)}..#{bottom.round(3)}"
        )
      end

      def add_connected_dimension(doc, layer, page, viewport, spec)
        start_3d = point3d(spec.fetch('start').fetch('point'))
        end_3d = point3d(spec.fetch('end').fetch('point'))
        offset = spec.fetch('offset')
        offset_3d = Geom::Point3d.new(
          start_3d.x + offset.fetch('x'),
          start_3d.y + offset.fetch('y'),
          start_3d.z + offset.fetch('z')
        )

        start_2d = viewport.model_to_paper_point(start_3d)
        end_2d = viewport.model_to_paper_point(end_3d)
        offset_2d = viewport.model_to_paper_point(offset_3d)
        height = signed_dimension_height(start_2d, end_2d, offset_2d)
        alignment = dimension_alignment(start_2d, end_2d)

        dimension = Layout::LinearDimension.new(start_2d, end_2d, height, alignment)
        dimension.custom_text = false
        dimension.auto_scale = true
        apply_dimension_style(dimension)

        doc.add_entity(dimension, layer, page)

        start_connection = Layout::ConnectionPoint.new(
          viewport,
          start_3d,
          spec.fetch('start').fetch('persistent_id_path')
        )
        end_connection = Layout::ConnectionPoint.new(
          viewport,
          end_3d,
          spec.fetch('end').fetch('persistent_id_path')
        )
        dimension.connect(start_connection, end_connection)

        raise "dimension #{spec.fetch('id')} unexpectedly uses custom text" if dimension.custom_text?
        dimension
      end

      def point3d(value)
        Geom::Point3d.new(value.fetch('x'), value.fetch('y'), value.fetch('z'))
      end

      def signed_dimension_height(start_point, end_point, offset_point)
        dx = end_point.x - start_point.x
        dy = end_point.y - start_point.y
        ox = offset_point.x - start_point.x
        oy = offset_point.y - start_point.y
        length = Math.sqrt((ox * ox) + (oy * oy))
        raise 'dimension offset projects to zero paper distance' if length <= 0.000001

        dot = (dy * ox) + (-dx * oy)
        dot.negative? ? -length : length
      end

      def dimension_alignment(start_point, end_point)
        dx = (end_point.x - start_point.x).abs
        dy = (end_point.y - start_point.y).abs
        dx >= dy ? Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL : Layout::LinearDimension::DIMENSION_LINE_VERTICAL
      end

      def apply_dimension_style(dimension)
        style = dimension.style
        style.set_dimension_units(Layout::Style::DECIMAL_MILLIMETERS, 0.1)
        style.suppress_dimension_units = true
        style.dimension_rotation_alignment = Layout::Style::DIMENSION_TEXT_HORIZONTAL
        style.dimension_vertical_alignment = Layout::Style::DIMENSION_TEXT_ABOVE
        style.stroke_width = 0.35
        style.start_arrow_type = Layout::Style::ARROW_SLASH_RIGHT
        style.end_arrow_type = Layout::Style::ARROW_SLASH_LEFT
        dimension.style = style
      end

      def add_view_label(doc, layer, page, view)
        rect = view.fetch('rect')
        x = rect.fetch('left')
        width = rect.fetch('right') - rect.fetch('left')
        y = rect.fetch('bottom') + 0.08

        title = Layout::FormattedText.new(view.fetch('title'), Geom::Bounds2d.new(x, y, width, 0.22))
        title_style = title.style
        title_style.font_family = 'Arial'
        title_style.font_size = 9.0
        title_style.text_bold = true
        title_style.text_alignment = Layout::Style::ALIGN_CENTER
        title.style = title_style
        doc.add_entity(title, layer, page)

        scale_label = view.fetch('perspective') ? 'KHÔNG THEO TỶ LỆ' : scale_text(view.fetch('scale'))
        scale = Layout::FormattedText.new("TỶ LỆ: #{scale_label}", Geom::Bounds2d.new(x, y + 0.24, width, 0.17))
        scale_style = scale.style
        scale_style.font_family = 'Arial'
        scale_style.font_size = 6.5
        scale_style.text_alignment = Layout::Style::ALIGN_CENTER
        scale.style = scale_style
        doc.add_entity(scale, layer, page)
      end

      def scale_text(scale)
        denominator = (1.0 / scale.to_f).round(2)
        value = denominator.to_i == denominator ? denominator.to_i : denominator
        "1:#{value}"
      end

      def add_sheet_frame(doc, layer, page)
        add_rectangle(doc, layer, page, [0.14, 0.14, 16.25, 11.40], 0.45)
        add_line(doc, layer, page, [0.14, 5.55], [16.39, 5.55], 0.30)
        add_line(doc, layer, page, [5.55, 0.14], [5.55, 5.55], 0.25)
        add_line(doc, layer, page, [11.64, 0.14], [11.64, 5.55], 0.25)
        add_line(doc, layer, page, [7.18, 5.55], [7.18, 11.54], 0.25)
        add_line(doc, layer, page, [10.68, 5.55], [10.68, 11.54], 0.25)
      end

      def add_rectangle(doc, layer, page, rect, stroke_width)
        entity = Layout::Rectangle.new(Geom::Bounds2d.new(*rect))
        style = entity.style
        style.stroke_width = stroke_width
        style.solid_filled = false
        style.pattern_filled = false
        entity.style = style
        doc.add_entity(entity, layer, page)
      end

      def add_line(doc, layer, page, p1, p2, stroke_width)
        entity = Layout::Path.new(Geom::Point2d.new(*p1), Geom::Point2d.new(*p2))
        style = entity.style
        style.stroke_width = stroke_width
        entity.style = style
        doc.add_entity(entity, layer, page)
      end

      def add_section_markers(doc, layer, page)
        add_line(doc, layer, page, [3.45, 0.88], [3.45, 3.55], 0.30)
        add_marker_text(doc, layer, page, 'A', 3.28, 0.66)
        add_marker_text(doc, layer, page, 'A', 3.28, 3.55)
        add_line(doc, layer, page, [0.72, 2.20], [5.10, 2.20], 0.30)
        add_marker_text(doc, layer, page, 'B', 0.47, 2.06)
        add_marker_text(doc, layer, page, 'B', 5.07, 2.06)
      end

      def add_marker_text(doc, layer, page, value, x, y)
        text = Layout::FormattedText.new(value, Geom::Bounds2d.new(x, y, 0.35, 0.25))
        style = text.style
        style.font_family = 'Arial'
        style.font_size = 7.0
        style.text_bold = true
        style.text_alignment = Layout::Style::ALIGN_CENTER
        text.style = style
        doc.add_entity(text, layer, page)
      end

      def add_notes(doc, layer, page)
        @spec.fetch('notes').each_with_index do |value, index|
          text = Layout::FormattedText.new(value, Geom::Bounds2d.new(11.05, 10.88 + (index * 0.15), 5.0, 0.14))
          style = text.style
          style.font_family = 'Arial'
          style.font_size = index.zero? ? 6.4 : 5.8
          style.text_bold = index.zero?
          text.style = style
          doc.add_entity(text, layer, page)
        end
      end

      def resolve_exported_png(requested)
        return requested if File.exist?(requested)

        directory = File.dirname(requested)
        base = File.basename(requested, '.png')
        matches = Dir.glob(File.join(directory, "#{base}*.png"))
        raise 'LayOut PNG export failed' if matches.empty?

        matches.max_by { |path| File.mtime(path) }
      end
    end
  end
end
