# frozen_string_literal: true

# Run this directory with TestUp inside a supported SketchUp 2026 26.x build after loading the
# extension. TestUp runs tests in SketchUp, so drain_once exercises the same
# main-thread dispatcher path used by the repeating UI.start_timer callback.

require 'minitest/autorun'
require 'thread'

class TC_GiaokhoaSketchupMcpDispatcher < Minitest::Test
  def test_session_info_executes_on_sketchup_main_thread
    bridge = Giaokhoa::SketchupMcp::Bridge
    requests = Queue.new
    responses = Queue.new
    model_state = Giaokhoa::SketchupMcp::ModelState.new
    model_state.track(Sketchup.active_model)
    registry = Giaokhoa::SketchupMcp::Commands::Registry.new(
      session_id: '00000000-0000-4000-8000-000000000001',
      model_state: model_state
    )
    dispatcher = Giaokhoa::SketchupMcp::Dispatcher::MainThreadDispatcher.new(
      request_queue: requests,
      command_registry: registry,
      ui: UI
    )
    requests << bridge::Request.new(
      id: '00000000-0000-4000-8000-000000000002',
      session_id: '00000000-0000-4000-8000-000000000001',
      operation: 'session.info',
      payload: {},
      deadline: Process.clock_gettime(Process::CLOCK_MONOTONIC) + 1.0,
      response_queue: responses,
      cancelled: -> { false }
    )

    assert_equal 1, dispatcher.drain_once
    response = responses.pop
    assert response.fetch('ok')
    assert_equal Sketchup.active_model.guid, response.fetch('payload').fetch('model').fetch('guid')
  ensure
    model_state&.stop
  end
end
