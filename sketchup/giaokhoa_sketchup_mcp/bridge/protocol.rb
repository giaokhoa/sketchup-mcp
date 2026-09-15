# frozen_string_literal: true

require 'json'

module Giaokhoa
  module SketchupMcp
    module Bridge
      module Protocol
        VERSION = 1
        MAX_MESSAGE_BYTES = 1_048_576
        MAX_OUTSTANDING_REQUESTS = 32
        MIN_TIMEOUT_MS = 100
        MAX_TIMEOUT_MS = 30_000
        UUID_PATTERN = /\A[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\z/i.freeze

        class FrameError < StandardError; end
        class TimeoutError < FrameError; end

        class ValidationError < StandardError
          attr_reader :code, :request_id

          def initialize(message, request_id = nil, code = 'INVALID_REQUEST')
            super(message)
            @request_id = request_id
            @code = code
          end
        end

        module_function

        def encode_frame(message)
          payload = JSON.generate(message).encode(Encoding::UTF_8)
          raise FrameError, 'frame payload is empty' if payload.empty?
          raise FrameError, 'frame payload exceeds 1 MiB' if payload.bytesize > MAX_MESSAGE_BYTES

          [payload.bytesize].pack('N') + payload
        end

        def write_frame(io, message, timeout: nil)
          frame = encode_frame(message)
          if timeout && io.respond_to?(:write_nonblock)
            write_nonblocking(io, frame, monotonic_deadline(timeout))
          else
            write_blocking(io, frame)
          end
          io.flush if io.respond_to?(:flush)
          nil
        end

        def read_frame(io, timeout: nil)
          deadline = timeout && monotonic_deadline(timeout)
          header = read_exact(io, 4, deadline)
          return nil if header.nil?

          length = header.unpack1('N')
          raise FrameError, 'frame payload length must be positive' if length.zero?
          raise FrameError, 'frame payload exceeds 1 MiB' if length > MAX_MESSAGE_BYTES

          payload = read_exact(io, length, deadline)
          raise FrameError, 'unexpected EOF while reading frame payload' if payload.nil?

          payload.force_encoding(Encoding::UTF_8)
          raise FrameError, 'frame payload is not valid UTF-8' unless payload.valid_encoding?

          message = JSON.parse(payload)
          raise FrameError, 'frame JSON must be an object' unless message.is_a?(Hash)

          message
        rescue JSON::ParserError => e
          raise FrameError, "invalid JSON: #{e.message}"
        end

        def validate_hello(message)
          exact_keys!(message, %w[type protocol_version client client_nonce])
          require_type!(message, 'type', String)
          raise ValidationError, 'expected hello message' unless message['type'] == 'hello'
          require_protocol_version!(message)
          bounded_string!(message, 'client', 1, 128)
          uuid_string!(message, 'client_nonce')
          message
        end

        def validate_authenticate(message)
          exact_keys!(message, %w[type protocol_version token])
          require_type!(message, 'type', String)
          raise ValidationError, 'expected authenticate message' unless message['type'] == 'authenticate'
          require_protocol_version!(message)
          bounded_string!(message, 'token', 16, 512)
          message
        end

        def validate_request(message)
          request_id = candidate_request_id(message)
          exact_keys!(message, %w[type protocol_version id session_id operation timeout_ms payload], request_id)
          require_type!(message, 'type', String, request_id)
          raise ValidationError.new('expected request message', request_id) unless message['type'] == 'request'
          require_protocol_version!(message, request_id)
          uuid_string!(message, 'id', request_id)
          uuid_string!(message, 'session_id', request_id)
          bounded_string!(message, 'operation', 1, 128, request_id)
          require_type!(message, 'timeout_ms', Integer, request_id)
          require_type!(message, 'payload', Hash, request_id)

          normalized = message.dup
          normalized['timeout_ms'] = [[message['timeout_ms'], MIN_TIMEOUT_MS].max, MAX_TIMEOUT_MS].min
          normalized
        end

        def success_response(request_id, payload)
          {
            'type' => 'response',
            'protocol_version' => VERSION,
            'id' => request_id,
            'ok' => true,
            'payload' => payload
          }
        end

        def error_response(request_id, code, message, details = nil)
          error = {'code' => code, 'message' => message}
          error['details'] = details if details
          {
            'type' => 'response',
            'protocol_version' => VERSION,
            'id' => request_id,
            'ok' => false,
            'error' => error
          }
        end

        def secure_compare(left, right)
          return false unless left.is_a?(String) && right.is_a?(String)
          return false unless left.bytesize == right.bytesize

          diff = 0
          left.bytes.zip(right.bytes) { |a, b| diff |= a ^ b }
          diff.zero?
        end

        def read_exact(io, length, deadline = nil)
          if deadline && io.respond_to?(:read_nonblock)
            read_nonblocking(io, length, deadline)
          else
            read_blocking(io, length)
          end
        end
        private_class_method :read_exact

        def read_blocking(io, length)
          buffer = +''
          buffer.force_encoding(Encoding::BINARY)
          while buffer.bytesize < length
            chunk = io.read(length - buffer.bytesize)
            return nil if chunk.nil? && buffer.empty?
            raise FrameError, 'unexpected EOF while reading frame' if chunk.nil?
            raise FrameError, 'zero-length read while reading frame' if chunk.empty?

            buffer << chunk
          end
          buffer
        end
        private_class_method :read_blocking

        def read_nonblocking(io, length, deadline)
          buffer = +''
          buffer.force_encoding(Encoding::BINARY)
          while buffer.bytesize < length
            chunk = io.read_nonblock(length - buffer.bytesize, exception: false)
            case chunk
            when :wait_readable
              wait_for_io(io, :read, deadline)
            when nil
              return nil if buffer.empty?

              raise FrameError, 'unexpected EOF while reading frame'
            when String
              raise FrameError, 'zero-length read while reading frame' if chunk.empty?

              buffer << chunk
            else
              raise FrameError, 'unexpected nonblocking read result'
            end
          end
          buffer
        end
        private_class_method :read_nonblocking

        def write_blocking(io, frame)
          offset = 0
          while offset < frame.bytesize
            written = io.write(frame.byteslice(offset, frame.bytesize - offset))
            raise IOError, 'socket write returned no bytes' unless written && written.positive?

            offset += written
          end
        end
        private_class_method :write_blocking

        def write_nonblocking(io, frame, deadline)
          offset = 0
          while offset < frame.bytesize
            written = io.write_nonblock(
              frame.byteslice(offset, frame.bytesize - offset),
              exception: false
            )
            case written
            when :wait_writable
              wait_for_io(io, :write, deadline)
            when Integer
              raise IOError, 'socket write returned no bytes' unless written.positive?

              offset += written
            else
              raise FrameError, 'unexpected nonblocking write result'
            end
          end
        end
        private_class_method :write_nonblocking

        def monotonic_deadline(timeout)
          timeout = Float(timeout)
          raise ArgumentError, 'timeout must be positive' unless timeout.positive?

          Process.clock_gettime(Process::CLOCK_MONOTONIC) + timeout
        end
        private_class_method :monotonic_deadline

        def wait_for_io(io, direction, deadline)
          remaining = deadline - Process.clock_gettime(Process::CLOCK_MONOTONIC)
          raise TimeoutError, "#{direction} deadline exceeded" unless remaining.positive?

          ready =
            if direction == :read
              IO.select([io], nil, nil, remaining)
            else
              IO.select(nil, [io], nil, remaining)
            end
          raise TimeoutError, "#{direction} deadline exceeded" unless ready
        end
        private_class_method :wait_for_io

        def exact_keys!(message, expected, request_id = nil)
          actual = message.keys.sort
          wanted = expected.sort
          return if actual == wanted

          raise ValidationError.new('message fields do not match protocol schema', request_id)
        end
        private_class_method :exact_keys!

        def require_protocol_version!(message, request_id = nil)
          require_type!(message, 'protocol_version', Integer, request_id)
          return if message['protocol_version'] == VERSION

          raise ValidationError.new('unsupported protocol version', request_id, 'UNSUPPORTED_PROTOCOL')
        end
        private_class_method :require_protocol_version!

        def require_type!(message, key, klass, request_id = nil)
          return if message[key].is_a?(klass)

          raise ValidationError.new("#{key} has invalid type", request_id)
        end
        private_class_method :require_type!

        def bounded_string!(message, key, min, max, request_id = nil)
          require_type!(message, key, String, request_id)
          length = message[key].bytesize
          return if length >= min && length <= max

          raise ValidationError.new("#{key} has invalid length", request_id)
        end
        private_class_method :bounded_string!

        def uuid_string!(message, key, request_id = nil)
          bounded_string!(message, key, 36, 36, request_id)
          return if UUID_PATTERN.match?(message[key])

          raise ValidationError.new("#{key} must be a UUID", request_id)
        end
        private_class_method :uuid_string!

        def candidate_request_id(message)
          return nil unless message.is_a?(Hash)

          value = message['id']
          value if value.is_a?(String) && UUID_PATTERN.match?(value)
        end
        private_class_method :candidate_request_id
      end
    end
  end
end
