package layoutgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/giaokhoa/sketchup-mcp/internal/model"
)

type Runner interface {
	Generate(context.Context, model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error)
}

type ExecRunner struct {
	WorkerPath string
}

func NewSiblingRunner() *ExecRunner {
	exe, err := os.Executable()
	if err != nil {
		return &ExecRunner{WorkerPath: "layout-worker.exe"}
	}
	return &ExecRunner{WorkerPath: filepath.Join(filepath.Dir(exe), "layout-worker.exe")}
}

func (r *ExecRunner) Generate(ctx context.Context, spec model.LayoutDrawingSpec) (model.LayoutA3SheetOutput, error) {
	var output model.LayoutA3SheetOutput
	if r == nil || r.WorkerPath == "" {
		return output, fmt.Errorf("layout worker path is empty")
	}
	if err := os.MkdirAll(spec.OutputDirectory, 0o755); err != nil {
		return output, fmt.Errorf("create output directory: %w", err)
	}
	specPath := filepath.Join(spec.OutputDirectory, spec.BaseName+".drawing-spec.json")
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return output, fmt.Errorf("encode drawing spec: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(specPath, data, 0o644); err != nil {
		return output, fmt.Errorf("write drawing spec: %w", err)
	}

	cmd := exec.CommandContext(ctx, r.WorkerPath, "generate", "--spec", specPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return output, fmt.Errorf("layout worker failed: %w: %s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return output, fmt.Errorf("decode layout worker output: %w: %s", err, stdout.String())
	}
	if output.SpecPath == "" {
		output.SpecPath = specPath
	}
	return output, nil
}
