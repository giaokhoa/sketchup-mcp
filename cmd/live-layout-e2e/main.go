package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	McpExe         string
	RbzPath        string
	Pid            int
	SessionId      string
	SourceModel    string
	Template       string
	OutputDir      string
	FixturePath    string
	WorkflowRunId  string
	ArtifactId     string
	ToolTimeout    time.Duration
	OverallTimeout time.Duration
}

type PanelSummary struct {
	PanelId        string `json:"panel_id"`
	PageIndex      int    `json:"page_index"`
	EntityCount    int    `json:"entity_count"`
	Fits           bool   `json:"fits"`
	ViolationCount int    `json:"violation_count"`
}

type StageResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Report struct {
	Fixture                 string            `json:"fixture"`
	Status                  string            `json:"status"`
	StartedAt               time.Time         `json:"started_at"`
	FinishedAt              time.Time         `json:"finished_at"`
	WorkflowRunId           string            `json:"workflow_run_id,omitempty"`
	ArtifactId              string            `json:"artifact_id,omitempty"`
	McpExecutable           string            `json:"mcp_executable"`
	McpSha256               string            `json:"mcp_sha256"`
	RbzPath                 string            `json:"rbz_path,omitempty"`
	RbzSha256               string            `json:"rbz_sha256,omitempty"`
	SketchUpPid             int               `json:"sketchup_pid"`
	SketchUpVersion         string            `json:"sketchup_version"`
	SessionId               string            `json:"session_id"`
	ModelGuid               string            `json:"model_guid"`
	StartRevision           uint64            `json:"start_revision"`
	EndRevision             uint64            `json:"end_revision"`
	OutputPaths             map[string]string `json:"output_paths"`
	PanelValidations        []PanelSummary    `json:"panel_validations"`
	DimensionCount          int               `json:"dimension_count"`
	ConnectedDimensionCount int               `json:"connected_dimension_count"`
	PresentationFirstCreated int               `json:"presentation_first_created"`
	PresentationFirstReused  int               `json:"presentation_first_reused"`
	PresentationSecondCreated int              `json:"presentation_second_created"`
	PresentationSecondReused  int              `json:"presentation_second_reused"`
	PngBytes                int               `json:"png_bytes"`
	Stages                  []StageResult     `json:"stages"`
	Error                   string            `json:"error,omitempty"`
}

func parseFlags() Config {
	var cfg Config
	flag.StringVar(&cfg.McpExe, "mcp-exe", "", "absolute packaged sketchup-mcp-windows-amd64.exe path")
	flag.StringVar(&cfg.RbzPath, "rbz", "", "absolute matching packaged RBZ path for checksum reporting")
	flag.IntVar(&cfg.Pid, "pid", 0, "target SketchUp PID")
	flag.StringVar(&cfg.SessionId, "session", "", "target SketchUp MCP session ID (alternative to -pid)")
	flag.StringVar(&cfg.SourceModel, "source-model", "", "absolute path expected to already be open in SketchUp")
	flag.StringVar(&cfg.Template, "template", "", "absolute LayOut template path")
	flag.StringVar(&cfg.OutputDir, "output-dir", "", "output directory")
	flag.StringVar(&cfg.FixturePath, "fixture", "", "fixture/spec JSON path")
	flag.StringVar(&cfg.WorkflowRunId, "workflow-run-id", "", "CI workflow run ID that produced the package")
	flag.StringVar(&cfg.ArtifactId, "artifact-id", "", "CI artifact ID that produced the package")
	flag.DurationVar(&cfg.ToolTimeout, "tool-timeout", 45*time.Second, "per MCP call timeout")
	flag.DurationVar(&cfg.OverallTimeout, "timeout", 8*time.Minute, "overall harness timeout")
	flag.Parse()
	return cfg
}

func (cfg Config) Validate() error {
	required := map[string]string{
		"mcp-exe":         cfg.McpExe,
		"rbz":             cfg.RbzPath,
		"workflow-run-id": cfg.WorkflowRunId,
		"artifact-id":     cfg.ArtifactId,
		"source-model":    cfg.SourceModel,
		"template":        cfg.Template,
		"output-dir":      cfg.OutputDir,
		"fixture":         cfg.FixturePath,
	}
	for name, value := range required {
		if value == "" {
			return fmt.Errorf("-%s is required", name)
		}
	}
	if cfg.Pid == 0 && cfg.SessionId == "" {
		return fmt.Errorf("exactly one target selector is required: -pid or -session")
	}
	if cfg.Pid != 0 && cfg.SessionId != "" {
		return fmt.Errorf("use only one target selector: -pid or -session")
	}
	for name, path := range map[string]string{
		"mcp-exe": cfg.McpExe, "source-model": cfg.SourceModel, "template": cfg.Template, "fixture": cfg.FixturePath,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("-%s must be absolute", name)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("-%s: %w", name, err)
		}
	}
	if !filepath.IsAbs(cfg.RbzPath) {
		return fmt.Errorf("-rbz must be absolute")
	}
	if _, err := os.Stat(cfg.RbzPath); err != nil {
		return fmt.Errorf("-rbz: %w", err)
	}
	if !filepath.IsAbs(cfg.OutputDir) {
		return fmt.Errorf("-output-dir must be absolute")
	}
	if cfg.ToolTimeout <= 0 || cfg.OverallTimeout <= 0 {
		return fmt.Errorf("timeouts must be positive")
	}
	return nil
}

