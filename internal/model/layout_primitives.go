package model

import (
	"errors"
	"path/filepath"
	"strings"
)

type LayoutRectMM struct {
	X      float64 `json:"x" jsonschema:"left edge in millimeters"`
	Y      float64 `json:"y" jsonschema:"top edge in millimeters"`
	Width  float64 `json:"width" jsonschema:"width in millimeters"`
	Height float64 `json:"height" jsonschema:"height in millimeters"`
}

func (r LayoutRectMM) Validate() error {
	if !finite(r.X) || !finite(r.Y) || !finite(r.Width) || !finite(r.Height) {
		return errors.New("bounds_mm values must be finite")
	}
	if r.Width <= 0 || r.Height <= 0 {
		return errors.New("bounds_mm width and height must be greater than zero")
	}
	return nil
}

type LayoutPoint2MM struct {
	X float64 `json:"x" jsonschema:"x coordinate in millimeters"`
	Y float64 `json:"y" jsonschema:"y coordinate in millimeters"`
}

func (p LayoutPoint2MM) Validate() error {
	if !finite(p.X) || !finite(p.Y) {
		return errors.New("paper point values must be finite")
	}
	return nil
}

type LayoutPoint3MM struct {
	X float64 `json:"x" jsonschema:"model x coordinate in millimeters"`
	Y float64 `json:"y" jsonschema:"model y coordinate in millimeters"`
	Z float64 `json:"z" jsonschema:"model z coordinate in millimeters"`
}

func (p LayoutPoint3MM) Validate() error {
	if !finite(p.X) || !finite(p.Y) || !finite(p.Z) {
		return errors.New("model point values must be finite")
	}
	return nil
}

type LayoutEntityRef struct {
	LayoutPath string `json:"layout_path"`
	PageIndex  int    `json:"page_index"`
	EntityID   string `json:"entity_id"`
}

func (r LayoutEntityRef) Validate() error {
	if strings.TrimSpace(r.LayoutPath) == "" || !filepath.IsAbs(r.LayoutPath) {
		return errors.New("layout_path must be absolute")
	}
	if r.PageIndex < 0 {
		return errors.New("page_index must be non-negative")
	}
	if strings.TrimSpace(r.EntityID) == "" {
		return errors.New("entity_id is required")
	}
	return nil
}

func validateLayoutPath(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return errors.New("layout_path must be absolute")
	}
	if strings.ToLower(filepath.Ext(path)) != ".layout" {
		return errors.New("layout_path must end in .layout")
	}
	return nil
}

type LayoutDocumentCreateInput struct {
	MutationEnvelope
	LayoutPath   string  `json:"layout_path" jsonschema:"absolute .layout output path on the SketchUp machine"`
	TemplatePath string  `json:"template_path,omitempty" jsonschema:"optional absolute .layout template path; when set, page size comes from the template"`
	PageWidthMM  float64 `json:"page_width_mm,omitempty" jsonschema:"page width in millimeters for blank documents; use 0 with template_path"`
	PageHeightMM float64 `json:"page_height_mm,omitempty" jsonschema:"page height in millimeters for blank documents; use 0 with template_path"`
}

func (i LayoutDocumentCreateInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }

	if strings.TrimSpace(i.TemplatePath) != "" {
		if !filepath.IsAbs(i.TemplatePath) || strings.ToLower(filepath.Ext(i.TemplatePath)) != ".layout" {
			return errors.New("template_path must be an absolute .layout path")
		}
		if i.PageWidthMM != 0 || i.PageHeightMM != 0 {
			return errors.New("page dimensions must be 0 when template_path is provided")
		}
		return nil
	}

	if !finite(i.PageWidthMM) || !finite(i.PageHeightMM) || i.PageWidthMM <= 0 || i.PageHeightMM <= 0 {
		return errors.New("page dimensions must be positive finite millimeter values for blank documents")
	}
	return nil
}

func (i LayoutDocumentCreateInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		TemplatePath string `json:"template_path"`
		PageWidthMM float64 `json:"page_width_mm"`
		PageHeightMM float64 `json:"page_height_mm"`
	}{i.bridgeMutation("SketchUp MCP: Create LayOut Document"), i.LayoutPath, i.TemplatePath, i.PageWidthMM, i.PageHeightMM}
}

type LayoutViewportAddInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	SKPPath string `json:"skp_path" jsonschema:"absolute saved SketchUp model path"`
	PageIndex int `json:"page_index"`
	LayerName string `json:"layer_name,omitempty" jsonschema:"optional existing LayOut layer name"`
	BoundsMM LayoutRectMM `json:"bounds_mm"`
	SceneName string `json:"scene_name,omitempty" jsonschema:"SketchUp scene name; prefer a named documentation scene when one exists; use exactly one of scene_name or standard_view"`
	StandardView string `json:"standard_view,omitempty" jsonschema:"top front back left right bottom iso; use either standard_view or scene_name"`
	Perspective bool `json:"perspective"`
	ScaleDenominator float64 `json:"scale_denominator" jsonschema:"orthographic drawing scale denominator such as 10 for 1:10; use 0 for perspective"`
	RenderMode string `json:"render_mode" jsonschema:"raster hybrid or vector"`
	PanelID string `json:"panel_id,omitempty" jsonschema:"optional template panel ID; associates this runtime entity for layout.panel.validate containment checks"`
	RequireFit bool `json:"require_fit,omitempty" jsonschema:"for accepted drawings normally true; fail before save when projected model bounds do not fit viewport"`
	FitModelBoundsMM *LayoutModelBoundsMM `json:"fit_model_bounds_mm,omitempty" jsonschema:"required with require_fit; SketchUp model-space bounds in millimeters, not paper-space bounds"`
	FitMarginMM float64 `json:"fit_margin_mm,omitempty" jsonschema:"inward paper-space safety margin in millimeters"`
}

func (i LayoutViewportAddInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if strings.TrimSpace(i.SKPPath) == "" || !filepath.IsAbs(i.SKPPath) { return errors.New("skp_path must be absolute") }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if err := i.BoundsMM.Validate(); err != nil { return err }
	hasScene := strings.TrimSpace(i.SceneName) != ""
	hasView := strings.TrimSpace(i.StandardView) != ""
	if hasScene == hasView { return errors.New("provide exactly one of scene_name or standard_view") }
	if hasView {
		switch strings.ToLower(i.StandardView) {
		case "top", "bottom", "front", "back", "left", "right", "iso":
		default: return errors.New("standard_view must be top, bottom, front, back, left, right, or iso")
		}
	}
	if i.Perspective {
		if i.ScaleDenominator != 0 { return errors.New("scale_denominator must be 0 for perspective viewports") }
	} else if !finite(i.ScaleDenominator) || i.ScaleDenominator <= 0 {
		return errors.New("scale_denominator must be greater than zero for orthographic viewports")
	}
	switch strings.ToLower(i.RenderMode) {
	case "raster", "hybrid", "vector":
	default: return errors.New("render_mode must be raster, hybrid, or vector")
	}
	if !finite(i.FitMarginMM) || i.FitMarginMM < 0 { return errors.New("fit_margin_mm must be finite and non-negative") }
	if i.RequireFit {
		if i.FitModelBoundsMM == nil { return errors.New("fit_model_bounds_mm is required when require_fit is true") }
		if err := i.FitModelBoundsMM.Validate(); err != nil { return err }
	}
	return nil
}

func (i LayoutViewportAddInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		SKPPath string `json:"skp_path"`
		PageIndex int `json:"page_index"`
		LayerName string `json:"layer_name"`
		BoundsMM LayoutRectMM `json:"bounds_mm"`
		SceneName string `json:"scene_name"`
		StandardView string `json:"standard_view"`
		Perspective bool `json:"perspective"`
		ScaleDenominator float64 `json:"scale_denominator"`
		RenderMode string `json:"render_mode"`
		PanelID string `json:"panel_id"`
		RequireFit bool `json:"require_fit"`
		FitModelBoundsMM *LayoutModelBoundsMM `json:"fit_model_bounds_mm"`
		FitMarginMM float64 `json:"fit_margin_mm"`
	}{i.bridgeMutation("SketchUp MCP: Add LayOut Viewport"), i.LayoutPath, i.SKPPath, i.PageIndex, i.LayerName, i.BoundsMM, i.SceneName, i.StandardView, i.Perspective, i.ScaleDenominator, i.RenderMode, i.PanelID, i.RequireFit, i.FitModelBoundsMM, i.FitMarginMM}
}

type LayoutDimensionAddInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	PageIndex int `json:"page_index"`
	LayerName string `json:"layer_name,omitempty" jsonschema:"optional existing LayOut layer name"`
	ViewportRef LayoutEntityRef `json:"viewport_ref"`
	StartPointMM LayoutPoint3MM `json:"start_point_mm"`
	EndPointMM LayoutPoint3MM `json:"end_point_mm"`
	StartPIDPath string `json:"start_pid_path,omitempty" jsonschema:"optional SketchUp persistent ID for a deep connection"`
	EndPIDPath string `json:"end_pid_path,omitempty" jsonschema:"optional SketchUp persistent ID for a deep connection"`
	OffsetMM float64 `json:"offset_mm" jsonschema:"signed paper-space distance from measured points to the dimension line in millimeters"`
	Alignment string `json:"alignment" jsonschema:"auto horizontal vertical or aligned"`
	StyleID string `json:"style_id,omitempty" jsonschema:"optional tagged template style sample id"`
	PanelID string `json:"panel_id,omitempty" jsonschema:"optional template panel ID; associates this runtime entity for layout.panel.validate containment checks"`
}

