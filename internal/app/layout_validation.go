package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addLayoutValidationTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutPanelValidateToolName,
		Description: "Validate that all runtime LayOut entities associated with a template panel remain inside that panel's drawing bounds.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutPanelValidateInput) (*mcp.CallToolResult, model.LayoutPanelValidateOutput, error) {
		var output model.LayoutPanelValidateOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		payload := map[string]any{
			"layout_path": input.LayoutPath,
			"page_index":  input.PageIndex,
			"panel_id":    input.PanelID,
			"margin_mm":   input.MarginMM,
		}
		if err := service.Call(ctx, input.SessionID, LayoutPanelValidateToolName, payload, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutPanelValidateOutput{}, err
		}
		if output.Violations == nil {
			output.Violations = []model.LayoutPanelViolation{}
		}
		return &mcp.CallToolResult{}, output, nil
	})
}
