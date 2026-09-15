package sessions

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giaokhoa/sketchup-mcp/internal/bridge"
)

func TestRegistryListsOneFakeBridgeSession(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "11111111-1111-4111-8111-111111111111", "One")
	defer fake.stop(true)

	registry := testRegistry(directory)
	defer registry.Close()

	output, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(output.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want one", output.Sessions)
	}
	session := output.Sessions[0]
	if session.SessionID != fake.descriptor.SessionID {
		t.Fatalf("session_id = %q, want %q", session.SessionID, fake.descriptor.SessionID)
	}
	if session.PID != fake.descriptor.PID {
		t.Fatalf("pid = %d, want %d", session.PID, fake.descriptor.PID)
	}
	if session.SketchUpVersion != "26.0.429" {
		t.Fatalf("sketchup_version = %q", session.SketchUpVersion)
	}
	if session.Model.Title != "One" || session.Model.GUID == "" {
		t.Fatalf("model = %#v", session.Model)
	}
	if session.BridgeEndpoint != fake.descriptor.Endpoint() {
		t.Fatalf("bridge_endpoint = %q, want %q", session.BridgeEndpoint, fake.descriptor.Endpoint())
	}
	if session.LastSeenAt == "" {
		t.Fatal("last_seen_at is empty")
	}
}

func TestRegistryListsMultipleSessions(t *testing.T) {
	directory := t.TempDir()
	first := newFakeBridge(t, directory, "11111111-1111-4111-8111-111111111111", "One")
	defer first.stop(true)
	second := newFakeBridge(t, directory, "22222222-2222-4222-8222-222222222222", "Two")
	defer second.stop(true)

	registry := testRegistry(directory)
	defer registry.Close()

	output, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(output.Sessions) != 2 {
		t.Fatalf("sessions = %#v, want two", output.Sessions)
	}
	if output.Sessions[0].SessionID == output.Sessions[1].SessionID {
		t.Fatalf("duplicate logical session: %#v", output.Sessions)
	}
}

func TestRegistryRejectsWrongSecret(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "33333333-3333-4333-8333-333333333333", "Secret")
	defer fake.stop(true)

	bad := fake.descriptor
	bad.Token = "ffffffffffffffffffffffffffffffff"
	writeDescriptor(t, directory, bad)

	registry := testRegistry(directory)
	defer registry.Close()

	output, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(output.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for wrong secret", output.Sessions)
	}
}

func TestRegistryRejectsWrongProtocolVersion(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "44444444-4444-4444-8444-444444444444", "Version")
	defer fake.stop(true)

	bad := fake.descriptor
	bad.ProtocolVersion = 99
	writeDescriptor(t, directory, bad)

	registry := testRegistry(directory)
	defer registry.Close()

	output, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(output.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for wrong protocol", output.Sessions)
	}
}

func TestRegistryRemovesDeadSessionEvenWhenDescriptorRemains(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "55555555-5555-4555-8555-555555555555", "Dead")

	registry := testRegistry(directory)
	defer registry.Close()

	first, err := registry.List(context.Background())
	if err != nil || len(first.Sessions) != 1 {
		t.Fatalf("first List() = %#v, %v", first, err)
	}

	fake.stop(false)
	second, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want stale session removed", second.Sessions)
	}
	_ = os.Remove(filepath.Join(directory, fake.descriptor.SessionID+".json"))
}

func TestRegistryReconnectsWithoutDuplicateLogicalSession(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "66666666-6666-4666-8666-666666666666", "Reconnect")
	defer fake.stop(true)

	registry := testRegistry(directory)
	defer registry.Close()

	first, err := registry.List(context.Background())
	if err != nil || len(first.Sessions) != 1 {
		t.Fatalf("first List() = %#v, %v", first, err)
	}
	fake.closeConnections()

	second, err := registry.List(context.Background())
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(second.Sessions) != 1 {
		t.Fatalf("sessions = %#v, want one after reconnect", second.Sessions)
	}
	if second.Sessions[0].SessionID != first.Sessions[0].SessionID {
		t.Fatalf("session changed across reconnect: %#v -> %#v", first.Sessions, second.Sessions)
	}
	if fake.accepts.Load() < 2 {
		t.Fatalf("accept count = %d, want reconnect", fake.accepts.Load())
	}
}

