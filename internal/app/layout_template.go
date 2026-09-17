package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addLayoutTemplateTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutTemplateInspectToolName,
		Description: "Inspect a machine-readable LayOut template: page sizes, layers, tagged viewport slots, tagged style samples, and Auto-Text capabilities.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutTemplateInspectInput) (*mcp.CallToolResult, model.LayoutTemplateInspectOutput, error) {
		var output model.LayoutTemplateInspectOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutTemplateInspectToolName, map[string]any{"template_path": input.TemplatePath}, &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutTemplateInspectOutput{}, err
		}
		if output.Pages == nil { output.Pages = []model.LayoutTemplatePage{} }
		if output.Layers == nil { output.Layers = []model.LayoutTemplateLayer{} }
		if output.Slots == nil { output.Slots = []model.LayoutTemplateSlot{} }
		if output.Panels == nil { output.Panels = []model.LayoutTemplatePanel{} }
		if output.Styles == nil { output.Styles = []model.LayoutTemplateStyle{} }
		if output.AutoTextTypes == nil { output.AutoTextTypes = []string{} }
		return &mcp.CallToolResult{}, output, nil
	})
}
