# frozen_string_literal: true

require 'fileutils'

module Giaokhoa
  module SketchupMcp
    class LayoutSheetBuilder
      A3_WIDTH_IN = 420.0 / 25.4
      A3_HEIGHT_IN = 297.0 / 25.4

      def initialize(model:, output_directory:, base_name:, export_pdf:)
        @model = model
        @output_directory = output_directory
        @base_name = base_name
        @export_pdf = export_pdf
      end

      def prepare_scenes!
        FileUtils.mkdir_p(@output_directory)
        prefix = "MCP-A3-#{@base_name}"
        scene_names = {
          plan: "#{prefix}-PLAN",
          front: "#{prefix}-FRONT",
          side: "#{prefix}-SIDE",
          section_a: "#{prefix}-SECTION-A",
          section_b: "#{prefix}-SECTION-B",
          iso: "#{prefix}-ISO"
        }

        center = Geom::Point3d.new(900.0 / 25.4, 225.0 / 25.4, 450.0 / 25.4)
        create_scene(scene_names[:plan], [center.x, center.y, 150.0], center, [0, 1, 0], false, 90.0)
        create_scene(scene_names[:front], [center.x, -120.0, center.z], center, [0, 0, 1], false, 60.0)
        create_scene(scene_names[:side], [140.0, center.y, center.z], center, [0, 0, 1], false, 55.0)
        create_scene(scene_names[:iso], [115.0, -80.0, 85.0], center, [0, 0, 1], true, nil)

        section_a = @model.entities.add_section_plane(
          [450.0 / 25.4, center.y, center.z],
          [-1.0, 0.0, 0.0]
        )
        raise 'failed to create section plane A-A' unless section_a

        section_a.activate
        create_scene(
          scene_names[:section_a],
          [-100.0, center.y, center.z],
          [450.0 / 25.4, center.y, center.z],
          [0, 0, 1],
          false,
          58.0,
          capture_section: true
        )

        section_b = @model.entities.add_section_plane(
          [center.x, 95.0 / 25.4, center.z],
          [0.0, -1.0, 0.0]
        )
        raise 'failed to create section plane B-B' unless section_b

        section_b.activate
        create_scene(
          scene_names[:section_b],
          [center.x, -110.0, center.z],
          [center.x, 95.0 / 25.4, center.z],
          [0, 0, 1],
          false,
          60.0,
          capture_section: true
        )

        if @model.entities.respond_to?(:active_section_plane=)
          @model.entities.active_section_plane = nil
        end
        rendering = @model.rendering_options
        rendering['DisplaySectionPlanes'] = false if rendering
        rendering['DisplaySectionCuts'] = true if rendering

        scene_names
      end

      def build!(scene_names)
        paths = output_paths
        raise "output already exists: #{paths[:layout]}" if File.exist?(paths[:layout])
        raise "output already exists: #{paths[:skp]}" if File.exist?(paths[:skp])
        raise "output already exists: #{paths[:pdf]}" if @export_pdf && File.exist?(paths[:pdf])

        saved = if @model.path.to_s.empty?
                  @model.save(paths[:skp])
                else
                  @model.save_copy(paths[:skp])
                end
        raise 'SketchUp model save failed' unless saved

        doc = Layout::Document.new
        setup_document(doc)
        page = doc.pages.first
        layer = doc.layers.first

        add_sheet_frame(doc, layer, page)

        specs = [
          [:plan, 'MẶT BẰNG', [0.55, 0.60, 4.70, 3.25], 1.0 / 15.0, false, 4.55],
          [:front, 'MẶT ĐỨNG CHÍNH', [5.82, 0.55, 5.48, 3.40], 1.0 / 15.0, false, 4.55],
          [:section_a, 'MẶT CẮT A-A', [11.92, 0.55, 4.10, 3.45], 1.0 / 10.0, false, 4.55],
          [:section_b, 'MẶT CẮT B-B', [0.55, 5.95, 6.35, 3.75], 1.0 / 15.0, false, 10.62],
          [:side, 'MẶT BÊN', [7.42, 5.95, 2.95, 3.80], 1.0 / 10.0, false, 10.62],
          [:iso, 'PHỐI CẢNH', [10.88, 5.80, 5.15, 4.05], nil, true, 10.62]
        ]

        viewports = {}
        specs.each do |key, title, rect, scale, perspective, label_y|
          viewport = add_viewport(
            doc, layer, page, paths[:skp], scene_names.fetch(key),
            rect, scale: scale, perspective: perspective
          )
          viewports[key] = viewport
          add_view_label(
            doc, layer, page, rect, title,
            perspective ? 'KHÔNG THEO TỶ LỆ' : scale_label(scale),
            label_y
          )
        end

        add_section_markers(doc, layer, page)
        add_notes(doc, layer, page)

        dimension_count = 0
        dimension_count += add_plan_dimensions(doc, layer, page)
        dimension_count += add_front_dimensions(doc, layer, page)
        dimension_count += add_section_a_dimensions(doc, layer, page)
        dimension_count += add_section_b_dimensions(doc, layer, page)
        dimension_count += add_side_dimensions(doc, layer, page)

        doc.save(paths[:layout])
        raise 'LayOut document save failed' unless File.exist?(paths[:layout])

        if @export_pdf
          doc.export(paths[:pdf])
          raise 'LayOut PDF export failed' unless File.exist?(paths[:pdf])
        end

        {
          'skp_path' => paths[:skp],
          'layout_path' => paths[:layout],
          'pdf_path' => @export_pdf ? paths[:pdf] : '',
          'page_width_mm' => 420.0,
          'page_height_mm' => 297.0,
          'viewport_count' => viewports.length,
          'dimension_count' => dimension_count,
          'scenes' => scene_names.values
        }
      end

      private

      def output_paths
        {
          skp: File.join(@output_directory, "#{@base_name}.skp"),
          layout: File.join(@output_directory, "#{@base_name}.layout"),
          pdf: File.join(@output_directory, "#{@base_name}.pdf")
        }
      end

      def create_scene(name, eye, target, up, perspective, height, capture_section: false)
        page = @model.pages.add(name)
        raise "failed to create scene #{name}" unless page

        camera = page.camera
        camera.set(
          Geom::Point3d.new(*eye),
          Geom::Point3d.new(*target),
          Geom::Vector3d.new(*up)
        )
        camera.perspective = perspective
        camera.height = height if !perspective && height
        page.use_camera = true

        if capture_section
          page.use_section_planes = true
          page.update(PAGE_USE_SECTION_PLANES)
        else
          page.use_section_planes = false
        end
        page
      end

      def setup_document(doc)
        info = doc.page_info
        info.width = A3_WIDTH_IN
        info.height = A3_HEIGHT_IN
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

      def add_viewport(doc, layer, page, skp_path, scene_name, rect, scale:, perspective:)
        viewport = Layout::SketchUpModel.new(skp_path, Geom::Bounds2d.new(*rect))
        scene_index = viewport.scenes.index(scene_name)
        raise "scene not found in LayOut model: #{scene_name}" unless scene_index

        viewport.current_scene = scene_index
        viewport.perspective = perspective
        viewport.display_background = false
        viewport.preserve_scale_on_resize = true
        viewport.scale = scale if scale && !perspective
        viewport.render_mode = Layout::SketchUpModel::HYBRID_RENDER
        doc.add_entity(viewport, layer, page)
        viewport.render
        viewport
      end

      def add_view_label(doc, layer, page, rect, title, scale_text, label_y)
        x, _y, w, _h = rect
        title_text = Layout::FormattedText.new(
          title,
          Geom::Bounds2d.new(x, label_y, w, 0.24)
        )
        title_style = title_text.style
        title_style.font_family = 'Arial'
        title_style.font_size = 9.0
        title_style.text_bold = true
        title_style.text_alignment = Layout::Style::ALIGN_CENTER
        title_text.style = title_style
        doc.add_entity(title_text, layer, page)

        scale = Layout::FormattedText.new(
          "TỶ LỆ: #{scale_text}",
          Geom::Bounds2d.new(x, label_y + 0.25, w, 0.18)
        )
        scale_style = scale.style
        scale_style.font_family = 'Arial'
        scale_style.font_size = 6.5
        scale_style.text_alignment = Layout::Style::ALIGN_CENTER
        scale.style = scale_style
        doc.add_entity(scale, layer, page)
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
        entity
      end

      def add_line(doc, layer, page, p1, p2, stroke_width = 0.30)
        entity = Layout::Path.new(Geom::Point2d.new(*p1), Geom::Point2d.new(*p2))
        style = entity.style
        style.stroke_width = stroke_width
        entity.style = style
        doc.add_entity(entity, layer, page)
        entity
      end

      def add_plain_text(doc, layer, page, value, x, y, width = 0.35, height = 0.25, font_size = 7.0, bold = true)
        text = Layout::FormattedText.new(value, Geom::Bounds2d.new(x, y, width, height))
        style = text.style
        style.font_family = 'Arial'
        style.font_size = font_size
        style.text_bold = bold
        style.text_alignment = Layout::Style::ALIGN_CENTER
        text.style = style
        doc.add_entity(text, layer, page)
        text
      end

      def add_section_markers(doc, layer, page)
        # Plan: A-A longitudinal marker and B-B transverse marker.
        add_line(doc, layer, page, [3.45, 0.88], [3.45, 3.55], 0.30)
        add_plain_text(doc, layer, page, 'A', 3.28, 0.66)
        add_plain_text(doc, layer, page, 'A', 3.28, 3.55)
        add_line(doc, layer, page, [0.72, 2.20], [5.10, 2.20], 0.30)
        add_plain_text(doc, layer, page, 'B', 0.47, 2.06)
        add_plain_text(doc, layer, page, 'B', 5.07, 2.06)

        # Front: show the location of section A-A through the cabinet.
        add_line(doc, layer, page, [8.56, 0.78], [8.56, 3.72], 0.30)
        add_plain_text(doc, layer, page, 'A', 8.39, 0.58)
        add_plain_text(doc, layer, page, 'A', 8.39, 3.72)
      end

      def scale_label(scale)
        denominator = (1.0 / scale).round(2)
        value = denominator.to_i == denominator ? denominator.to_i : denominator
        "1:#{value}"
      end

      def add_dimension(doc, layer, page, p1, p2, height, label, alignment, text_pos: nil, font_size: 7.0)
        dim = Layout::LinearDimension.new(
          Geom::Point2d.new(*p1),
          Geom::Point2d.new(*p2),
          height,
          alignment
        )
        dim.custom_text = true
        position = text_pos || if alignment == Layout::LinearDimension::DIMENSION_LINE_VERTICAL
                                 [p1[0] + height, (p1[1] + p2[1]) / 2.0]
                               else
                                 [(p1[0] + p2[0]) / 2.0, p1[1] + height]
                               end
        text = Layout::FormattedText.new(
          label,
          Geom::Point2d.new(*position),
          Layout::FormattedText::ANCHOR_TYPE_CENTER_CENTER
        )
        text_style = text.style
        text_style.font_family = 'Arial'
        text_style.font_size = font_size
        text.style = text_style
        dim.text = text
        style = dim.style
        style.stroke_width = 0.35
        style.suppress_dimension_units = true
        style.start_arrow_type = Layout::Style::ARROW_SLASH_RIGHT
        style.end_arrow_type = Layout::Style::ARROW_SLASH_LEFT
        dim.style = style
        doc.add_entity(dim, layer, page)
        1
      end

      def add_plan_dimensions(doc, layer, page)
        count = 0
        x0 = 0.88
        x1 = 4.92
        y0 = 1.18
        mid = (x0 + x1) / 2.0
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.62, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [x0, y0], [mid, y0], -0.34, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [mid, y0], [x1, y0], -0.34, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [0.88, 1.48], [0.88, 3.18], -0.38, '450', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_front_dimensions(doc, layer, page)
        count = 0
        x0 = 6.02
        x1 = 11.10
        y_top = 1.18
        mid = (x0 + x1) / 2.0

        # Row 1: overall + main 900/900 split.
        count += add_dimension(doc, layer, page, [x0, y_top], [x1, y_top], -0.62, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [x0, y_top], [mid, y_top], -0.34, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [mid, y_top], [x1, y_top], -0.34, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)

        # Row 2: inset-door subdivision, including the actual 2 mm gaps.
        y_bottom = 3.70
        spans = [20.0, 438.5, 2.0, 438.5, 2.0, 438.5, 2.0, 438.5, 20.0]
        total = spans.sum
        cursor = x0
        spans.each_with_index do |span, index|
          nxt = cursor + (x1 - x0) * (span / total)
          tiny = span <= 2.0
          text_pos = tiny ? [(cursor + nxt) / 2.0, 4.28 + ((index / 2) % 2) * 0.10] : nil
          count += add_dimension(
            doc, layer, page, [cursor, y_bottom], [nxt, y_bottom], 0.34,
            span == 438.5 ? '438.5' : span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL,
            text_pos: text_pos,
            font_size: tiny ? 6.0 : 6.7
          )
          cursor = nxt
        end

        # Overall vertical dimension on the left.
        count += add_dimension(doc, layer, page, [5.98, 1.22], [5.98, 3.78], -0.27, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)

        # Detailed vertical zones, matching the reference drawing style:
        # 250 legs + 420 lower cabinet + 200 drawer band + 30 top = 900.
        z_spans = [250.0, 420.0, 200.0, 30.0]
        y1 = 3.78
        full = 3.78 - 1.22
        z_spans.each do |span|
          y2 = y1 - full * (span / 900.0)
          count += add_dimension(
            doc, layer, page, [11.16, y1], [11.16, y2], 0.25,
            span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_VERTICAL,
            font_size: 6.6
          )
          y1 = y2
        end
        count
      end

      def add_section_a_dimensions(doc, layer, page)
        count = 0
        x0 = 12.18
        x1 = 15.76
        y0 = 1.18
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.58, '450', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)

        spans = [18.0, 406.0, 6.0, 20.0]
        total = spans.sum
        cursor = x0
        spans.each do |span|
          nxt = cursor + (x1 - x0) * (span / total)
          text_pos = if span <= 20.0
                       [(cursor + nxt) / 2.0, span == 6.0 ? 0.82 : 1.00]
                     end
          count += add_dimension(
            doc, layer, page, [cursor, y0], [nxt, y0], -0.30,
            span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL,
            text_pos: text_pos,
            font_size: span <= 20.0 ? 6.1 : 6.7
          )
          cursor = nxt
        end

        count += add_dimension(doc, layer, page, [12.10, 1.26], [12.10, 3.78], -0.25, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)

        # Internal vertical zones: legs / lower shelf levels / drawer band / top.
        z_spans = [250.0, 220.0, 18.0, 182.0, 200.0, 30.0]
        y1 = 3.78
        full = 3.78 - 1.26
        z_spans.each do |span|
          y2 = y1 - full * (span / 900.0)
          count += add_dimension(
            doc, layer, page, [15.78, y1], [15.78, y2], 0.20,
            span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_VERTICAL,
            font_size: span <= 30.0 ? 6.0 : 6.4
          )
          y1 = y2
        end

        # Local thickness callouts.
        count += add_dimension(doc, layer, page, [15.45, 1.98], [15.45, 2.24], 0.18, '12', Layout::LinearDimension::DIMENSION_LINE_VERTICAL, font_size: 6.1)
        count += add_dimension(doc, layer, page, [15.45, 2.70], [15.45, 2.96], 0.18, '18', Layout::LinearDimension::DIMENSION_LINE_VERTICAL, font_size: 6.1)
        count
      end

      def add_section_b_dimensions(doc, layer, page)
        count = 0
        x0 = 0.86
        x1 = 6.62
        y0 = 6.55
        mid = (x0 + x1) / 2.0
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.60, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [x0, y0], [mid, y0], -0.32, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [mid, y0], [x1, y0], -0.32, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)

        count += add_dimension(doc, layer, page, [0.82, 6.72], [0.82, 9.78], -0.30, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)

        y_bottom = 9.72
        spans = [20.0, 438.5, 2.0, 438.5, 2.0, 438.5, 2.0, 438.5, 20.0]
        cursor = x0
        spans.each_with_index do |span, index|
          nxt = cursor + (x1 - x0) * (span / 1800.0)
          tiny = span <= 2.0
          text_pos = tiny ? [(cursor + nxt) / 2.0, 10.28 + ((index / 2) % 2) * 0.08] : nil
          count += add_dimension(
            doc, layer, page, [cursor, y_bottom], [nxt, y_bottom], 0.30,
            span == 438.5 ? '438.5' : span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL,
            text_pos: text_pos,
            font_size: tiny ? 6.0 : 6.5
          )
          cursor = nxt
        end

        count += add_dimension(doc, layer, page, [6.68, 6.85], [6.68, 7.22], 0.22, '12', Layout::LinearDimension::DIMENSION_LINE_VERTICAL, font_size: 6.4)
        count += add_dimension(doc, layer, page, [6.68, 8.15], [6.68, 8.45], 0.22, '18', Layout::LinearDimension::DIMENSION_LINE_VERTICAL, font_size: 6.4)
        count
      end

      def add_side_dimensions(doc, layer, page)
        count = 0
        x0 = 7.72
        x1 = 10.12
        y0 = 6.55
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.58, '450', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)

        spans = [18.0, 406.0, 6.0, 20.0]
        cursor = x0
        spans.each do |span|
          nxt = cursor + (x1 - x0) * (span / 450.0)
          text_pos = if span <= 20.0
                       [(cursor + nxt) / 2.0, span == 6.0 ? 6.15 : 6.34]
                     end
          count += add_dimension(
            doc, layer, page, [cursor, y0], [nxt, y0], -0.30,
            span.to_i.to_s,
            Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL,
            text_pos: text_pos,
            font_size: span <= 20.0 ? 6.0 : 6.5
          )
          cursor = nxt
        end

        count += add_dimension(doc, layer, page, [7.63, 6.72], [7.63, 9.76], -0.25, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_notes(doc, layer, page)
        add_note_line(doc, layer, page, 'GHI CHÚ:', 11.25, 10.92, 4.55, 6.4, true)
        add_note_line(doc, layer, page, '- Kích thước tính bằng millimet (mm).', 11.25, 11.08, 4.55, 5.8, false)
        add_note_line(doc, layer, page, '- Cánh dưới lọt lòng; khe giữa các cánh: 2 mm.', 11.25, 11.22, 4.55, 5.8, false)
        add_note_line(doc, layer, page, '- Ván hộp ngăn kéo: 12 mm; đợt di động: 18 mm.', 11.25, 11.36, 4.55, 5.8, false)
      end

      def add_note_line(doc, layer, page, value, x, y, width, font_size, bold)
        text = Layout::FormattedText.new(value, Geom::Bounds2d.new(x, y, width, 0.13))
        style = text.style
        style.font_family = 'Arial'
        style.font_size = font_size
        style.text_bold = bold
        text.style = style
        doc.add_entity(text, layer, page)
      end

    end
  end
end
