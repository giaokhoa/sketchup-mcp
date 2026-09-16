package model

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var safeLayoutBaseName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type LayoutA3SheetInput struct {
	MutationEnvelope
	OutputDirectory string `json:"output_directory" jsonschema:"existing or creatable absolute output directory on the MCP host"`
	BaseName        string `json:"base_name" jsonschema:"safe output base name using letters digits dot dash underscore"`
}

func (i LayoutA3SheetInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.OutputDirectory) == "" {
		return errors.New("output_directory is required")
	}
	if !filepath.IsAbs(i.OutputDirectory) {
		return errors.New("output_directory must be absolute")
	}
	if strings.TrimSpace(i.BaseName) == "" {
		return errors.New("base_name is required")
	}
	if len(i.BaseName) > 80 || !safeLayoutBaseName.MatchString(i.BaseName) {
		return errors.New("base_name must be at most 80 bytes and contain only letters, digits, dot, dash, underscore")
	}
	return nil
}

func (i LayoutA3SheetInput) SnapshotBridgePayload() any {
	return struct {
		Mutation        BridgeMutation `json:"mutation"`
		OutputDirectory string         `json:"output_directory"`
		BaseName        string         `json:"base_name"`
	}{
		Mutation:        i.bridgeMutation("SketchUp MCP: Prepare Documentation Snapshot"),
		OutputDirectory: i.OutputDirectory,
		BaseName:        i.BaseName,
	}
}

type LayoutPoint3D struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type LayoutBounds3D struct {
	Min LayoutPoint3D `json:"min"`
	Max LayoutPoint3D `json:"max"`
}

type LayoutPaperRect struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type LayoutViewSpec struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	SceneIndex  int             `json:"scene_index"`
	Rect        LayoutPaperRect `json:"rect"`
	Scale       float64         `json:"scale,omitempty"`
	Perspective bool            `json:"perspective"`
}

type LayoutDimensionEndpoint struct {
	Point            LayoutPoint3D `json:"point"`
	PersistentIDPath string        `json:"persistent_id_path,omitempty"`
}

type LayoutDimensionSpec struct {
	ID     string                  `json:"id"`
	ViewID string                  `json:"view_id"`
	Start  LayoutDimensionEndpoint `json:"start"`
	End    LayoutDimensionEndpoint `json:"end"`
	Offset LayoutPoint3D           `json:"offset"`
}

type LayoutDrawingSpec struct {
	SchemaVersion   int                   `json:"schema_version"`
	SnapshotPath    string                `json:"snapshot_path"`
	OutputDirectory string                `json:"output_directory"`
	BaseName        string                `json:"base_name"`
	PageWidthMM     float64               `json:"page_width_mm"`
	PageHeightMM    float64               `json:"page_height_mm"`
	RootBounds      LayoutBounds3D        `json:"root_bounds"`
	Views           []LayoutViewSpec      `json:"views"`
	Dimensions      []LayoutDimensionSpec `json:"dimensions"`
	Notes           []string              `json:"notes"`
}

type LayoutSnapshotOutput struct {
	OperationID string            `json:"operation_id,omitempty"`
	ModelGUID   string            `json:"model_guid,omitempty"`
	Revision    uint64            `json:"revision"`
	SKPPath     string            `json:"skp_path,omitempty"`
	Spec        LayoutDrawingSpec `json:"spec"`
	Error       *ToolError        `json:"error,omitempty"`
}

type LayoutQACheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

type LayoutQAReport struct {
	Passed bool            `json:"passed"`
	Checks []LayoutQACheck `json:"checks"`
}

type LayoutA3SheetOutput struct {
	OperationID    string         `json:"operation_id,omitempty"`
	ModelGUID      string         `json:"model_guid,omitempty"`
	Revision       uint64         `json:"revision"`
	SKPPath        string         `json:"skp_path,omitempty"`
	SpecPath       string         `json:"spec_path,omitempty"`
	LayOutPath     string         `json:"layout_path,omitempty"`
	PDFPath        string         `json:"pdf_path,omitempty"`
	PNGPath        string         `json:"png_path,omitempty"`
	QAPath         string         `json:"qa_path,omitempty"`
	PageWidthMM    float64        `json:"page_width_mm,omitempty"`
	PageHeightMM   float64        `json:"page_height_mm,omitempty"`
	ViewportCount  int            `json:"viewport_count"`
	DimensionCount int            `json:"dimension_count"`
	Scenes         []string       `json:"scenes"`
	QA             LayoutQAReport `json:"qa"`
	Error          *ToolError     `json:"error,omitempty"`
}
