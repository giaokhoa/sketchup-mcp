package model

import (
	"errors"
	"strings"
)

type LayoutModelBoundsMM struct {
	MinMM LayoutPoint3MM `json:"min_mm"`
	MaxMM LayoutPoint3MM `json:"max_mm"`
}

func (b LayoutModelBoundsMM) Validate() error {
	if err := b.MinMM.Validate(); err != nil { return err }
	if err := b.MaxMM.Validate(); err != nil { return err }
	if b.MinMM.X > b.MaxMM.X || b.MinMM.Y > b.MaxMM.Y || b.MinMM.Z > b.MaxMM.Z {
		return errors.New("model bounds min_mm must not exceed max_mm")
	}
	return nil
}

type LayoutPanelValidateInput struct {
	SessionID  string  `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
	LayoutPath string  `json:"layout_path"`
	PageIndex  int     `json:"page_index"`
	PanelID    string  `json:"panel_id"`
	MarginMM   float64 `json:"margin_mm,omitempty" jsonschema:"inward safety margin in millimeters"`
}

func (i LayoutPanelValidateInput) Validate() error {
	if strings.TrimSpace(i.SessionID) == "" { return errors.New("session_id is required") }
	if err := validateLayoutPath(i.LayoutPath); err != nil { return err }
	if i.PageIndex < 0 { return errors.New("page_index must be non-negative") }
	if strings.TrimSpace(i.PanelID) == "" { return errors.New("panel_id is required") }
	if !finite(i.MarginMM) || i.MarginMM < 0 { return errors.New("margin_mm must be finite and non-negative") }
	return nil
}

type LayoutPanelViolation struct {
	EntityID   string       `json:"entity_id,omitempty"`
	EntityType string       `json:"entity_type"`
	BoundsMM   LayoutRectMM `json:"bounds_mm"`
}

type LayoutPanelValidateOutput struct {
	LayoutPath    string                 `json:"layout_path,omitempty"`
	PageIndex     int                    `json:"page_index"`
	PanelID       string                 `json:"panel_id,omitempty"`
	PanelBoundsMM *LayoutRectMM          `json:"panel_bounds_mm,omitempty"`
	EntityCount   int                    `json:"entity_count"`
	Fits          bool                   `json:"fits"`
	Violations    []LayoutPanelViolation `json:"violations"`
	Error         *ToolError             `json:"error,omitempty"`
}
