package app

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFinalDemoSurfaceIsExactAndDiscoverable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), emptySessionLister{})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "surface-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	initResult := clientSession.InitializeResult()
	if initResult == nil {
		t.Fatal("InitializeResult() = nil")
	}
	if initResult.Instructions != ServerInstructions {
		t.Fatalf("server instructions = %q, want %q", initResult.Instructions, ServerInstructions)
	}
	for _, hint := range []string{"sketchup.sessions.list", "model.summary", "operation_id", "STALE_REVISION"} {
		if !strings.Contains(initResult.Instructions, hint) {
			t.Fatalf("server instructions missing %q: %q", hint, initResult.Instructions)
		}
	}

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	got := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
		if strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q missing typed input/output schema", tool.Name)
		}
		if tool.Annotations == nil {
			t.Fatalf("tool %q missing annotations", tool.Name)
		}
	}
	slices.Sort(got)

	want := []string{
		BoxCreateToolName,
		ModelUndoToolName,
		EntityInspectToolName,
		EntityTranslateToolName,
		ModelSummaryToolName,
		SelectionGetToolName,
		SessionsListToolName,
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("tool surface = %v, want exactly %v", got, want)
	}

	readOnly := map[string]bool{
		SessionsListToolName:  true,
		ModelSummaryToolName:  true,
		SelectionGetToolName:  true,
		EntityInspectToolName: true,
	}
	for _, tool := range result.Tools {
		if tool.Annotations.ReadOnlyHint != readOnly[tool.Name] {
			t.Fatalf("tool %q readOnlyHint = %v, want %v", tool.Name, tool.Annotations.ReadOnlyHint, readOnly[tool.Name])
		}
		if !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q must advertise idempotent behavior", tool.Name)
		}
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
}
