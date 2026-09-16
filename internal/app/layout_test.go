package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type snapshotService struct {
	operation string
}

func (s *snapshotService) List(context.Context) (sessions.ListOutput, error) {
	return sessions.ListOutput{Sessions: []sessions.Session{}}, nil
}

func (s *snapshotService) Call(_ context.Context, _ string, operation string, _ any, output any) error {
	s.operation = operation
	target := output.(*model.LayoutSnapshotOutput)
	*target = model.LayoutSnapshotOutput{
		OperationID: "layout-op",
		ModelGUID:   "model-guid",
		Revision:    8,
		SKPPath:     "C:\\out\\cabinet.skp",
		Spec: model.LayoutDrawingSpec{
			SchemaVersion:   1,
			SnapshotPath:    "C:\\out\\cabinet.skp",
			OutputDirectory: "C:\\out",
			BaseName:        "cabinet",
			PageWidthMM:     420,
			PageHeightMM:    297,
		},
	}
	return nil
}

type fakeLayoutRunner struct {
	called bool
	spec   model.LayoutDrawingSpec
}

func (r *fakeLayoutRunner) Generate(_ context.Context, spec model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error) {
	r.called = true
	r.spec = spec
	return model.LayoutA3SheetOutput{
		LayOutPath:     "C:\\out\\cabinet.layout",
		PDFPath:        "C:\\out\\cabinet.pdf",
		PNGPath:        "C:\\out\\cabinet-preview-1.png",
		QAPath:         "C:\\out\\cabinet.qa.json",
		PageWidthMM:    420,
		PageHeightMM:   297,
		ViewportCount:  6,
		DimensionCount: 30,
		QA: model.LayoutQAReport{
			Passed: true,
			Checks: []model.LayoutQACheck{{Name: "pdf_file", Passed: true}},
		},
	}, nil
}

func TestLayoutToolSnapshotsThenRunsStandaloneWorker(t *testing.T) {
	ctx := context.Background()
	service := &snapshotService{}
	runner := &fakeLayoutRunner{}
	server := NewServerWithLayoutRunner(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		service,
		runner,
	)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "layout-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: LayoutA3SheetCreateToolName,
		Arguments: map[string]any{
			"session_id":          "11111111-1111-4111-8111-111111111111",
			"operation_id":        "layout-op",
			"expected_model_guid": "model-guid",
			"expected_revision":   float64(7),
			"output_directory":    "C:\\out",
			"base_name":           "cabinet",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() tool error = %#v", result.StructuredContent)
	}
	if service.operation != documentationSnapshotOperation {
		t.Fatalf("bridge operation = %q, want %q", service.operation, documentationSnapshotOperation)
	}
	if !runner.called {
		t.Fatal("standalone runner was not called")
	}
	if runner.spec.SnapshotPath != "C:\\out\\cabinet.skp" {
		t.Fatalf("runner snapshot path = %q", runner.spec.SnapshotPath)
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("client close: %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("server wait: %v", err)
	}
}

type failingQARunner struct{}

func (failingQARunner) Generate(context.Context, model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error) {
	return model.LayoutA3SheetOutput{
		QA: model.LayoutQAReport{
			Passed: false,
			Checks: []model.LayoutQACheck{{Name: "png_preview", Passed: false}},
		},
	}, nil
}

func TestLayoutToolReturnsStructuredFailureWhenWorkerQAFails(t *testing.T) {
	ctx := context.Background()
	server := NewServerWithLayoutRunner(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		&snapshotService{},
		failingQARunner{},
	)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "layout-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: LayoutA3SheetCreateToolName,
		Arguments: map[string]any{
			"session_id":          "11111111-1111-4111-8111-111111111111",
			"operation_id":        "layout-op-qa",
			"expected_model_guid": "model-guid",
			"expected_revision":   float64(7),
			"output_directory":    "C:\\out",
			"base_name":           "cabinet",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError {
		t.Fatal("IsError = false, want QA failure")
	}
	clientSession.Close()
	serverSession.Wait()
}
