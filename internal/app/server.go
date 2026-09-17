package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/giaokhoa/sketchup-mcp/internal/bridge"
	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/giaokhoa/sketchup-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	SessionsListToolName    = "sketchup.sessions.list"
	ModelSummaryToolName    = "model.summary"
	ModelBoundsToolName     = "model.bounds"
	SelectionGetToolName    = "selection.get"
	EntityInspectToolName      = "entity.inspect"
	EntityChildrenListToolName = "entity.children.list"
	EntityTranslateToolName    = "entity.translate"
	EntityDeleteToolName      = "entity.delete"
	EntityMaterialSetToolName = "entity.material.set"
	EntityNameSetToolName     = "entity.name.set"
	AssemblyCreateToolName     = "assembly.create"
	BoxCreateToolName          = "geometry.create_box"
	SectionPlaneCreateToolName = "section_plane.create"
	SceneCreateToolName        = "scene.create"
	ModelSaveCopyToolName      = "model.file.save_copy"
	ModelUndoToolName          = "changes.undo"
	LayoutDocumentCreateToolName = "layout.document.create"
	LayoutTemplateInspectToolName = "layout.template.inspect"
	LayoutPanelValidateToolName = "layout.panel.validate"
	LayoutViewportAddToolName = "layout.viewport.add"
	LayoutDimensionAddToolName = "layout.dimension.add"
	LayoutTextAddToolName = "layout.text.add"
	LayoutLineAddToolName = "layout.line.add"
	LayoutRectangleAddToolName = "layout.rectangle.add"
	LayoutExportToolName = "layout.export"

	ServerInstructions = "Start with sketchup.sessions.list and choose one live session. Read model.summary before any write and use the returned model GUID and revision. For LayOut documentation: read model.bounds, prepare SketchUp scenes/section state, save a .skp copy, inspect the .layout template, create from that template, add viewports/dimensions/annotations with panel_id, validate every populated panel, then export. Prefer named scenes for viewport presentation; orthographic views need an explicit scale denominator; accepted drawings should normally use require_fit=true with model-space fit_model_bounds_mm. Paper-space coordinates are millimeters. Reuse durable entity references, give every intended write a unique operation_id, and on STALE_REVISION refresh state before retrying with a new operation_id."
)

type SessionService interface {
	List(context.Context) (sessions.ListOutput, error)
	Call(context.Context, string, string, any, any) error
}

func NewServer(logger *slog.Logger, service SessionService) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "sketchup-mcp", Version: version.Version},
		&mcp.ServerOptions{
			Logger:       logger,
			Capabilities: &mcp.ServerCapabilities{},
			Instructions: ServerInstructions,
		},
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:        SessionsListToolName,
		Description: "List healthy authenticated SketchUp desktop sessions available to this local MCP host.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ sessions.ListInput) (*mcp.CallToolResult, sessions.ListOutput, error) {
		if err := ctx.Err(); err != nil {
			return nil, sessions.ListOutput{}, err
		}
		output, err := service.List(ctx)
		if err != nil {
			return nil, sessions.ListOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        ModelSummaryToolName,
		Description: "Read a bounded summary of one live SketchUp model without traversing its geometry tree.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.SummaryInput) (*mcp.CallToolResult, model.SummaryOutput, error) {
		var output model.SummaryOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, "model.summary", map[string]any{}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SummaryOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        SelectionGetToolName,
		Description: "Read the current SketchUp selection with bounded entity summaries and durable references for supported entities.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.SelectionInput) (*mcp.CallToolResult, model.SelectionOutput, error) {
		var output model.SelectionOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, "selection.get", map[string]any{}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SelectionOutput{}, err
		}
		if output.Entities == nil {
			output.Entities = []model.SelectionEntity{}
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityInspectToolName,
		Description: "Inspect a supported SketchUp entity by persistent EntityRef and report revision mismatch explicitly.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.InspectInput) (*mcp.CallToolResult, model.InspectOutput, error) {
		var output model.InspectOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		ref := input.Ref()
		if err := service.Call(ctx, ref.SessionID, "entity.inspect", ref, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.InspectOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityChildrenListToolName,
		Description: "List up to 100 immediate supported Group or ComponentInstance children of one assembly entity.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.ChildrenInput) (*mcp.CallToolResult, model.ChildrenOutput, error) {
		var output model.ChildrenOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		ref := input.Ref()
		if err := service.Call(ctx, ref.SessionID, EntityChildrenListToolName, ref, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.ChildrenOutput{}, err
		}
		if output.Children == nil {
			output.Children = []model.ChildEntity{}
		}
		return &mcp.CallToolResult{}, output, nil
	})

	addMutationTools(server, service)
	addPresentationTools(server, service)
	addLayoutPrimitiveTools(server, service)
	addLayoutTemplateTools(server, service)
	addLayoutValidationTools(server, service)
	return server
}

func readOnlyAnnotations() *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		DestructiveHint: &no,
		OpenWorldHint:   &no,
	}
}

func invalidRequest(err error) *model.ToolError {
	return &model.ToolError{Code: model.ErrorInvalidRequest, Message: err.Error()}
}

func toolFailure(target **model.ToolError, value *model.ToolError) *mcp.CallToolResult {
	*target = value
	return &mcp.CallToolResult{IsError: true}
}

func domainError(err error) *model.ToolError {
	if errors.Is(err, sessions.ErrSessionNotFound) {
		return &model.ToolError{Code: model.ErrorSessionNotFound, Message: "SketchUp session not found"}
	}
	var response *bridge.ResponseError
	if !errors.As(err, &response) {
		return nil
	}
	result := &model.ToolError{Code: response.Code, Message: response.Message}
	if len(response.Details) > 0 {
		var details map[string]any
		if json.Unmarshal(response.Details, &details) == nil {
			result.Details = details
		}
	}
	return result
}
