package app

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/giaokhoa/sketchup-mcp/internal/bridge"
	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pipelineSessionID = "11111111-2222-4333-8444-555555555555"
	pipelineModelGUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	pipelineToken     = "0123456789abcdef0123456789abcdef"
)

func TestMCPPipelineReachesAuthenticatedBridgeAndMapsDomainErrors(t *testing.T) {
	t.Parallel()

	fake := newPipelineBridge(t)
	registry := sessions.NewRegistry(
		fake.directory,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		time.Second,
	)

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), registry)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "pipeline-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	list, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      SessionsListToolName,
		Arguments: map[string]any{},
	})
	if err != nil || list.IsError {
		t.Fatalf("sessions.list = %#v, %v", list, err)
	}
	var listed sessions.ListOutput
	decodeStructured(t, list.StructuredContent, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].SessionID != pipelineSessionID {
		t.Fatalf("sessions = %#v, want authenticated fake bridge", listed.Sessions)
	}

	summary, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      ModelSummaryToolName,
		Arguments: map[string]any{"session_id": pipelineSessionID},
	})
	if err != nil || summary.IsError {
		t.Fatalf("model.summary = %#v, %v", summary, err)
	}
	var summaryOutput model.SummaryOutput
	decodeStructured(t, summary.StructuredContent, &summaryOutput)
	if summaryOutput.ModelGUID != pipelineModelGUID || summaryOutput.Revision != 7 {
		t.Fatalf("summary = %#v", summaryOutput)
	}

	stale, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: BoxCreateToolName,
		Arguments: map[string]any{
			"session_id":          pipelineSessionID,
			"operation_id":        "pipeline-stale",
			"expected_model_guid": pipelineModelGUID,
			"expected_revision":   float64(6),
			"origin_inches": map[string]any{
				"x": float64(0), "y": float64(0), "z": float64(0),
			},
			"dimensions_inches": map[string]any{
				"width": float64(1), "depth": float64(1), "height": float64(1),
			},
		},
	})
	if err != nil {
		t.Fatalf("geometry.create_box error = %v", err)
	}
	if !stale.IsError {
		t.Fatalf("geometry.create_box IsError = false, want domain error")
	}
	var staleOutput model.CreateBoxOutput
	decodeStructured(t, stale.StructuredContent, &staleOutput)
	if staleOutput.Error == nil || staleOutput.Error.Code != model.ErrorStaleRevision {
		t.Fatalf("stale error = %#v", staleOutput.Error)
	}
	if got := staleOutput.Error.Details["actual_revision"]; got != float64(7) {
		t.Fatalf("actual_revision = %#v, want 7", got)
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("registry.Close() error = %v", err)
	}
	fake.stop()
}

func decodeStructured(t *testing.T, input any, output any) {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}

type pipelineBridge struct {
	directory string
	listener  net.Listener
	wg        sync.WaitGroup
}

func newPipelineBridge(t *testing.T) *pipelineBridge {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	directory := t.TempDir()
	descriptor := bridge.Descriptor{
		Bridge: "sketchup-mcp", ProtocolVersion: bridge.ProtocolVersion,
		SessionID: pipelineSessionID, PID: 4321, Host: "127.0.0.1",
		Port: listener.Addr().(*net.TCPAddr).Port, Token: pipelineToken,
		StartedAt: "2026-09-16T00:00:00Z",
	}
	data, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatalf("json.Marshal(descriptor) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, pipelineSessionID+".json"),
		append(data, '\n'),
		0o600,
	); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}

	fake := &pipelineBridge{directory: directory, listener: listener}
	fake.wg.Add(1)
	go fake.serve()
	return fake
}

func (f *pipelineBridge) serve() {
	defer f.wg.Done()
	conn, err := f.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	var hello map[string]any
	if readPipelineFrame(conn, &hello) != nil {
		return
	}
	if writePipelineFrame(conn, map[string]any{
		"type": "hello", "protocol_version": bridge.ProtocolVersion,
		"session_id":        pipelineSessionID,
		"server_nonce":      "99999999-8888-4777-8666-555555555555",
		"max_message_bytes": bridge.MaxMessageBytes,
	}) != nil {
		return
	}

	var auth map[string]any
	if readPipelineFrame(conn, &auth) != nil || auth["token"] != pipelineToken {
		return
	}
	if writePipelineFrame(conn, map[string]any{
		"type": "authenticate", "protocol_version": bridge.ProtocolVersion, "ok": true,
	}) != nil {
		return
	}

	for {
		var request struct {
			ID        string         `json:"id"`
			Operation string         `json:"operation"`
			Payload   map[string]any `json:"payload"`
		}
		if readPipelineFrame(conn, &request) != nil {
			return
		}

		var response map[string]any
		switch request.Operation {
		case "session.info":
			response = pipelineSuccess(request.ID, map[string]any{
				"session_id":       pipelineSessionID,
				"pid":              4321,
				"sketchup_version": "26.0.429",
				"model": map[string]any{
					"guid": pipelineModelGUID, "title": "Pipeline", "revision": 7,
				},
			})
		case ModelSummaryToolName:
			response = pipelineSuccess(request.ID, map[string]any{
				"session_id": pipelineSessionID,
				"model_guid": pipelineModelGUID,
				"revision":   7,
				"title":      "Pipeline",
			})
		case BoxCreateToolName:
			response = map[string]any{
				"type": "response", "protocol_version": bridge.ProtocolVersion,
				"id": request.ID, "ok": false,
				"error": map[string]any{
					"code":    model.ErrorStaleRevision,
					"message": "model revision changed; refresh context before mutating",
					"details": map[string]any{
						"expected_revision": 6, "actual_revision": 7,
					},
				},
			}
		default:
			response = map[string]any{
				"type": "response", "protocol_version": bridge.ProtocolVersion,
				"id": request.ID, "ok": false,
				"error": map[string]any{
					"code": model.ErrorInvalidRequest, "message": "unsupported test operation",
				},
			}
		}
		if writePipelineFrame(conn, response) != nil {
			return
		}
	}
}

func pipelineSuccess(id string, payload any) map[string]any {
	return map[string]any{
		"type": "response", "protocol_version": bridge.ProtocolVersion,
		"id": id, "ok": true, "payload": payload,
	}
}

func (f *pipelineBridge) stop() {
	_ = f.listener.Close()
	f.wg.Wait()
}

func writePipelineFrame(w io.Writer, value any) error {
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

func readPipelineFrame(r io.Reader, value any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return err
	}
	return json.Unmarshal(payload, value)
}
