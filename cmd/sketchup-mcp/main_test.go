package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/giaokhoa/sketchup-mcp/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const stdioHelperEnv = "SKETCHUP_MCP_STDIO_HELPER"

func TestStdioProtocolIsNotContaminatedByDiagnostics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := exec.Command(os.Args[0], "-test.run=TestStdioHelperProcess")
	command.Env = append(os.Environ(), stdioHelperEnv+"=1")

	var stderr bytes.Buffer
	command.Stderr = &stderr

	client := mcp.NewClient(
		&mcp.Implementation{Name: "stdio-integration-test", Version: "test"},
		nil,
	)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v; stderr = %q", err, stderr.String())
	}

	listResult, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v; stderr = %q", err, stderr.String())
	}

	found := false
	for _, tool := range listResult.Tools {
		if tool.Name == app.SessionsListToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tool %q not found", app.SessionsListToolName)
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      app.SessionsListToolName,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v; stderr = %q", err, stderr.String())
	}
	if result.IsError {
		t.Fatalf("CallTool() returned tool error: %#v", result.Content)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session.Close() error = %v; stderr = %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "stdio diagnostic probe") {
		t.Fatalf("stderr = %q, want diagnostic probe", stderr.String())
	}
}

func TestStdioHelperProcess(_ *testing.T) {
	if os.Getenv(stdioHelperEnv) != "1" {
		return
	}

	logger := newLogger(os.Stderr)
	logger.Info("stdio diagnostic probe")

	if err := run(context.Background(), logger); err != nil {
		logger.Error("stdio helper stopped", "error", err)
		os.Exit(1)
	}
	os.Exit(0)
}
