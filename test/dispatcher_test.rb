# frozen_string_literal: true

require 'minitest/autorun'
require 'thread'

require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/protocol'
require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/request'
require_relative '../sketchup/giaokhoa_sketchup_mcp/dispatcher/main_thread_dispatcher'

class DispatcherTest < Minitest::Test
  Bridge = Giaokhoa::SketchupMcp::Bridge
  Dispatcher = Giaokhoa::SketchupMcp::Dispatcher::MainThreadDispatcher

  class FakeRegistry
    attr_reader :calls

    def initialize
      @calls = []
    end

    def call(operation, payload)
      @calls << [operation, payload]
      {ok: true, payload: {'pong' => true}}
    end
  end

  def test_executes_queued_command_and_returns_response
    requests = Queue.new
    responses = Queue.new
    registry = FakeRegistry.new
    requests << request(responses, deadline: 11.0)
    dispatcher = Dispatcher.new(
      request_queue: requests,
      command_registry: registry,
      ui: Object.new,
      monotonic_clock: -> { 10.0 }
    )

    assert_equal 1, dispatcher.drain_once
    assert_equal [['system.ping', {}]], registry.calls
    assert_equal true, responses.pop['ok']
  end

  def test_times_out_before_command_execution
    requests = Queue.new
    responses = Queue.new
    registry = FakeRegistry.new
    requests << request(responses, deadline: 9.0)
    dispatcher = Dispatcher.new(
      request_queue: requests,
      command_registry: registry,
      ui: Object.new,
      monotonic_clock: -> { 10.0 }
    )

    dispatcher.drain_once
    response = responses.pop
    assert_empty registry.calls
    assert_equal 'TIMEOUT', response.fetch('error').fetch('code')
  end

  def test_drops_cancelled_request_without_executing
    requests = Queue.new
    responses = Queue.new
    registry = FakeRegistry.new
    requests << request(responses, deadline: 11.0, cancelled: true)
    dispatcher = Dispatcher.new(
      request_queue: requests,
      command_registry: registry,
      ui: Object.new,
      monotonic_clock: -> { 10.0 }
    )

    dispatcher.drain_once
    assert_empty registry.calls
    assert responses.empty?
  end

  private

  def request(responses, deadline:, cancelled: false)
    Bridge::Request.new(
      id: '00000000-0000-4000-8000-000000000001',
      session_id: '00000000-0000-4000-8000-000000000009',
      operation: 'system.ping',
      payload: {},
      deadline: deadline,
      response_queue: responses,
      cancelled: -> { cancelled }
    )
  end
end
