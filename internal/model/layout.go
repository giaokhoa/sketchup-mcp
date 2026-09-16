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
	OutputDirectory string `json:"output_directory" jsonschema:"existing or creatable absolute output directory on the SketchUp machine"`
	BaseName        string `json:"base_name" jsonschema:"safe output base name using letters digits dot dash underscore"`
	ExportPDF       bool   `json:"export_pdf" jsonschema:"also export a PDF beside the editable LayOut document"`
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

func (i LayoutA3SheetInput) BridgePayload() any {
	return struct {
		Mutation        BridgeMutation `json:"mutation"`
		OutputDirectory string         `json:"output_directory"`
		BaseName        string         `json:"base_name"`
		ExportPDF       bool           `json:"export_pdf"`
	}{
		Mutation:        i.bridgeMutation("SketchUp MCP: Create A3 LayOut Sheet"),
		OutputDirectory: i.OutputDirectory,
		BaseName:        i.BaseName,
		ExportPDF:       i.ExportPDF,
	}
}

type LayoutA3SheetOutput struct {
	OperationID    string     `json:"operation_id,omitempty"`
	ModelGUID      string     `json:"model_guid,omitempty"`
	Revision       uint64     `json:"revision"`
	SKPPath        string     `json:"skp_path,omitempty"`
	LayOutPath     string     `json:"layout_path,omitempty"`
	PDFPath        string     `json:"pdf_path,omitempty"`
	PageWidthMM    float64    `json:"page_width_mm,omitempty"`
	PageHeightMM   float64    `json:"page_height_mm,omitempty"`
	ViewportCount  int        `json:"viewport_count"`
	DimensionCount int        `json:"dimension_count"`
	Scenes         []string   `json:"scenes"`
	Error          *ToolError `json:"error,omitempty"`
}