func TestRegistryConcurrentListIsRaceFree(t *testing.T) {
	directory := t.TempDir()
	fake := newFakeBridge(t, directory, "77777777-7777-4777-8777-777777777777", "Concurrent")
	defer fake.stop(true)

	registry := testRegistry(directory)
	defer registry.Close()
	if _, err := registry.List(context.Background()); err != nil {
		t.Fatalf("warm List() error = %v", err)
	}

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			output, err := registry.List(context.Background())
			if err != nil {
				errs <- err
				return
			}
			if len(output.Sessions) != 1 {
				errs <- fmt.Errorf("got %d sessions", len(output.Sessions))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func testRegistry(directory string) *Registry {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRegistry(directory, logger, 250*time.Millisecond)
}

type fakeBridge struct {
	descriptor     bridge.Descriptor
	descriptorPath string
	info           bridge.SessionInfo
	token          string
	listener       net.Listener
	accepts        atomic.Int32

	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	stopped bool
	done    chan struct{}
}

func newFakeBridge(t *testing.T, directory, sessionID, title string) *fakeBridge {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	token := "0123456789abcdef0123456789abcdef"
	fake := &fakeBridge{
		descriptor: bridge.Descriptor{
			Bridge: "sketchup-mcp", ProtocolVersion: bridge.ProtocolVersion,
			SessionID: sessionID, PID: 4321, Host: "127.0.0.1", Port: port,
			Token: token, StartedAt: "2026-09-15T08:00:00Z",
		},
		info: bridge.SessionInfo{
			SessionID: sessionID, PID: 4321, SketchUpVersion: "26.0.429",
			Model: bridge.ModelInfo{
				GUID:  "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
				Title: title, Revision: 7,
			},
		},
		descriptorPath: filepath.Join(directory, sessionID+".json"),
		token:          token, listener: listener, conns: make(map[net.Conn]struct{}), done: make(chan struct{}),
	}
	writeDescriptor(t, directory, fake.descriptor)
	go fake.acceptLoop()
	return fake
}

func (f *fakeBridge) acceptLoop() {
	defer close(f.done)
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.accepts.Add(1)
		f.mu.Lock()
		f.conns[conn] = struct{}{}
		f.mu.Unlock()
		go f.handle(conn)
	}
}

func (f *fakeBridge) handle(conn net.Conn) {
	defer func() {
		f.mu.Lock()
		delete(f.conns, conn)
		f.mu.Unlock()
		_ = conn.Close()
	}()

	var hello map[string]any
	if readTestFrame(conn, &hello) != nil {
		return
	}
	if writeTestFrame(conn, map[string]any{
		"type": "hello", "protocol_version": bridge.ProtocolVersion,
		"session_id":        f.descriptor.SessionID,
		"server_nonce":      "88888888-8888-4888-8888-888888888888",
		"max_message_bytes": bridge.MaxMessageBytes,
	}) != nil {
		return
	}

	var auth map[string]any
	if readTestFrame(conn, &auth) != nil {
		return
	}
	authToken, _ := auth["token"].(string)
	ok := authToken == f.token
	if writeTestFrame(conn, map[string]any{
		"type": "authenticate", "protocol_version": bridge.ProtocolVersion, "ok": ok,
	}) != nil || !ok {
		return
	}

	for {
		var request struct {
			Type      string         `json:"type"`
			ID        string         `json:"id"`
			SessionID string         `json:"session_id"`
			Operation string         `json:"operation"`
			Payload   map[string]any `json:"payload"`
		}
		if readTestFrame(conn, &request) != nil {
			return
		}
		if request.Operation != "session.info" {
			return
		}
		if writeTestFrame(conn, map[string]any{
			"type": "response", "protocol_version": bridge.ProtocolVersion,
			"id": request.ID, "ok": true, "payload": f.info,
		}) != nil {
			return
		}
	}
}

func (f *fakeBridge) closeConnections() {
	f.mu.Lock()
	conns := make([]net.Conn, 0, len(f.conns))
	for conn := range f.conns {
		conns = append(conns, conn)
	}
	f.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (f *fakeBridge) stop(removeDescriptor bool) {
	f.mu.Lock()
	if f.stopped {
		f.mu.Unlock()
		return
	}
	f.stopped = true
	f.mu.Unlock()

	_ = f.listener.Close()
	f.closeConnections()
	select {
	case <-f.done:
	case <-time.After(time.Second):
	}
	if removeDescriptor {
		_ = os.Remove(f.descriptorPath)
	}
}

func writeDescriptor(t *testing.T, directory string, descriptor bridge.Descriptor) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	path := filepath.Join(directory, descriptor.SessionID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writeTestFrame(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func readTestFrame(r io.Reader, value any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > bridge.MaxMessageBytes {
		return fmt.Errorf("invalid frame size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}
