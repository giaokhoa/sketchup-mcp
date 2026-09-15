# frozen_string_literal: true

require 'minitest/autorun'
require 'stringio'

require_relative '../sketchup/giaokhoa_sketchup_mcp/bridge/protocol'

class ProtocolTest < Minitest::Test
  Protocol = Giaokhoa::SketchupMcp::Bridge::Protocol

  def test_frame_round_trip
    message = {'type' => 'hello', 'protocol_version' => 1, 'client' => 'test', 'client_nonce' => uuid(1)}
    frame = Protocol.encode_frame(message)

    assert_equal message, Protocol.read_frame(StringIO.new(frame))
  end

  def test_rejects_oversized_frame_before_payload_read
    io = StringIO.new([Protocol::MAX_MESSAGE_BYTES + 1].pack('N'))

    assert_raises(Protocol::FrameError) { Protocol.read_frame(io) }
  end

  def test_request_rejects_unknown_fields_with_correlation_id
    request_id = uuid(2)
    message = valid_request(request_id).merge('unexpected' => true)

    error = assert_raises(Protocol::ValidationError) { Protocol.validate_request(message) }
    assert_equal request_id, error.request_id
  end

  def test_unsupported_protocol_version_has_specific_error_code
    message = valid_request(uuid(5)).merge('protocol_version' => 99)

    error = assert_raises(Protocol::ValidationError) { Protocol.validate_request(message) }
    assert_equal 'UNSUPPORTED_PROTOCOL', error.code
  end

  def test_timeout_is_clamped_to_protocol_bounds
    low = Protocol.validate_request(valid_request(uuid(3)).merge('timeout_ms' => 1))
    high = Protocol.validate_request(valid_request(uuid(4)).merge('timeout_ms' => 999_999))

    assert_equal 100, low['timeout_ms']
    assert_equal 30_000, high['timeout_ms']
  end

  def test_secure_compare
    assert Protocol.secure_compare('same-token', 'same-token')
    refute Protocol.secure_compare('same-token', 'other-token')
    refute Protocol.secure_compare('short', 'longer')
  end

  private

  def valid_request(id)
    {
      'type' => 'request',
      'protocol_version' => 1,
      'id' => id,
      'session_id' => uuid(9),
      'operation' => 'system.ping',
      'timeout_ms' => 5_000,
      'payload' => {}
    }
  end

  def uuid(seed)
    format('00000000-0000-4000-8000-%012d', seed)
  end
end
