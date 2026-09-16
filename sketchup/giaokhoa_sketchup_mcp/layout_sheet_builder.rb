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

        specs = [
          [:plan, 'MẶT BẰNG', [0.45, 0.60, 5.00, 3.45], 1.0 / 12.0, false],
          [:front, 'MẶT ĐỨNG CHÍNH', [5.60, 0.60, 5.95, 3.45], 1.0 / 12.0, false],
          [:section_a, 'MẶT CẮT A-A', [11.70, 0.60, 4.35, 3.45], 1.0 / 8.0, false],
          [:section_b, 'MẶT CẮT B-B', [0.45, 6.00, 7.15, 4.55], 1.0 / 10.0, false],
          [:side, 'MẶT BÊN', [7.78, 6.00, 2.75, 4.55], 1.0 / 8.0, false],
          [:iso, 'PHỐI CẢNH', [10.72, 6.00, 5.35, 4.55], nil, true]
        ]

        viewports = {}
        specs.each do |key, title, rect, scale, perspective|
          viewport = add_viewport(
            doc, layer, page, paths[:skp], scene_names.fetch(key),
            rect, scale: scale, perspective: perspective
          )
          viewports[key] = viewport
          add_view_label(doc, layer, page, rect, title, perspective ? 'KHÔNG THEO TỶ LỆ' : scale_label(scale))
        end

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

      def add_view_label(doc, layer, page, rect, title, scale_text)
        x, y, w, h = rect
        text = Layout::FormattedText.new(
          "#{title}\nTỶ LỆ: #{scale_text}",
          Geom::Bounds2d.new(x, y + h - 0.12, w, 0.42)
        )
        style = text.style
        style.font_family = 'Arial'
        style.font_size = 9.0
        style.text_bold = true
        style.text_alignment = Layout::Style::ALIGN_CENTER
        text.style = style
        doc.add_entity(text, layer, page)
      end

      def scale_label(scale)
        denominator = (1.0 / scale).round(2)
        value = denominator.to_i == denominator ? denominator.to_i : denominator
        "1:#{value}"
      end

      def add_dimension(doc, layer, page, p1, p2, height, label, alignment)
        dim = Layout::LinearDimension.new(
          Geom::Point2d.new(*p1),
          Geom::Point2d.new(*p2),
          height,
          alignment
        )
        dim.custom_text = true
        midpoint = Geom::Point2d.new((p1[0] + p2[0]) / 2.0, (p1[1] + p2[1]) / 2.0)
        text = Layout::FormattedText.new(
          label,
          midpoint,
          Layout::FormattedText::ANCHOR_TYPE_CENTER_CENTER
        )
        text_style = text.style
        text_style.font_family = 'Arial'
        text_style.font_size = 7.5
        text.style = text_style
        dim.text = text
        style = dim.style
        style.stroke_width = 0.5
        dim.style = style
        doc.add_entity(dim, layer, page)
        1
      end

      def add_plan_dimensions(doc, layer, page)
        count = 0
        count += add_dimension(doc, layer, page, [0.75, 1.10], [5.10, 1.10], -0.32, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [0.75, 1.10], [2.925, 1.10], -0.62, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [2.925, 1.10], [5.10, 1.10], -0.62, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [0.75, 1.10], [0.75, 2.55], -0.38, '450', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_front_dimensions(doc, layer, page)
        count = 0
        x0 = 5.92
        x1 = 11.22
        y0 = 1.05
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.33, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)

        spans = [20, 438.5, 2, 438.5, 2, 438.5, 2, 438.5, 20]
        total = spans.sum.to_f
        cursor = x0
        spans.each do |span|
          nxt = cursor + (x1 - x0) * (span / total)
          count += add_dimension(doc, layer, page, [cursor, y0], [nxt, y0], -0.68, span.to_s.sub('.0', ''), Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
          cursor = nxt
        end

        count += add_dimension(doc, layer, page, [5.86, 1.15], [5.86, 3.75], -0.32, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [5.86, 1.15], [5.86, 1.88], -0.62, '250', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [5.86, 1.88], [5.86, 3.10], -0.62, '398', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [5.86, 3.10], [5.86, 3.75], -0.62, '252', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_section_a_dimensions(doc, layer, page)
        count = 0
        count += add_dimension(doc, layer, page, [12.05, 1.12], [15.72, 1.12], -0.30, '450', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [12.05, 1.12], [12.22, 1.12], -0.60, '18', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [12.22, 1.12], [15.53, 1.12], -0.60, '414', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [15.53, 1.12], [15.72, 1.12], -0.60, '18', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [12.00, 1.18], [12.00, 3.72], -0.32, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [15.82, 2.72], [15.82, 3.35], 0.34, '12', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_section_b_dimensions(doc, layer, page)
        count = 0
        x0 = 0.78
        x1 = 7.18
        y0 = 6.52
        count += add_dimension(doc, layer, page, [x0, y0], [x1, y0], -0.34, '1800', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [x0, y0], [(x0 + x1) / 2.0, y0], -0.68, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [(x0 + x1) / 2.0, y0], [x1, y0], -0.68, '900', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [0.70, 6.65], [0.70, 10.18], -0.34, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [7.28, 8.82], [7.28, 9.22], 0.34, '12', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [7.28, 7.50], [7.28, 7.90], 0.34, '18', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count
      end

      def add_side_dimensions(doc, layer, page)
        count = 0
        count += add_dimension(doc, layer, page, [8.05, 6.55], [10.20, 6.55], -0.32, '450', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [7.98, 6.68], [7.98, 10.12], -0.32, '900', Layout::LinearDimension::DIMENSION_LINE_VERTICAL)
        count += add_dimension(doc, layer, page, [8.05, 6.55], [8.23, 6.55], -0.64, '18', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [8.23, 6.55], [10.02, 6.55], -0.64, '414', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count += add_dimension(doc, layer, page, [10.02, 6.55], [10.20, 6.55], -0.64, '18', Layout::LinearDimension::DIMENSION_LINE_HORIZONTAL)
        count
      end
    end
  end
end
