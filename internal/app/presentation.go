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
		Description: "Create one SketchUp scene from an explicit camera, with an optional active section plane captured into the scene.",
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