func (i LayoutDimensionAddInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if err := i.ViewportRef.Validate(); err != nil { return err }
	if i.ViewportRef.LayoutPath != i.LayoutPath || i.ViewportRef.PageIndex != i.PageIndex { return errors.New("viewport_ref must target the same layout_path and page_index") }
	if err := i.StartPointMM.Validate(); err != nil { return err }
	if err := i.EndPointMM.Validate(); err != nil { return err }
	if !finite(i.OffsetMM) || i.OffsetMM == 0 { return errors.New("offset_mm must be finite and non-zero") }
	switch strings.ToLower(i.Alignment) {
	case "auto", "horizontal", "vertical", "aligned":
	default: return errors.New("alignment must be auto, horizontal, vertical, or aligned")
	}
	return nil
}

func (i LayoutDimensionAddInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		PageIndex int `json:"page_index"`
		LayerName string `json:"layer_name"`
		ViewportRef LayoutEntityRef `json:"viewport_ref"`
		StartPointMM LayoutPoint3MM `json:"start_point_mm"`
		EndPointMM LayoutPoint3MM `json:"end_point_mm"`
		StartPIDPath string `json:"start_pid_path"`
		EndPIDPath string `json:"end_pid_path"`
		OffsetMM float64 `json:"offset_mm"`
		Alignment string `json:"alignment"`
		StyleID string `json:"style_id"`
		PanelID string `json:"panel_id"`
	}{i.bridgeMutation("SketchUp MCP: Add LayOut Dimension"), i.LayoutPath, i.PageIndex, i.LayerName, i.ViewportRef, i.StartPointMM, i.EndPointMM, i.StartPIDPath, i.EndPIDPath, i.OffsetMM, i.Alignment, i.StyleID, i.PanelID}
}

type LayoutTextAddInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	PageIndex int `json:"page_index"`
	LayerName string `json:"layer_name,omitempty" jsonschema:"optional existing LayOut layer name"`
	BoundsMM LayoutRectMM `json:"bounds_mm"`
	Text string `json:"text"`
	FontSizePT float64 `json:"font_size_pt"`
	Bold bool `json:"bold"`
	Alignment string `json:"alignment" jsonschema:"left center or right"`
	StyleID string `json:"style_id,omitempty" jsonschema:"optional tagged template style sample id"`
	PanelID string `json:"panel_id,omitempty" jsonschema:"optional template panel ID; associates this runtime entity for layout.panel.validate containment checks"`
}

func (i LayoutTextAddInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if err := i.BoundsMM.Validate(); err != nil { return err }
	if strings.TrimSpace(i.Text) == "" { return errors.New("text is required") }
	if !finite(i.FontSizePT) || i.FontSizePT <= 0 { return errors.New("font_size_pt must be greater than zero") }
	switch strings.ToLower(i.Alignment) {
	case "left", "center", "right":
	default: return errors.New("alignment must be left, center, or right")
	}
	return nil
}

func (i LayoutTextAddInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		PageIndex int `json:"page_index"`
		LayerName string `json:"layer_name"`
		BoundsMM LayoutRectMM `json:"bounds_mm"`
		Text string `json:"text"`
		FontSizePT float64 `json:"font_size_pt"`
		Bold bool `json:"bold"`
		Alignment string `json:"alignment"`
		StyleID string `json:"style_id"`
		PanelID string `json:"panel_id"`
	}{i.bridgeMutation("SketchUp MCP: Add LayOut Text"), i.LayoutPath, i.PageIndex, i.LayerName, i.BoundsMM, i.Text, i.FontSizePT, i.Bold, i.Alignment, i.StyleID, i.PanelID}
}

type LayoutLineAddInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	PageIndex int `json:"page_index"`
	LayerName string `json:"layer_name,omitempty" jsonschema:"optional existing LayOut layer name"`
	StartMM LayoutPoint2MM `json:"start_mm"`
	EndMM LayoutPoint2MM `json:"end_mm"`
	StrokeWidth float64 `json:"stroke_width"`
	StyleID string `json:"style_id,omitempty" jsonschema:"optional tagged template style sample id"`
	PanelID string `json:"panel_id,omitempty" jsonschema:"optional template panel ID; associates this runtime entity for layout.panel.validate containment checks"`
}

func (i LayoutLineAddInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if err := i.StartMM.Validate(); err != nil { return err }
	if err := i.EndMM.Validate(); err != nil { return err }
	if i.StartMM == i.EndMM { return errors.New("line start and end must differ") }
	if !finite(i.StrokeWidth) || i.StrokeWidth <= 0 { return errors.New("stroke_width must be greater than zero") }
	return nil
}

