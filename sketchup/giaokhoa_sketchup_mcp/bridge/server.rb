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
        POLL_INTERVAL_SECONDS = 0.01
        MAX_CLIENTS = 32
        MAX_ACCEPTS_PER_TICK = 8
        MAX_READS_PER_CLIENT_PER_TICK = 8
        MAX_FRAMES_PER_CLIENT_PER_TICK = 32
        MAX_WRITES_PER_CLIENT_PER_TICK = 8
        READ_CHUNK_BYTES = 64 * 1024
        HANDSHAKE_TIMEOUT_SECONDS = 5.0
        IDLE_READ_TIMEOUT_SECONDS = 120.0
        WRITE_TIMEOUT_SECONDS = 5.0
        MAX_PENDING_WRITE_BYTES = 2 * Protocol::MAX_MESSAGE_BYTES

        attr_reader :request_queue

        def initialize(session_id:, token:, discovery:, ui: nil, monotonic_clock: nil)
          @session_id = session_id
          @token = token
          @discovery = discovery
          @ui = ui || UI
          @monotonic_clock = monotonic_clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
          @request_queue = SizedQueue.new(REQUEST_QUEUE_CAPACITY)
          @clients = {}
          @running = false
          @listener = nil
          @timer_id = nil
        end

        def start
          return @listener.addr[1] if running?

          listener = TCPServer.new(LOOPBACK, 0)
          port = listener.addr[1]
          @listener = listener
          @running = true
          @discovery.publish(host: LOOPBACK, port: port)
          @timer_id = @ui.start_timer(POLL_INTERVAL_SECONDS, true) { poll_once }
          port
        rescue StandardError
          close_quietly(listener)
          @listener = nil
          @running = false
          @discovery.remove
          raise
        end

        def stop
          return unless @running || @listener || @timer_id

          @running = false
          if @timer_id
            safely { @ui.stop_timer(@timer_id) }
            @timer_id = nil
          end

          @clients.values.each(&:close)
          @clients.clear
          close_quietly(@listener)
          @listener = nil
          @discovery.remove
          nil
        end

        def running?
          @running
        end

        def poll_once
          return 0 unless @running

          now = @monotonic_clock.call
          activity = accept_pending_clients(now)
          @clients.values.each do |connection|
            activity += connection.read_pending(now)
            activity += connection.flush_pending(now)
            connection.expire_if_needed(now)
            @clients.delete(connection.socket) if connection.closed?
          end
          activity
        end

        private

        def accept_pending_clients(now)
          accepted = 0
          while accepted < MAX_ACCEPTS_PER_TICK
            socket = @listener.accept_nonblock(exception: false)
            break if socket == :wait_readable
            break unless socket

            if @clients.length >= MAX_CLIENTS
              close_quietly(socket)
              next
            end

            configure_socket(socket)
            connection = Connection.new(
              socket: socket,
              session_id: @session_id,
              token: @token,
              request_queue: @request_queue,
              monotonic_clock: @monotonic_clock,
              connected_at: now
            )
            @clients[socket] = connection
            accepted += 1
          end
          accepted
        rescue IOError, SystemCallError
          0
        end

        def configure_socket(socket)
          socket.setsockopt(Socket::IPPROTO_TCP, Socket::TCP_NODELAY, 1)
        rescue SystemCallError
          nil
        end

        def close_quietly(io)
          io&.close unless io&.closed?
        rescue IOError, SystemCallError
          nil
        end

        def safely
          yield
        rescue StandardError
          nil
        end

        class Connection
          attr_reader :socket

          def initialize(socket:, session_id:, token:, request_queue:, monotonic_clock:, connected_at:)
            @socket = socket
            @session_id = session_id
            @token = token
            @request_queue = request_queue
            @monotonic_clock = monotonic_clock
            @connected_at = connected_at
            @last_read_at = connected_at
            @last_write_progress_at = connected_at
            @read_buffer = ''.b
            @write_queue = []
            @pending_write_bytes = 0
            @outstanding = {}
            @state = :hello
            @closed = false
            @close_after_write = false
            @response_sink = ResponseSink.new(self)
          end

          def read_pending(now)
            return 0 if closed? || @close_after_write

            reads = 0
            frames = drain_frames(MAX_FRAMES_PER_CLIENT_PER_TICK)
            return frames if closed? || @close_after_write || frames >= MAX_FRAMES_PER_CLIENT_PER_TICK

            while reads < MAX_READS_PER_CLIENT_PER_TICK &&
                  frames < MAX_FRAMES_PER_CLIENT_PER_TICK
              chunk = @socket.read_nonblock(READ_CHUNK_BYTES, exception: false)
              case chunk
              when :wait_readable
                break
              when nil
                close
                break
              when String
                raise Protocol::FrameError, 'zero-length socket read' if chunk.empty?

                @last_read_at = now
                @read_buffer << chunk
                reads += 1
                frames += drain_frames(MAX_FRAMES_PER_CLIENT_PER_TICK - frames)
              else
                raise Protocol::FrameError, 'unexpected nonblocking read result'
              end
            end
            reads + frames
          rescue Protocol::FrameError, IOError, SystemCallError
            close
            0
          end

          def flush_pending(now)
            return 0 if closed?

            writes = 0
            while writes < MAX_WRITES_PER_CLIENT_PER_TICK && (item = @write_queue.first)
              remaining = item[:frame].byteslice(item[:offset], item[:frame].bytesize - item[:offset])
              result = @socket.write_nonblock(remaining, exception: false)
              case result
              when :wait_writable
                break
              when Integer
                raise IOError, 'socket write returned no bytes' unless result.positive?

                item[:offset] += result
                @pending_write_bytes -= result
                @last_write_progress_at = now
                writes += 1
                if item[:offset] == item[:frame].bytesize
                  @write_queue.shift
                  release_request_id(item[:release_id]) if item[:release_id]
                end
              else
                raise Protocol::FrameError, 'unexpected nonblocking write result'
              end
            end

            close if @close_after_write && @write_queue.empty?
            writes
          rescue Protocol::FrameError, IOError, SystemCallError
            close
            0
          end

          def expire_if_needed(now)
            return if closed?

            if @state != :ready && now - @connected_at >= HANDSHAKE_TIMEOUT_SECONDS
              close
            elsif @state == :ready && now - @last_read_at >= IDLE_READ_TIMEOUT_SECONDS
              close
            elsif !@write_queue.empty? && now - @last_write_progress_at >= WRITE_TIMEOUT_SECONDS
              close
            end
          end

          def enqueue_response(response, release:)
            return if closed?

            frame = Protocol.encode_frame(response)
            if @pending_write_bytes + frame.bytesize > MAX_PENDING_WRITE_BYTES
              close
              return
            end

            @last_write_progress_at = @monotonic_clock.call if @write_queue.empty?
            @write_queue << {
              frame: frame,
              offset: 0,
              release_id: release ? response['id'] : nil
            }
            @pending_write_bytes += frame.bytesize
            nil
          end

          def closed?
            @closed
          end

          def close
            return if @closed

            @closed = true
            @outstanding.clear
            @write_queue.clear
            @pending_write_bytes = 0
            @socket.close unless @socket.closed?
            nil
          rescue IOError, SystemCallError
            nil
          end

          private

          def drain_frames(limit)
            frames = 0
            while frames < limit
              break if @read_buffer.bytesize < 4

              length = @read_buffer.byteslice(0, 4).unpack1('N')
              raise Protocol::FrameError, 'frame payload length must be positive' if length.zero?
              raise Protocol::FrameError, 'frame payload exceeds 1 MiB' if length > Protocol::MAX_MESSAGE_BYTES

              frame_bytes = 4 + length
              break if @read_buffer.bytesize < frame_bytes

              payload = @read_buffer.byteslice(4, length)
              remainder = @read_buffer.byteslice(frame_bytes, @read_buffer.bytesize - frame_bytes)
              @read_buffer = remainder || ''.b
              handle_message(Protocol.decode_payload(payload))
              frames += 1
              break if closed? || @close_after_write
            end
            frames
          end

          def handle_message(message)
            case @state
            when :hello
              handle_hello(message)
            when :authenticate
              handle_authenticate(message)
            when :ready
              handle_request(message)
            else
              raise Protocol::FrameError, 'invalid connection state'
            end
          rescue Protocol::ValidationError => e
            if @state == :ready && e.request_id
              enqueue_response(
                Protocol.error_response(e.request_id, e.code, e.message),
                release: false
              )
            else
              close
            end
          end
          def handle_hello(message)
            Protocol.validate_hello(message)
            enqueue_response(
              {
                'type' => 'hello',
                'protocol_version' => Protocol::VERSION,
                'session_id' => @session_id,
                'server_nonce' => SecureRandom.uuid,
                'max_message_bytes' => Protocol::MAX_MESSAGE_BYTES
              },
              release: false
            )
            @state = :authenticate
          end

          def handle_authenticate(message)
            Protocol.validate_authenticate(message)
            authenticated = Protocol.secure_compare(message['token'], @token)
            if authenticated
              enqueue_response(
                {'type' => 'authenticate', 'protocol_version' => Protocol::VERSION, 'ok' => true},
                release: false
              )
              @state = :ready
            else
              enqueue_response(
                {
                  'type' => 'authenticate',
                  'protocol_version' => Protocol::VERSION,
                  'ok' => false,
                  'error' => {'code' => 'AUTH_FAILED', 'message' => 'authentication failed'}
                },
                release: false
              )
              @close_after_write = true
            end
          end
          def handle_request(message)
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

          def reserve_request_id(request_id)
            return :duplicate if @outstanding.key?(request_id)
            return :limit if @outstanding.length >= Protocol::MAX_OUTSTANDING_REQUESTS

            @outstanding[request_id] = true
            :ok
          end

          def release_request_id(request_id)
            @outstanding.delete(request_id)
          end
          class ResponseSink
            def initialize(connection)
              @connection = connection
            end

            def <<(response)
              @connection.enqueue_response(response, release: true)
              self
            end
          end
        end
      end
    end
  end
end
