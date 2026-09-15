package app

import (
	"context"
	"log/slog"

	"github.com/giaokhoa/sketchup-mcp/internal/sessions"
	"github.com/giaokhoa/sketchup-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const SessionsListToolName = "sketchup.sessions.list"

// NewServer constructs the public MCP boundary. SketchUp transport belongs in
// later issues and is deliberately absent here.
func NewServer(logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "sketchup-mcp",
			Version: version.Version,
		},
		&mcp.ServerOptions{
			Logger: logger,
			// Avoid the SDK's historical default logging capability. This
			// host exposes tools only at this stage.
			Capabilities: &mcp.ServerCapabilities{},
		},
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        SessionsListToolName,
			Description: "List SketchUp desktop sessions available to this local MCP host.",
		},
		listSessions,
	)

	return server
}

func listSessions(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ sessions.ListInput,
) (*mcp.CallToolResult, sessions.ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, sessions.ListOutput{}, err
	}
	return &mcp.CallToolResult{}, sessions.List(), nil
}
