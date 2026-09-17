package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var expectedPublicToolNames = []string{
	SessionsListToolName,
	ModelSummaryToolName,
	ModelBoundsToolName,
	SelectionGetToolName,
	EntityInspectToolName,
	EntityChildrenListToolName,
	EntityTranslateToolName,
	EntityDeleteToolName,
	EntityMaterialSetToolName,
	EntityNameSetToolName,
	AssemblyCreateToolName,
	BoxCreateToolName,
	SectionPlaneCreateToolName,
	SceneCreateToolName,
	ModelSaveCopyToolName,
	ModelUndoToolName,
	LayoutDocumentCreateToolName,
	LayoutTemplateInspectToolName,
	LayoutPanelValidateToolName,
	LayoutViewportAddToolName,
	LayoutDimensionAddToolName,
	LayoutTextAddToolName,
	LayoutLineAddToolName,
	LayoutRectangleAddToolName,
	LayoutExportToolName,
}

func sortedExpectedPublicToolNames() []string {
	want := slices.Clone(expectedPublicToolNames)
	slices.Sort(want)
	return want
}

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

	want := sortedExpectedPublicToolNames()
	if !slices.Equal(got, want) {
		t.Fatalf("tool surface = %v, want exactly %v", got, want)
	}

	readOnly := map[string]bool{
		SessionsListToolName:           true,
		ModelSummaryToolName:           true,
		ModelBoundsToolName:            true,
		LayoutTemplateInspectToolName:  true,
		LayoutPanelValidateToolName:    true,
		SelectionGetToolName:           true,
		EntityInspectToolName:          true,
		EntityChildrenListToolName:     true,
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

func TestPublicToolGuidanceExposesLayoutInvariants(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)), emptySessionLister{})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "guidance-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	tools := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		tools[tool.Name] = tool
	}

	descriptionHints := map[string][]string{
		SceneCreateToolName:          {"reusable presentation state", "named scenes"},
		LayoutTemplateInspectToolName: {"template-first", "viewport slots"},
		LayoutViewportAddToolName:     {"scene_name", "scale_denominator", "require_fit=true"},
		LayoutDimensionAddToolName:    {"associative", "connected=true"},
		LayoutPanelValidateToolName:   {"panel_id", "before accepted export"},
		LayoutExportToolName:          {"layout.panel.validate", "before accepted export"},
	}
	for name, hints := range descriptionHints {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("missing tool %q", name)
		}
		for _, hint := range hints {
			if !strings.Contains(tool.Description, hint) {
				t.Errorf("tool %q description missing %q: %q", name, hint, tool.Description)
			}
		}
	}

	assertSchemaHints(t, tools[LayoutViewportAddToolName].InputSchema,
		"prefer a named documentation scene", "orthographic drawing scale denominator",
		"accepted drawings normally true", "model-space bounds in millimeters, not paper-space bounds",
		"layout.panel.validate containment checks")
	assertSchemaHints(t, tools[LayoutDimensionAddToolName].OutputSchema,
		"associative dimension", "connected")
	assertSchemaHints(t, tools[LayoutPanelValidateToolName].InputSchema,
		"template panel ID", "containment")

	if err := clientSession.Close(); err != nil {
		t.Fatalf("clientSession.Close() error = %v", err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Fatalf("serverSession.Wait() error = %v", err)
	}
}

func assertSchemaHints(t *testing.T, schema any, hints ...string) {
	t.Helper()
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	text := string(data)
	for _, hint := range hints {
		if !strings.Contains(text, hint) {
			t.Errorf("schema missing %q: %s", hint, text)
		}
	}
}

func TestDemoDocumentsExactPublicToolSurface(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../docs/demo.md")
	if err != nil {
		t.Fatalf("read docs/demo.md: %v", err)
	}
	text := string(data)
	const startMarker = "<!-- PUBLIC_TOOL_LIST_START -->"
	const endMarker = "<!-- PUBLIC_TOOL_LIST_END -->"
	start := strings.Index(text, startMarker)
	end := strings.Index(text, endMarker)
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("docs/demo.md must contain one ordered public-tool marker block")
	}

	block := text[start+len(startMarker) : end]
	got := make([]string, 0, len(expectedPublicToolNames))
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		got = append(got, line)
	}
	slices.Sort(got)
	want := sortedExpectedPublicToolNames()
	if !slices.Equal(got, want) {
		t.Fatalf("docs public tool list = %v, want exactly %v", got, want)
	}
	if !strings.Contains(text, "| Local MCP tools | 25 |") {
		t.Fatal("docs/demo.md must state the current 25-tool surface")
	}
	for _, stale := range []string{"| Local MCP tools | 23 |", "exactly 23 MCP tools discovered"} {
		if strings.Contains(text, stale) {
			t.Fatalf("docs/demo.md still contains stale tool-surface text %q", stale)
		}
	}
}