func (i LayoutLineAddInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		PageIndex int `json:"page_index"`
		LayerName string `json:"layer_name"`
		StartMM LayoutPoint2MM `json:"start_mm"`
		EndMM LayoutPoint2MM `json:"end_mm"`
		StrokeWidth float64 `json:"stroke_width"`
		StyleID string `json:"style_id"`
		PanelID string `json:"panel_id"`
	}{i.bridgeMutation("SketchUp MCP: Add LayOut Line"), i.LayoutPath, i.PageIndex, i.LayerName, i.StartMM, i.EndMM, i.StrokeWidth, i.StyleID, i.PanelID}
}

type LayoutRectangleAddInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	PageIndex int `json:"page_index"`
	LayerName string `json:"layer_name,omitempty" jsonschema:"optional existing LayOut layer name"`
	BoundsMM LayoutRectMM `json:"bounds_mm"`
	StrokeWidth float64 `json:"stroke_width"`
	StyleID string `json:"style_id,omitempty" jsonschema:"optional tagged template style sample id"`
	PanelID string `json:"panel_id,omitempty" jsonschema:"optional template panel ID; associates this runtime entity for layout.panel.validate containment checks"`
}

func (i LayoutRectangleAddInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if err := i.BoundsMM.Validate(); err != nil { return err }
	if !finite(i.StrokeWidth) || i.StrokeWidth <= 0 { return errors.New("stroke_width must be greater than zero") }
	return nil
}

func (i LayoutRectangleAddInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		PageIndex int `json:"page_index"`
		LayerName string `json:"layer_name"`
		BoundsMM LayoutRectMM `json:"bounds_mm"`
		StrokeWidth float64 `json:"stroke_width"`
		StyleID string `json:"style_id"`
		PanelID string `json:"panel_id"`
	}{i.bridgeMutation("SketchUp MCP: Add LayOut Rectangle"), i.LayoutPath, i.PageIndex, i.LayerName, i.BoundsMM, i.StrokeWidth, i.StyleID, i.PanelID}
}

type LayoutExportInput struct {
	MutationEnvelope
	LayoutPath string `json:"layout_path"`
	OutputPath string `json:"output_path" jsonschema:"absolute .pdf .png or .jpg output path"`
	DPI int `json:"dpi,omitempty" jsonschema:"image export DPI; ignored for PDF"`
}

func (i LayoutExportInput) Validate() error {
	if err := i.MutationEnvelope.Validate(); err != nil { return err }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if strings.TrimSpace(i.OutputPath) == "" || !filepath.IsAbs(i.OutputPath) { return errors.New("output_path must be absolute") }
	switch strings.ToLower(filepath.Ext(i.OutputPath)) {
	case ".pdf":
		if i.DPI != 0 { return errors.New("dpi must be 0 for PDF export") }
	case ".png", ".jpg", ".jpeg":
		if i.DPI <= 0 { return errors.New("dpi must be greater than zero for image export") }
	default:
		return errors.New("output_path must end in .pdf, .png, .jpg, or .jpeg")
	}
	return nil
}

func (i LayoutExportInput) BridgePayload() any {
	return struct {
		Mutation BridgeMutation `json:"mutation"`
		LayoutPath string `json:"layout_path"`
		OutputPath string `json:"output_path"`
		DPI int `json:"dpi"`
	}{i.bridgeMutation("SketchUp MCP: Export LayOut Document"), i.LayoutPath, i.OutputPath, i.DPI}
}

type LayoutFileOutput struct {
	OperationID string `json:"operation_id,omitempty"`
	ModelGUID string `json:"model_guid,omitempty"`
	Revision uint64 `json:"revision"`
	LayoutPath string `json:"layout_path,omitempty"`
	Error *ToolError `json:"error,omitempty"`
}

type LayoutEntityOutput struct {
	OperationID string `json:"operation_id,omitempty"`
	ModelGUID string `json:"model_guid,omitempty"`
	Revision uint64 `json:"revision"`
	LayoutPath string `json:"layout_path,omitempty"`
	EntityRef *LayoutEntityRef `json:"entity_ref,omitempty"`
	Connected bool `json:"connected,omitempty" jsonschema:"true when an associative dimension is connected to its LayOut viewport and SketchUp model geometry"`
	FitChecked bool `json:"fit_checked,omitempty"`
	FitsBounds bool `json:"fits_bounds,omitempty"`
	ProjectedBoundsMM *LayoutRectMM `json:"projected_bounds_mm,omitempty"`
	Error *ToolError `json:"error,omitempty"`
}

type LayoutExportOutput struct {
	OperationID string `json:"operation_id,omitempty"`
	ModelGUID string `json:"model_guid,omitempty"`
	Revision uint64 `json:"revision"`
	LayoutPath string `json:"layout_path,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Error *ToolError `json:"error,omitempty"`
}
