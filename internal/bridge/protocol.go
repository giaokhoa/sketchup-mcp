package bridge

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
)

const (
	ProtocolVersion       = 1
	MaxMessageBytes       = 1_048_576
	MaxDescriptorBytes    = 64 * 1024
	DefaultRequestTimeout = 5_000
)

var (
	ErrAuthentication  = errors.New("bridge authentication failed")
	ErrProtocolVersion = errors.New("unsupported bridge protocol version")
)

type Descriptor struct {
	Bridge          string `json:"bridge"`
	ProtocolVersion int    `json:"protocol_version"`
	SessionID       string `json:"session_id"`
	PID             int    `json:"pid"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Token           string `json:"token"`
	StartedAt       string `json:"started_at"`
}

func (d Descriptor) Endpoint() string {
	return net.JoinHostPort(d.Host, fmt.Sprintf("%d", d.Port))
}

func (d Descriptor) SameConnection(other Descriptor) bool {
	return d.ProtocolVersion == other.ProtocolVersion &&
		d.SessionID == other.SessionID &&
		d.Host == other.Host &&
		d.Port == other.Port &&
		d.Token == other.Token
}

func ValidateDescriptor(d Descriptor) error {
	switch {
	case d.Bridge != "sketchup-mcp":
		return fmt.Errorf("unexpected bridge %q", d.Bridge)
	case d.ProtocolVersion != ProtocolVersion:
		return fmt.Errorf("%w: got %d want %d", ErrProtocolVersion, d.ProtocolVersion, ProtocolVersion)
	case !validUUID(d.SessionID):
		return errors.New("descriptor session_id must be a UUID")
	case d.PID <= 0:
		return errors.New("descriptor pid must be positive")
	case d.Host != "127.0.0.1":
		return errors.New("descriptor endpoint must be IPv4 loopback 127.0.0.1")
	case d.Port < 1 || d.Port > 65535:
		return errors.New("descriptor port is out of range")
	case len(d.Token) < 16 || len(d.Token) > 512:
		return errors.New("descriptor token has invalid length")
	case d.StartedAt == "":
		return errors.New("descriptor started_at is required")
	default:
		return nil
	}
}

func writeFrame(w io.Writer, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal bridge frame: %w", err)
	}
	if len(payload) == 0 {
		return errors.New("bridge frame payload is empty")
	}
	if len(payload) > MaxMessageBytes {
		return fmt.Errorf("bridge frame exceeds %d bytes", MaxMessageBytes)
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func readFrame(r io.Reader, message any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("read bridge frame header: %w", err)
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return errors.New("bridge frame payload length must be positive")
	}
	if length > MaxMessageBytes {
		return fmt.Errorf("bridge frame exceeds %d bytes", MaxMessageBytes)
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("read bridge frame payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(message); err != nil {
		return fmt.Errorf("decode bridge frame: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("bridge frame contains trailing JSON")
		}
		return fmt.Errorf("decode trailing bridge frame data: %w", err)
	}
	return nil
}

func writeAll(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return fmt.Errorf("write bridge frame: %w", err)
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}
