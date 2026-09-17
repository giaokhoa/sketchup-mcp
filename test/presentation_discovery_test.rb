# frozen_string_literal: true

require 'minitest/autorun'

module Sketchup
  class SectionPlane; end
end

require_relative '../sketchup/giaokhoa_sketchup_mcp/mutation_engine'
require_relative '../sketchup/giaokhoa_sketchup_mcp/presentation_engine'

class PresentationDiscoveryTest < Minitest::Test
  Engine = Giaokhoa::SketchupMcp::MutationEngine
  SESSION_ID = '11111111-1111-4111-8111-111111111111'
  Point = Struct.new(:x, :y, :z)

  class FakeState
    def initialize(model, revision: 7)
      @model = model
      @snapshot = {guid: 'model-guid', revision: revision}
    end

    def capture
      [@model, @snapshot]
    end
  end

  class FakeSectionPlane < Sketchup::SectionPlane
    attr_reader :persistent_id, :name, :symbol

    def initialize(persistent_id:, name:, symbol:, plane:, active: false)
      @persistent_id = persistent_id
      @name = name
      @symbol = symbol
      @plane = plane
      @active = active
    end

    def get_plane
      @plane
    end

    def active?
      @active
    end
  end

  class FakeCamera
    attr_reader :eye, :target, :up, :height, :fov

    def initialize(perspective:, height: 0.0, fov: 0.0)
      @perspective = perspective
      @height = height
      @fov = fov
      @eye = Point.new(10.0, 20.0, 30.0)
      @target = Point.new(1.0, 2.0, 3.0)
      @up = Point.new(0.0, 0.0, 1.0)
    end

    def perspective?
      @perspective
    end
  end

  class FakePage
    attr_reader :persistent_id, :name, :camera, :rendering_options

    def initialize(persistent_id:, name:, camera:, section_plane: nil)
      @persistent_id = persistent_id
      @name = name
      @camera = camera
      @active_section_planes = section_plane ? [section_plane] : []
      @rendering_options = {'DisplaySectionCuts' => !section_plane.nil?, 'DisplaySectionPlanes' => false}
    end

    def active_section_planes
      @active_section_planes
    end
  end

  class FakePages
    attr_reader :selected_page

    def initialize(items, selected_page)
      @items = items
      @selected_page = selected_page
    end

    def each(&block)
      @items.each(&block)
    end
  end

  class FakeModel
    attr_reader :entities, :pages

    def initialize(section_planes:, pages:)
      @entities = section_planes
      @pages = pages
    end
  end

  def engine_for(section_planes:, pages: [], selected_page: nil)
    page_collection = FakePages.new(pages, selected_page)
    model = FakeModel.new(section_planes: section_planes, pages: page_collection)
    Engine.new(session_id: SESSION_ID, model_state: FakeState.new(model))
  end

  def test_section_plane_list_returns_durable_ref_and_normalized_plane
    section = FakeSectionPlane.new(
      persistent_id: 42, name: 'Cut A', symbol: 'A', plane: [2.0, 0.0, 0.0, -20.0], active: true
    )
    result = engine_for(section_planes: [section]).list_section_planes({})

    assert result.fetch(:ok)
    output = result.fetch(:payload)
    assert_equal 1, output.fetch('returned_count')
    item = output.fetch('section_planes').first
    assert_equal 42, item.fetch('entity_ref').fetch('persistent_id')
    assert_equal 7, item.fetch('entity_ref').fetch('revision')
    assert_in_delta 254.0, item.fetch('origin_mm').fetch('x'), 0.0001
    assert_in_delta 1.0, item.fetch('normal').fetch('x'), 0.0001
    assert_equal true, item.fetch('active')
  end

  def test_section_plane_list_is_bounded
    sections = 101.times.map do |index|
      FakeSectionPlane.new(
        persistent_id: index + 1, name: "Cut #{index}", symbol: '', plane: [1.0, 0.0, 0.0, 0.0]
      )
    end
    output = engine_for(section_planes: sections).list_section_planes({}).fetch(:payload)

    assert_equal 101, output.fetch('total_count')
    assert_equal 100, output.fetch('returned_count')
    assert_equal true, output.fetch('truncated')
  end

  def test_scene_list_reports_camera_active_scene_and_captured_section
    section = FakeSectionPlane.new(
      persistent_id: 55, name: 'Cut A', symbol: 'A', plane: [1.0, 0.0, 0.0, -10.0]
    )
    page = FakePage.new(
      persistent_id: 77, name: 'DOC Front',
      camera: FakeCamera.new(perspective: false, height: 40.0), section_plane: section
    )
    result = engine_for(section_planes: [section], pages: [page], selected_page: page).list_scenes({})

    assert result.fetch(:ok)
    item = result.fetch(:payload).fetch('scenes').first
    assert_equal 'DOC Front', item.fetch('name')
    assert_equal 0, item.fetch('index')
    assert_equal 77, item.fetch('persistent_id')
    assert_equal true, item.fetch('active')
    assert_equal false, item.fetch('camera').fetch('perspective')
    assert_in_delta 1016.0, item.fetch('camera').fetch('orthographic_height_mm'), 0.0001
    assert_equal 55, item.fetch('active_section_plane_ref').fetch('persistent_id')
    assert_equal true, item.fetch('display_section_cuts')
    assert_equal false, item.fetch('display_section_planes')
  end
end
