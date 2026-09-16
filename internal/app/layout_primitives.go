[Reading 158 lines from start (total: 158 lines, 0 remaining)]

package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addLayoutPrimitiveTools(server *mcp.Server, service SessionService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutDocumentCreateToolName,
		Description: "Create one empty LayOut document with one page. Paper size is provided in millimeters.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutDocumentCreateInput) (*mcp.CallToolResult, model.LayoutFileOutput, error) {
		var output model.LayoutFileOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutDocumentCreateToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutFileOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutViewportAddToolName,
		Description: "Add one SketchUp viewport to an existing LayOut page using millimeter paper bounds and either a scene or standard view.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutViewportAddInput) (*mcp.CallToolResult, model.LayoutEntityOutput, error) {
		var output model.LayoutEntityOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutViewportAddToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutEntityOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutDimensionAddToolName,
		Description: "Add one associative linear dimension connected to model geometry through a LayOut viewport and SketchUp persistent-id paths.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutDimensionAddInput) (*mcp.CallToolResult, model.LayoutEntityOutput, error) {
		var output model.LayoutEntityOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutDimensionAddToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutEntityOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutTextAddToolName,
		Description: "Add one text box to an existing LayOut page using millimeter paper bounds.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutTextAddInput) (*mcp.CallToolResult, model.LayoutEntityOutput, error) {
		var output model.LayoutEntityOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutTextAddToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutEntityOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutLineAddToolName,
		Description: "Add one straight line to an existing LayOut page using millimeter paper coordinates.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutLineAddInput) (*mcp.CallToolResult, model.LayoutEntityOutput, error) {
		var output model.LayoutEntityOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutLineAddToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutEntityOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutRectangleAddToolName,
		Description: "Add one unfilled rectangle to an existing LayOut page using millimeter paper bounds.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutRectangleAddInput) (*mcp.CallToolResult, model.LayoutEntityOutput, error) {
		var output model.LayoutEntityOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutRectangleAddToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutEntityOutput{}, err
		}
		return &mcp.CallToolResult{}, output, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutExportToolName,
		Description: "Export an existing LayOut document to PDF, PNG, or JPEG. Image exports are also returned as native MCP image content.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutExportInput) (*mcp.CallToolResult, model.LayoutExportOutput, error) {
		var output model.LayoutExportOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}
		if err := service.Call(ctx, input.SessionID, LayoutExportToolName, input.BridgePayload(), &output); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutExportOutput{}, err
		}
		if !strings.HasPrefix(output.MIMEType, "image/") {
			return &mcp.CallToolResult{}, output, nil
		}
		if output.OutputPath == "" {
			return nil, model.LayoutExportOutput{}, fmt.Errorf("layout export image path is empty")
		}
		data, err := os.ReadFile(output.OutputPath)
		if err != nil {
			return nil, model.LayoutExportOutput{}, fmt.Errorf("read layout export image: %w", err)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.ImageContent{Data: data, MIMEType: output.MIMEType},
		}}, output, nil
	})
}

[executed on device: TRANKIMVU (8beafd98-533f-4106-8d11-e085770e63d2)]