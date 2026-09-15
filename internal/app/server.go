package app

import (
	"context"
	"log/slog"

	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/giaokhoa/sketchup-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const SessionsListToolName = "sketchup.sessions.list"

type SessionLister interface {
	List(context.Context) (sessions.ListOutput, error)
}

func NewServer(logger *slog.Logger, sessionLister SessionLister) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "sketchup-mcp",
			Version: version.Version,
		},
		&mcp.ServerOptions{
			Logger:       logger,
			Capabilities: &mcp.ServerCapabilities{},
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        SessionsListToolName,
			Description: "List healthy authenticated SketchUp desktop sessions available to this local MCP host.",
		},
		func(
			ctx context.Context,
			_ *mcp.CallToolRequest,
			_ sessions.ListInput,
		) (*mcp.CallToolResult, sessions.ListOutput, error) {
			if err := ctx.Err(); err != nil {
				return nil, sessions.ListOutput{}, err
			}
			output, err := sessionLister.List(ctx)
			if err != nil {
				return nil, sessions.ListOutput{}, err
			}
			return &mcp.CallToolResult{}, output, nil
		},
	)

	return server
}
