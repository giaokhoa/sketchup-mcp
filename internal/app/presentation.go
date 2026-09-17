package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addPresentationTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ModelBoundsToolName,
		Description: "Read root SketchUp model bounds in millimeters for camera, section, and LayOut calculations.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.ModelBoundsInput) (*mcp.CallToolResult, model.ModelBoundsOutput, error) {
		var output model.ModelBoundsOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, ModelBoundsToolName, map[string]any{}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.ModelBoundsOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        SectionPlaneListToolName,
		Description: "List bounded root-level SketchUp section planes with durable refs, model-space planes, and active state so clients can reuse presentation state.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.PresentationListInput) (*mcp.CallToolResult, model.SectionPlaneListOutput, error) {
		var output model.SectionPlaneListOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, SectionPlaneListToolName, map[string]any{}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SectionPlaneListOutput{}, err
		}
		if output.SectionPlanes == nil {
			output.SectionPlanes = []model.SectionPlaneInfo{}
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        SceneListToolName,
		Description: "List bounded SketchUp scenes with camera and captured section state so clients can reuse named presentation state before creating duplicates.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.PresentationListInput) (*mcp.CallToolResult, model.SceneListOutput, error) {
		var output model.SceneListOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, SceneListToolName, map[string]any{}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SceneListOutput{}, err
		}
		if output.Scenes == nil {
			output.Scenes = []model.SceneInfo{}
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        SectionPlaneCreateToolName,
		Description: "Create one root-level SketchUp section plane from a millimeter point and a normal vector.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.SectionPlaneCreateInput) (*mcp.CallToolResult, model.SectionPlaneCreateOutput, error) {
		var output model.SectionPlaneCreateOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, SectionPlaneCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SectionPlaneCreateOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        SceneCreateToolName,
		Description: "Create a named SketchUp scene as reusable presentation state for camera and optional active section plane; LayOut viewports should reference named scenes when available.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.SceneCreateInput) (*mcp.CallToolResult, model.SceneCreateOutput, error) {
		var output model.SceneCreateOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, SceneCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.SceneCreateOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        ModelSaveCopyToolName,
		Description: "Save the current SketchUp model state to a separate .skp copy without changing the model's associated file path.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.ModelSaveCopyInput) (*mcp.CallToolResult, model.ModelSaveCopyOutput, error) {
		var output model.ModelSaveCopyOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, ModelSaveCopyToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.ModelSaveCopyOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})
}
