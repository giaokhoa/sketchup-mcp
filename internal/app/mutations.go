package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addMutationTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityTranslateToolName,
		Description: "Translate an existing SketchUp group or component instance using a durable EntityRef and stale-write protection.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.TranslateInput) (*mcp.CallToolResult, model.TranslateOutput, error) {
		var output model.TranslateOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, EntityTranslateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.TranslateOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityDeleteToolName,
		Description: "Delete one existing SketchUp group or component instance by durable EntityRef with stale-write and replay protection.",
		Annotations: mutationAnnotations(true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.DeleteInput) (*mcp.CallToolResult, model.DeleteOutput, error) {
		var output model.DeleteOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, EntityDeleteToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.DeleteOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityMaterialSetToolName,
		Description: "Assign a bounded solid RGB material to one existing SketchUp group or component instance by durable EntityRef.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.MaterialSetInput) (*mcp.CallToolResult, model.MaterialSetOutput, error) {
		var output model.MaterialSetOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, EntityMaterialSetToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.MaterialSetOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        EntityNameSetToolName,
		Description: "Set a bounded human-readable name on one SketchUp group or component instance for structured Outliner navigation.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.NameSetInput) (*mcp.CallToolResult, model.NameSetOutput, error) {
		var output model.NameSetOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, EntityNameSetToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.NameSetOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        AssemblyCreateToolName,
		Description: "Create one named SketchUp assembly Group from 2 to 100 existing sibling Group or ComponentInstance entities.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.AssemblyCreateInput) (*mcp.CallToolResult, model.AssemblyCreateOutput, error) {
		var output model.AssemblyCreateOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, AssemblyCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.AssemblyCreateOutput{}, err
		}
		if output.Children == nil {
			output.Children = []model.EntityRef{}
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        BoxCreateToolName,
		Description: "Create one grouped rectangular box with an explicit origin and positive dimensions.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.CreateBoxInput) (*mcp.CallToolResult, model.CreateBoxOutput, error) {
		var output model.CreateBoxOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, BoxCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.CreateBoxOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        ModelUndoToolName,
		Description: "Undo the latest SketchUp operation using the verified SketchUp undo API and report the resulting model revision.",
		Annotations: mutationAnnotations(true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.UndoInput) (*mcp.CallToolResult, model.UndoOutput, error) {
		var output model.UndoOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, ModelUndoToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.UndoOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})
}

func mutationAnnotations(destructive bool) *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  true,
		DestructiveHint: &destructive,
		OpenWorldHint:   &no,
	}
}
