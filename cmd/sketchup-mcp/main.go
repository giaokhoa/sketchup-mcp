package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/giaokhoa/sketchup-mcp/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := newLogger(os.Stderr)
	if err := run(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("MCP server stopped", "error", err)
		os.Exit(1)
	}
}

func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, nil))
}

func run(ctx context.Context, logger *slog.Logger) error {
	return app.NewServer(logger).Run(ctx, &mcp.StdioTransport{})
}
