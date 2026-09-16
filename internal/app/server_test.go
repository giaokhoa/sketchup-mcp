package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type emptySessionLister struct{}

func (emptySessionLister) List(context.Context) (sessions.ListOutput, error) {
	return sessions.ListOutput{Sessions: []sessions.Session{}}, nil
}

func (emptySessionLister) Call(_ context.Context, _ string, operation string, _ any, output any) error {
	switch target := output.(type) {
	case *model.SummaryOutput:
		*target = model.SummaryOutput{SessionID: "11111111-1111-4111-8111-111111111111", ModelGUID: "model-guid", Revision: 7}
	case *model.SelectionOutput:
		*target = model.SelectionOutput{SessionID: "11111111-1111-4111-8111-111111111111", ModelGUID: "model-guid", Revision: 7, Entities: []model.SelectionEntity{}}
	case *model.InspectOutput:
		*target = model.InspectOutput{CurrentRevision: 7, RevisionMismatch: true}
	case *model.TranslateOutput:
		*target = model.TranslateOutput{
			OperationID: "op-translate", ModelGUID: "model-guid", Revision: 8,
			EntityRef: &model.EntityRef{
				SessionID: "11111111-1111-4111-8111-111111111111",
				ModelGUID: "model-guid", PersistentID: 42, Revision: 8,
			},
		}
	case *model.DeleteOutput:
		*target = model.DeleteOutput{
			OperationID: "op-delete", ModelGUID: "model-guid", Revision: 8,
			DeletedPersistentID: 42,
		}
	case *model.MaterialSetOutput:
		*target = model.MaterialSetOutput{
			OperationID: "op-material", ModelGUID: "model-guid", Revision: 8,
			EntityRef: &model.EntityRef{
				SessionID: "11111111-1111-4111-8111-111111111111",
				ModelGUID: "model-guid", PersistentID: 42, Revision: 8,
			},
			Material: &model.MaterialInfo{
				Name: "test-material",
				Color: model.RGBColor{R: 1, G: 2, B: 3},
			},
		}
	case *model.NameSetOutput:
		*target = model.NameSetOutput{
			OperationID: "op-name", ModelGUID: "model-guid", Revision: 8,
			EntityRef: &model.EntityRef{
				SessionID: "11111111-1111-4111-8111-111111111111",
				ModelGUID: "model-guid", PersistentID: 42, Revision: 8,
			},
			Name: "Cabinet Side Left",
		}
	case *model.CreateBoxOutput:
		*target = model.CreateBoxOutput{
			OperationID: "op-box", ModelGUID: "model-guid", Revision: 8,
			EntityRef: &model.EntityRef{
				SessionID: "11111111-1111-4111-8111-111111111111",
				ModelGUID: "model-guid", PersistentID: 99, Revision: 8,
			},
		}
	case *model.UndoOutput:
		*target = model.UndoOutput{OperationID: "op-undo", ModelGUID: "model-guid", Revision: 8}
	default:
		return errors.New("unexpected output type")
	}
	_ = operation
	return nil
}

func TestServerNegotiatesSupportedProtocolsAndServesTypedTool(t *testing.T) {
	t.Parallel()

	for _, protocolVersion := range mcp.SupportedProtocolVersions() {
		protocolVersion := protocolVersion
		t.Run(protocolVersion, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), emptySessionLister{})
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

type missingSessionService struct{}

func (missingSessionService) List(context.Context) (sessions.ListOutput, error) {
	return sessions.ListOutput{Sessions: []sessions.Session{}}, nil
}

func (missingSessionService) Call(context.Context, string, string, any, any) error {
	return sessions.ErrSessionNotFound
}

func TestLiveModelToolsExposeTypedReadOnlySchemas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), emptySessionLister{})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	listResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		tools[tool.Name] = tool
	}

	for _, name := range []string{ModelSummaryToolName, SelectionGetToolName, EntityInspectToolName} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("tool %q not found", name)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q missing inferred schemas", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q is not annotated read-only", name)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q must be closed-world", name)
		}
	}

	sessionID := "11111111-1111-4111-8111-111111111111"
	calls := []struct {
		name string
		args map[string]any
	}{
		{ModelSummaryToolName, map[string]any{"session_id": sessionID}},
		{SelectionGetToolName, map[string]any{"session_id": sessionID}},
		{EntityInspectToolName, map[string]any{
			"session_id": sessionID, "model_guid": "model-guid",
			"persistent_id": float64(42), "revision": float64(1),
		}},
	}
	for _, call := range calls {
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil {
			t.Fatalf("CallTool(%s) error = %v", call.name, err)
		}
		if result.IsError {
			t.Fatalf("CallTool(%s) returned error: %#v", call.name, result.StructuredContent)
		}
		if result.StructuredContent == nil {
			t.Fatalf("CallTool(%s) has nil structured content", call.name)
		}
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
}

func TestSessionNotFoundIsStructuredDomainError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), missingSessionService{})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      ModelSummaryToolName,
		Arguments: map[string]any{"session_id": "missing"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want structured domain failure")
	}

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output model.SummaryOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if output.Error == nil || output.Error.Code != model.ErrorSessionNotFound {
		t.Fatalf("error = %#v, want SESSION_NOT_FOUND", output.Error)
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
}

func TestMutationToolsExposeTypedIdempotentWriteSchemas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), emptySessionLister{})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer clientSession.Close()

	listResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		tools[tool.Name] = tool
	}

	for _, name := range []string{
		EntityTranslateToolName,
		EntityDeleteToolName,
		EntityMaterialSetToolName,
		EntityNameSetToolName,
		BoxCreateToolName,
		ModelUndoToolName,
	} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("tool %q not found", name)
		}
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Fatalf("tool %q missing inferred schemas", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q must be write + idempotent", name)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %q must be closed-world", name)
		}
	}
	sessionID := "11111111-1111-4111-8111-111111111111"
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: EntityTranslateToolName,
		Arguments: map[string]any{
			"session_id":          sessionID,
			"operation_id":        "op-translate",
			"expected_model_guid": "model-guid",
			"expected_revision":   float64(7),
			"entity_ref": map[string]any{
				"session_id":    sessionID,
				"model_guid":    "model-guid",
				"persistent_id": float64(42),
				"revision":      float64(7),
			},
			"translation_inches": map[string]any{
				"x": float64(1), "y": float64(0), "z": float64(0),
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(entity.translate) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(entity.translate) returned error: %#v", result.StructuredContent)
	}

	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output model.TranslateOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if output.OperationID != "op-translate" || output.EntityRef == nil || output.Revision != 8 {
		t.Fatalf("translate output = %#v", output)
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
}
