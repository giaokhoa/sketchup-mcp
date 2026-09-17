package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addLayoutValidationTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutPanelValidateToolName,
		Description: "Validate that runtime entities with a panel_id stay inside that template panel's paper-space bounds; run for every populated panel before accepted export.",
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
