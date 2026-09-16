package app

import (
	"context"

	"github.com/giaokhoa/sketchup-mcp/internal/layoutgen"
	"github.com/giaokhoa/sketchup-mcp/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const documentationSnapshotOperation = "documentation.snapshot.create"

func addLayoutTools(server *mcp.Server, service SessionService, runner layoutgen.Runner) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        LayoutA3SheetCreateToolName,
		Description: "Prepare a documentation snapshot in SketchUp, then generate an editable A3 LayOut sheet, PDF, PNG, and QA report with the standalone LayOut worker.",
		Annotations: mutationAnnotations(false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input model.LayoutA3SheetInput) (*mcp.CallToolResult, model.LayoutA3SheetOutput, error) {
		var output model.LayoutA3SheetOutput
		if err := input.Validate(); err != nil {
			return toolFailure(&output.Error, invalidRequest(err)), output, nil
		}

		var snapshot model.LayoutSnapshotOutput
		if err := service.Call(
			ctx,
			input.SessionID,
			documentationSnapshotOperation,
			input.SnapshotBridgePayload(),
			&snapshot,
		); err != nil {
			if domain := domainError(err); domain != nil {
				output.Error = domain
				return &mcp.CallToolResult{IsError: true}, output, nil
			}
			return nil, model.LayoutA3SheetOutput{}, err
		}
		if snapshot.Error != nil {
			output.Error = snapshot.Error
			return &mcp.CallToolResult{IsError: true}, output, nil
		}

		if snapshot.Spec.OutputDirectory == "" {
			snapshot.Spec.OutputDirectory = input.OutputDirectory
		}
		if snapshot.Spec.BaseName == "" {
			snapshot.Spec.BaseName = input.BaseName
		}
		if snapshot.Spec.SnapshotPath == "" {
			snapshot.Spec.SnapshotPath = snapshot.SKPPath
		}

		generated, err := runner.Generate(ctx, snapshot.Spec)
		if err != nil {
			return nil, model.LayoutA3SheetOutput{}, err
		}
		generated.OperationID = snapshot.OperationID
		generated.ModelGUID = snapshot.ModelGUID
		generated.Revision = snapshot.Revision
		if generated.SKPPath == "" {
			generated.SKPPath = snapshot.SKPPath
		}
		if generated.Scenes == nil {
			generated.Scenes = []string{}
		}
		if !generated.QA.Passed {
			generated.Error = &model.ToolError{
				Code:    model.ErrorLayoutQAFailed,
				Message: "generated LayOut artifacts did not pass the worker QA gate",
			}
			return &mcp.CallToolResult{IsError: true}, generated, nil
		}
		return &mcp.CallToolResult{}, generated, nil
	})
}
