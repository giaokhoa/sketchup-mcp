# frozen_string_literal: true

require 'sketchup.rb'
require 'extensions.rb'

module Giaokhoa
  module SketchupMcp
    EXTENSION = SketchupExtension.new(
      'Giaokhoa SketchUp MCP',
      'giaokhoa_sketchup_mcp/extension'
    )
    EXTENSION.creator = 'giaokhoa'
    EXTENSION.description = 'Local bridge used by the giaokhoa SketchUp MCP host.'
    EXTENSION.version = '0.1.0'
    EXTENSION.copyright = '2026 giaokhoa'

    Sketchup.register_extension(EXTENSION, true)
  end
end
