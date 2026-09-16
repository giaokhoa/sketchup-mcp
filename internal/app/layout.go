package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addLayoutTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutA3SheetCreateToolName,
		Description: "Create an editable A3 landscape LayOut sheet plus PDF and PNG from the current saved SketchUp model using associative model-connected dimensions.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutA3SheetInput) (*mcp.CallToolResult, model.LayoutA3SheetOutput, error) {
		var output model.LayoutA3SheetOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutA3SheetCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutA3SheetOutput{}, err
		}
		if output.Scenes == nil {
			output.Scenes = []string{}
		}
		return &mcp.CallToolResult{}, output, nil
	})
}
