# frozen_string_literal: true

require 'securerandom'
require 'socket'
require 'thread'

module Giaokhoa
  module SketchupMcp
    module Bridge
      class Server
        LOOPBACK = '127.0.0.1'
        REQUEST_QUEUE_CAPACITY = 128
        STOP_WRITER = Object.new.freeze

        attr_reader :request_queue

        def initialize(session_id:, token:, discovery:, monotonic_clock: nil)
          @session_id = session_id
          @token = token
          @discovery = discovery
          @monotonic_clock = monotonic_clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
          @request_queue = SizedQueue.new(REQUEST_QUEUE_CAPACITY)
          @mutex = Mutex.new
          @client_sockets = []
          @running = false
          @listener = nil
          @accept_thread = nil
        end

        def start
          return if running?

          listener = TCPServer.new(LOOPBACK, 0)
          port = listener.addr[1]
          @mutex.synchronize do
            @listener = listener
            @running = true
          end
          @discovery.publish(host: LOOPBACK, port: port)
          @accept_thread = background_thread { accept_loop(listener) }
          port
        rescue StandardError
          listener&.close
          @mutex.synchronize do
            @listener = nil
            @running = false
          end
          @discovery.remove
          raise
        end

        def stop
          listener = nil
          clients = []
          @mutex.synchronize do
            return unless @running

            @running = false
            listener = @listener
            @listener = nil
            clients = @client_sockets.dup
            @client_sockets.clear
          end

          close_quietly(listener)
          clients.each { |socket| close_quietly(socket) }
          @discovery.remove
          nil
        end

        def running?
          @mutex.synchronize { @running }
        end

        private

        def accept_loop(listener)
          loop do
            socket = listener.accept
            unless running?
              close_quietly(socket)
              break
            end

            configure_socket(socket)
            register_client(socket)
            background_thread do
              begin
                Connection.new(
                  socket: socket,
                  session_id: @session_id,
                  token: @token,
                  request_queue: @request_queue,
                  monotonic_clock: @monotonic_clock
                ).run
              ensure
                unregister_client(socket)
                close_quietly(socket)
              end
            end
          rescue IOError, Errno::EBADF
            break
          rescue SystemCallError
            break unless running?
          end
        ensure
          close_quietly(listener)
        end

        def configure_socket(socket)
          socket.setsockopt(Socket::IPPROTO_TCP, Socket::TCP_NODELAY, 1)
        rescue SystemCallError
          nil
        end

        def register_client(socket)
          @mutex.synchronize do
            if @running
              @client_sockets << socket
            else
              close_quietly(socket)
            end
          end
        end

        def unregister_client(socket)
          @mutex.synchronize { @client_sockets.delete(socket) }
        end

        def background_thread(&block)
          Thread.new(&block).tap { |thread| thread.report_on_exception = false }
        end

        def close_quietly(io)
          io&.close unless io&.closed?
        rescue IOError, SystemCallError
          nil
        end

        class Connection
          def initialize(socket:, session_id:, token:, request_queue:, monotonic_clock:)
            @socket = socket
            @session_id = session_id
            @token = token
            @request_queue = request_queue
            @monotonic_clock = monotonic_clock
            @wire_queue = Queue.new
            @response_sink = ResponseSink.new(self)
            @mutex = Mutex.new
            @outstanding = {}
            @closed = false
            @writer_thread = nil
          end

          def run
            return unless handshake

            @writer_thread = Thread.new { writer_loop }
            @writer_thread.report_on_exception = false
            request_loop
          rescue Protocol::FrameError, IOError, SystemCallError
            nil
          ensure
            close
          end

          private

          def handshake
            hello = Protocol.read_frame(@socket)
            return false unless hello

            Protocol.validate_hello(hello)
            Protocol.write_frame(
              @socket,
              {
                'type' => 'hello',
                'protocol_version' => Protocol::VERSION,
                'session_id' => @session_id,
                'server_nonce' => SecureRandom.uuid,
                'max_message_bytes' => Protocol::MAX_MESSAGE_BYTES
              }
            )

            auth = Protocol.read_frame(@socket)
            return false unless auth

            Protocol.validate_authenticate(auth)
            authenticated = Protocol.secure_compare(auth['token'], @token)
            if authenticated
              Protocol.write_frame(
                @socket,
                {'type' => 'authenticate', 'protocol_version' => Protocol::VERSION, 'ok' => true}
              )
              true
            else
              Protocol.write_frame(
                @socket,
                {
                  'type' => 'authenticate',
                  'protocol_version' => Protocol::VERSION,
                  'ok' => false,
                  'error' => {'code' => 'AUTH_FAILED', 'message' => 'authentication failed'}
                }
              )
              false
            end
          rescue Protocol::ValidationError
            false
          end

          def request_loop
            until closed?
              message = Protocol.read_frame(@socket)
              break unless message

              handle_message(message)
            end
          end

          def handle_message(message)
            request = Protocol.validate_request(message)
            request_id = request.fetch('id')

            reservation = reserve_request_id(request_id)
            case reservation
            when :duplicate
              enqueue_response(
                Protocol.error_response(request_id, 'INVALID_REQUEST', 'duplicate request id'),
                release: false
              )
              return
            when :limit
              enqueue_response(
                Protocol.error_response(
                  request_id,
                  'INVALID_REQUEST',
                  'too many outstanding requests'
                ),
                release: false
              )
              return
            end

            unless request['session_id'] == @session_id
              enqueue_response(
                Protocol.error_response(
                  request_id,
                  'SESSION_NOT_FOUND',
                  'request session does not match this bridge instance'
                ),
                release: true
              )
              return
            end

            deadline = @monotonic_clock.call + (request.fetch('timeout_ms') / 1000.0)
            work = Request.new(
              id: request_id,
              session_id: @session_id,
              operation: request.fetch('operation'),
              payload: request.fetch('payload'),
              deadline: deadline,
              response_queue: @response_sink,
              cancelled: -> { closed? }
            )
            @request_queue.push(work, true)
          rescue Protocol::ValidationError => e
            if e.request_id
              enqueue_response(
                Protocol.error_response(e.request_id, e.code, e.message),
                release: false
              )
            else
              raise Protocol::FrameError, e.message
            end
          rescue ThreadError
            enqueue_response(
              Protocol.error_response(
                request_id,
                'INTERNAL',
                'bridge request queue is full'
              ),
              release: true
            )
          end

          def writer_loop
            loop do
              item = @wire_queue.pop
              break if item.equal?(STOP_WRITER)

              response, release = item
              Protocol.write_frame(@socket, response)
              release_request_id(response['id']) if release
            end
          rescue Protocol::FrameError, IOError, SystemCallError
            nil
          ensure
            mark_closed
            close_socket
          end


          def enqueue_response(response, release:)
            @wire_queue << [response, release]
            nil
          end

          public :enqueue_response

          class ResponseSink
            def initialize(connection)
              @connection = connection
            end

            def <<(response)
              @connection.enqueue_response(response, release: true)
              self
            end
          end

          def reserve_request_id(request_id)
            @mutex.synchronize do
              return :duplicate if @outstanding.key?(request_id)
              return :limit if @outstanding.length >= Protocol::MAX_OUTSTANDING_REQUESTS

              @outstanding[request_id] = true
              :ok
            end
          end

          def release_request_id(request_id)
            @mutex.synchronize { @outstanding.delete(request_id) }
          end

          def closed?
            @mutex.synchronize { @closed }
          end

          def mark_closed
            @mutex.synchronize do
              @closed = true
              @outstanding.clear
            end
          end

          def close
            first_close = @mutex.synchronize do
              already_closed = @closed
              @closed = true
              @outstanding.clear
              !already_closed
            end
            @wire_queue << STOP_WRITER if first_close
            close_socket
            nil
          end

          def close_socket
            @socket.close unless @socket.closed?
          rescue IOError, SystemCallError
            nil
          end
        end
      end
    end
  end
end
