package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerNegotiatesSupportedProtocolsAndServesTypedTool(t *testing.T) {
	t.Parallel()

	for _, protocolVersion := range mcp.SupportedProtocolVersions() {
		protocolVersion := protocolVersion
		t.Run(protocolVersion, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
			serverTransport, clientTransport := mcp.NewInMemoryTransports()

			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatalf("server.Connect() error = %v", err)
			}

			client := mcp.NewClient(
				&mcp.Implementation{Name: "sketchup-mcp-test", Version: "test"},
				nil,
			)
			clientSession, err := client.Connect(
				ctx,
				clientTransport,
				&mcp.ClientSessionOptions{ProtocolVersion: protocolVersion},
			)
			if err != nil {
				t.Fatalf("client.Connect() error = %v", err)
			}

			initResult := clientSession.InitializeResult()
			if initResult == nil {
				t.Fatal("InitializeResult() = nil")
			}
			if got := initResult.ProtocolVersion; got != protocolVersion {
				t.Fatalf("negotiated protocol version = %q, want %q", got, protocolVersion)
			}

			listResult, err := clientSession.ListTools(ctx, nil)
			if err != nil {
				t.Fatalf("ListTools() error = %v", err)
			}

			var listTool *mcp.Tool
			for _, tool := range listResult.Tools {
				if tool.Name == SessionsListToolName {
					listTool = tool
					break
				}
			}
			if listTool == nil {
				t.Fatalf("tool %q not found in %#v", SessionsListToolName, listResult.Tools)
			}
			if listTool.InputSchema == nil {
				t.Fatal("input schema = nil, want SDK-generated schema")
			}
			if listTool.OutputSchema == nil {
				t.Fatal("output schema = nil, want SDK-generated schema")
			}

			callResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      SessionsListToolName,
				Arguments: map[string]any{},
			})
			if err != nil {
				t.Fatalf("CallTool() error = %v", err)
			}
			if callResult.IsError {
				t.Fatalf("CallTool() returned tool error: %#v", callResult.Content)
			}
			if callResult.StructuredContent == nil {
				t.Fatal("StructuredContent = nil")
			}

			data, err := json.Marshal(callResult.StructuredContent)
			if err != nil {
				t.Fatalf("marshal StructuredContent: %v", err)
			}
			var output sessions.ListOutput
			if err := json.Unmarshal(data, &output); err != nil {
				t.Fatalf("decode StructuredContent: %v", err)
			}
			if output.Sessions == nil {
				t.Fatal("sessions = nil, want an empty typed list")
			}
			if len(output.Sessions) != 0 {
				t.Fatalf("sessions = %#v, want empty list", output.Sessions)
			}

			if err := clientSession.Close(); err != nil {
				t.Fatalf("clientSession.Close() error = %v", err)
			}
			if err := serverSession.Wait(); err != nil {
				t.Fatalf("serverSession.Wait() error = %v", err)
			}
		})
	}
}
