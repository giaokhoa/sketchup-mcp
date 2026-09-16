# frozen_string_literal: true

require 'sketchup.rb'

module Giaokhoa
  module SketchupMcp
    ROOT = __dir__ unless const_defined?(:ROOT, false)
  end
end

Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'bridge', 'protocol')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'bridge', 'request')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'bridge', 'discovery')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'bridge', 'server')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'model_state')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'mutation_engine')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'documentation_snapshot_builder')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'commands', 'registry')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'dispatcher', 'main_thread_dispatcher')
Sketchup.require File.join(Giaokhoa::SketchupMcp::ROOT, 'runtime')

Giaokhoa::SketchupMcp::Runtime.start