func runStage(report *Report, name string, fn func() error) error {
	start := time.Now()
	err := fn()
	stage := StageResult{Name: name, Status: "pass", DurationMs: time.Since(start).Milliseconds()}
	if err != nil {
		stage.Status = "fail"
		stage.Error = err.Error()
	}
	report.Stages = append(report.Stages, stage)
	printStage(stage)
	return err
}

func printStage(stage StageResult) {
	data, _ := json.Marshal(map[string]any{
		"type": "stage", "name": stage.Name, "status": stage.Status,
		"duration_ms": stage.DurationMs, "error": stage.Error,
	})
	fmt.Println(string(data))
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func writeReport(report *Report, outputDir string) error {
	reportPath := filepath.Join(outputDir, "live-layout-e2e-report.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func main() {
	cfg := parseFlags()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "CONFIG:", err)
		os.Exit(2)
	}
	fixture, err := loadFixture(cfg.FixturePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FIXTURE:", err)
		os.Exit(2)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "OUTPUT:", err)
		os.Exit(2)
	}

	report := &Report{
		Fixture:          fixture.Name,
		Status:           "running",
		StartedAt:        time.Now().UTC(),
		WorkflowRunId:    cfg.WorkflowRunId,
		ArtifactId:       cfg.ArtifactId,
		McpExecutable:    cfg.McpExe,
		RbzPath:          cfg.RbzPath,
		OutputPaths:      map[string]string{},
		PanelValidations: []PanelSummary{},
		Stages:           []StageResult{},
	}
	report.McpSha256, err = sha256File(cfg.McpExe)
	if err != nil {
		fmt.Fprintln(os.Stderr, "CHECKSUM:", err)
		os.Exit(2)
	}
	if cfg.RbzPath != "" {
		report.RbzSha256, err = sha256File(cfg.RbzPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "CHECKSUM:", err)
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.OverallTimeout)
	defer cancel()
	runner := newRunner(cfg, fixture, report)
	defer runner.Close()

	stages := []struct {
		name string
		fn   func() error
	}{
		{"connect_mcp", func() error { return runner.connect(ctx) }},
		{"discover_session", func() error { return runner.discoverTarget(ctx) }},
		{"model_bounds", func() error { return runner.readBounds(ctx) }},
		{"template_inspect", func() error { return runner.inspectTemplate(ctx) }},
		{"prepare_presentation", func() error { return runner.preparePresentation(ctx) }},
		{"repeat_presentation", func() error { return runner.repeatPresentation(ctx) }},
		{"model_save_copy", func() error { return runner.saveModelCopy(ctx) }},
		{"layout_create", func() error { return runner.createLayout(ctx) }},
		{"viewports", func() error { return runner.addViewports(ctx) }},
		{"dimensions", func() error { return runner.addDimensions(ctx) }},
		{"annotations", func() error { return runner.addAnnotations(ctx) }},
		{"panel_validation", func() error { return runner.validatePanels(ctx) }},
		{"export", func() error { return runner.exportOutputs(ctx) }},
		{"sketchup_responsive", func() error { return runner.checkResponsive() }},
	}

	for _, stage := range stages {
		if err := runStage(report, stage.name, stage.fn); err != nil {
			report.Status = "fail"
			report.Error = err.Error()
			report.EndRevision = runner.revision
			report.FinishedAt = time.Now().UTC()
			_ = writeReport(report, cfg.OutputDir)
			fmt.Fprintln(os.Stderr, "LIVE_LAYOUT_E2E_FAIL:", err)
			os.Exit(1)
		}
	}
	report.Status = "pass"
	report.EndRevision = runner.revision
	report.FinishedAt = time.Now().UTC()
	if err := writeReport(report, cfg.OutputDir); err != nil {
		fmt.Fprintln(os.Stderr, "REPORT:", err)
		os.Exit(1)
	}
	fmt.Printf("LIVE_LAYOUT_E2E_PASS fixture=%s dimensions=%d connected=%d panels=%d\n",
		report.Fixture, report.DimensionCount, report.ConnectedDimensionCount, len(report.PanelValidations))
}
