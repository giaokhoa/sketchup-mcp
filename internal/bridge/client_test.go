package bridge

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

const (
	testSessionID = "11111111-1111-4111-8111-111111111111"
	testToken     = "0123456789abcdef0123456789abcdef"
	testNonce     = "22222222-2222-4222-8222-222222222222"
)

func TestDialRejectsWrongSecret(t *testing.T) {
	descriptor, stop := startTestBridge(t, func(conn net.Conn) {
		if !serverHandshake(t, conn, testToken, ProtocolVersion) {
			return
		}
	})
	defer stop()
	descriptor.Token = "fedcba9876543210fedcba9876543210"

	client, err := Dial(context.Background(), descriptor, time.Second)
	if client != nil {
		_ = client.Close()
	}
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Dial() error = %v, want ErrAuthentication", err)
	}
}

func TestDialRejectsWrongProtocolVersion(t *testing.T) {
	descriptor, stop := startTestBridge(t, func(conn net.Conn) {
		var hello helloRequest
		if err := readFrame(conn, &hello); err != nil {
			return
		}
		_ = writeFrame(conn, helloResponse{
			Type: "hello", ProtocolVersion: 99, SessionID: testSessionID,
			ServerNonce: testNonce, MaxMessageBytes: MaxMessageBytes,
		})
	})
	defer stop()

	client, err := Dial(context.Background(), descriptor, time.Second)
	if client != nil {
		_ = client.Close()
	}
	if !errors.Is(err, ErrProtocolVersion) {
		t.Fatalf("Dial() error = %v, want ErrProtocolVersion", err)
	}
}

func TestSessionInfoRejectsOversizedFrame(t *testing.T) {
	descriptor, stop := startTestBridge(t, func(conn net.Conn) {
		if !serverHandshake(t, conn, testToken, ProtocolVersion) {
			return
		}
		var request requestMessage
		if err := readFrame(conn, &request); err != nil {
			return
		}
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], MaxMessageBytes+1)
		_, _ = conn.Write(header[:])
	})
	defer stop()

	client, err := Dial(context.Background(), descriptor, time.Second)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	if _, err := client.SessionInfo(context.Background()); err == nil {
		t.Fatal("SessionInfo() error = nil, want oversized-frame rejection")
	}
}

func TestSessionInfoRejectsMalformedJSON(t *testing.T) {
	descriptor, stop := startTestBridge(t, func(conn net.Conn) {
		if !serverHandshake(t, conn, testToken, ProtocolVersion) {
			return
		}
		var request requestMessage
		if err := readFrame(conn, &request); err != nil {
			return
		}
		payload := []byte("{not-json")
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
		_, _ = conn.Write(header[:])
		_, _ = conn.Write(payload)
	})
	defer stop()

	client, err := Dial(context.Background(), descriptor, time.Second)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	if _, err := client.SessionInfo(context.Background()); err == nil {
		t.Fatal("SessionInfo() error = nil, want malformed-JSON rejection")
	}
}

func startTestBridge(t *testing.T, handler func(net.Conn)) (Descriptor, func()) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()

	descriptor := Descriptor{
		Bridge: "sketchup-mcp", ProtocolVersion: ProtocolVersion,
		SessionID: testSessionID, PID: 1234, Host: "127.0.0.1",
		Port: port, Token: testToken, StartedAt: "2026-09-15T08:00:00Z",
	}
	stop := func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("fake bridge did not stop")
		}
	}
	return descriptor, stop
}

func serverHandshake(t *testing.T, conn net.Conn, expectedToken string, version int) bool {
	t.Helper()
	var hello helloRequest
	if err := readFrame(conn, &hello); err != nil {
		return false
	}
	if err := writeFrame(conn, helloResponse{
		Type: "hello", ProtocolVersion: version, SessionID: testSessionID,
		ServerNonce: testNonce, MaxMessageBytes: MaxMessageBytes,
	}); err != nil {
		return false
	}

	var auth authenticateRequest
	if err := readFrame(conn, &auth); err != nil {
		return false
	}
	ok := auth.Token == expectedToken
	if err := writeFrame(conn, authenticateResponse{
		Type: "authenticate", ProtocolVersion: version, OK: ok,
	}); err != nil {
		return false
	}
	return ok
}
