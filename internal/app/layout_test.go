package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type layoutService struct {
	operation   string
	previewPath string
}

func (s *layoutService) List(context.Context) (sessions.ListOutput, error) {
	return sessions.ListOutput{Sessions: []sessions.Session{}}, nil
}

func (s *layoutService) Call(_ context.Context, _ string, operation string, _ any, output any) error {
	s.operation = operation
	target := output.(*model.LayoutA3SheetOutput)
	*target = model.LayoutA3SheetOutput{
		OperationID:             "layout-op",
		ModelGUID:               "model-guid",
		Revision:                8,
		SKPPath:                 "/tmp/out\\cabinet.skp",
		LayOutPath:              "/tmp/out\\cabinet.layout",
		PDFPath:                 "/tmp/out\\cabinet.pdf",
		PNGPath:                 s.previewPath,
		PageWidthMM:             420,
		PageHeightMM:            297,
		ViewportCount:           6,
		DimensionCount:          32,
		ConnectedDimensionCount: 32,
		Scenes:                  []string{"plan", "front", "section_a", "section_b", "side", "iso"},
	}
	return nil
}

func TestLayoutToolUsesSingleSketchUpBridgeOperation(t *testing.T) {
	ctx := context.Background()
	preview, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode preview fixture: %v", err)
	}
	previewPath := filepath.Join(t.TempDir(), "cabinet.png")
	if err := os.WriteFile(previewPath, preview, 0o600); err != nil {
		t.Fatalf("write preview: %v", err)
	}

	service := &layoutService{previewPath: previewPath}
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), service)
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
			"output_directory":    "/tmp/out",
			"base_name":           "cabinet",
		},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() tool error = %#v", result.StructuredContent)
	}
	if service.operation != LayoutA3SheetCreateToolName {
		t.Fatalf("bridge operation = %q, want %q", service.operation, LayoutA3SheetCreateToolName)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want one native preview image", len(result.Content))
	}
	image, ok := result.Content[0].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *mcp.ImageContent", result.Content[0])
	}
	if image.MIMEType != "image/png" {
		t.Fatalf("preview MIME type = %q, want image/png", image.MIMEType)
	}
	if !bytes.Equal(image.Data, preview) {
		t.Fatalf("preview bytes = %q, want %q", image.Data, preview)
	}

	if err := clientSession.Close(); err != nil {
		t.Fatalf("client close: %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("server wait: %v", err)
	}
}
