# frozen_string_literal: true

require 'json'
require 'minitest/autorun'
require 'securerandom'
require 'socket'
require 'tmpdir'
require 'timeout'

require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/protocol'
require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/request'
require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/discovery'
require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/server'
require_relative '../sketchup/giaokhoa_sketchup_mcp/dispatcher/main_thread_dispatcher'

class ServerIntegrationTest < Minitest::Test
  Bridge = Giaokhoa::SketchupMcp::Bridge
  Dispatcher = Giaokhoa::SketchupMcp::Dispatcher::MainThreadDispatcher

  class FakeUI
    attr_reader :timer

    def start_timer(_interval, _repeat, &block)
      @timer = block
      1
    end

    def stop_timer(_id)
      @timer = nil
      nil
    end
  end

  class PingRegistry
    def call(operation, payload)
      return {ok: false, error: {code: 'INVALID_REQUEST', message: 'bad command'}} unless operation == 'system.ping'
      return {ok: false, error: {code: 'INVALID_REQUEST', message: 'bad payload'}} unless payload.empty?

      {ok: true, payload: {'pong' => true}}
    end
  end

  def test_authenticated_request_round_trips_through_dispatch_queue
    Dir.mktmpdir do |local_app_data|
      session_id = SecureRandom.uuid
      token = SecureRandom.urlsafe_base64(32, false)
      discovery = Bridge::Discovery.new(
        session_id: session_id,
        token: token,
        local_app_data: local_app_data
      )
      ui = FakeUI.new
      server = Bridge::Server.new(
        session_id: session_id,
        token: token,
        discovery: discovery,
        ui: ui
      )
      dispatcher = Dispatcher.new(
        request_queue: server.request_queue,
        command_registry: PingRegistry.new,
        ui: Object.new
      )
      port = server.start
      descriptor = File.join(local_app_data, 'SketchUpMCP', 'sessions', "#{session_id}.json")
      assert File.file?(descriptor)
      refute_nil ui.timer

      socket = TCPSocket.new('127.0.0.1', port)
      write(socket, 'type' => 'hello', 'protocol_version' => 1, 'client' => 'test', 'client_nonce' => SecureRandom.uuid)
      hello = read_after_poll(server, socket)
      assert_equal session_id, hello.fetch('session_id')

      write(socket, 'type' => 'authenticate', 'protocol_version' => 1, 'token' => token)
      assert read_after_poll(server, socket).fetch('ok')

      request_id = SecureRandom.uuid
      write(
        socket,
        'type' => 'request',
        'protocol_version' => 1,
        'id' => request_id,
        'session_id' => session_id,
        'operation' => 'system.ping',
        'timeout_ms' => 5_000,
        'payload' => {}
      )

      Timeout.timeout(2) do
        loop do
          server.poll_once
          break if dispatcher.drain_once.positive?
          sleep 0.001
        end
      end
      response = read_after_poll(server, socket)
      assert_equal request_id, response.fetch('id')
      assert response.fetch('ok')
      assert_equal true, response.fetch('payload').fetch('pong')

      socket.close
      server.stop
      refute File.exist?(descriptor)
      assert_nil ui.timer
    ensure
      socket&.close unless socket&.closed?
      server&.stop
    end
  end

  def test_duplicate_id_is_rejected_before_session_mismatch
    Dir.mktmpdir do |local_app_data|
      session_id = SecureRandom.uuid
      token = SecureRandom.urlsafe_base64(32, false)
      discovery = Bridge::Discovery.new(
        session_id: session_id,
        token: token,
        local_app_data: local_app_data
      )
      server = Bridge::Server.new(
        session_id: session_id,
        token: token,
        discovery: discovery,
        ui: FakeUI.new
      )
      dispatcher = Dispatcher.new(
        request_queue: server.request_queue,
        command_registry: PingRegistry.new,
        ui: Object.new
      )
      port = server.start
      socket = TCPSocket.new('127.0.0.1', port)

      write(socket, 'type' => 'hello', 'protocol_version' => 1, 'client' => 'test', 'client_nonce' => SecureRandom.uuid)
      read_after_poll(server, socket)
      write(socket, 'type' => 'authenticate', 'protocol_version' => 1, 'token' => token)
      read_after_poll(server, socket)

      request_id = SecureRandom.uuid
      request = {
        'type' => 'request',
        'protocol_version' => 1,
        'id' => request_id,
        'session_id' => session_id,
        'operation' => 'system.ping',
        'timeout_ms' => 5_000,
        'payload' => {}
      }
      write(socket, request)
      write(socket, request.merge('session_id' => SecureRandom.uuid))

      duplicate = read_after_poll(server, socket)
      refute duplicate.fetch('ok')
      assert_equal 'INVALID_REQUEST', duplicate.fetch('error').fetch('code')
      assert_equal 'duplicate request id', duplicate.fetch('error').fetch('message')

      assert_equal 1, dispatcher.drain_once
      original = read_after_poll(server, socket)
      assert original.fetch('ok')
      assert_equal request_id, original.fetch('id')
    ensure
      socket&.close unless socket&.closed?
      server&.stop
    end
  end


  def test_buffered_frame_budget_continues_across_ticks_without_new_socket_data
    Dir.mktmpdir do |local_app_data|
      session_id = SecureRandom.uuid
      token = SecureRandom.urlsafe_base64(32, false)
      discovery = Bridge::Discovery.new(
        session_id: session_id,
        token: token,
        local_app_data: local_app_data
      )
      server = Bridge::Server.new(
        session_id: session_id,
        token: token,
        discovery: discovery,
        ui: FakeUI.new
      )
      dispatcher = Dispatcher.new(
        request_queue: server.request_queue,
        command_registry: PingRegistry.new,
        ui: Object.new
      )
      port = server.start
      socket = TCPSocket.new('127.0.0.1', port)

      write(socket, 'type' => 'hello', 'protocol_version' => 1, 'client' => 'test', 'client_nonce' => SecureRandom.uuid)
      read_after_poll(server, socket)
      write(socket, 'type' => 'authenticate', 'protocol_version' => 1, 'token' => token)
      read_after_poll(server, socket)

      request_ids = Array.new(40) { SecureRandom.uuid }
      burst = request_ids.map do |request_id|
        Bridge::Protocol.encode_frame(
          'type' => 'request',
          'protocol_version' => 1,
          'id' => request_id,
          'session_id' => session_id,
          'operation' => 'system.ping',
          'timeout_ms' => 5_000,
          'payload' => {}
        )
      end.join
      socket.write(burst)

      responses = {}
      Timeout.timeout(2) do
        until responses.length == request_ids.length
          server.poll_once
          dispatcher.drain_once
          server.poll_once

          while IO.select([socket], nil, nil, 0)
            response = Bridge::Protocol.read_frame(socket, timeout: 0.5)
            responses[response.fetch('id')] = response
          end
          sleep 0.001
        end
      end

      assert_equal request_ids.sort, responses.keys.sort
    ensure
      socket&.close unless socket&.closed?
      server&.stop
    end
  end

  private

  def write(socket, message)
    Bridge::Protocol.write_frame(socket, message)
  end

  def read_after_poll(server, socket)
    Timeout.timeout(2) do
      loop do
        server.poll_once
        ready = IO.select([socket], nil, nil, 0)
        return Bridge::Protocol.read_frame(socket, timeout: 0.5) if ready
        sleep 0.001
      end
    end
  end
end
