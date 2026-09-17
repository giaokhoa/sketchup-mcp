package model

import (
	"errors"
	"path/filepath"
	"strings"
)

type LayoutTemplateInspectInput struct {
	SessionID    string `json:"session_id" jsonschema:"SketchUp MCP session identifier"`
	TemplatePath string `json:"template_path" jsonschema:"absolute .layout template path on the SketchUp machine"`
}

func (i LayoutTemplateInspectInput) Validate() error {
	if strings.TrimSpace(i.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(i.TemplatePath) == "" || !filepath.IsAbs(i.TemplatePath) {
		return errors.New("template_path must be absolute")
	}
	if strings.ToLower(filepath.Ext(i.TemplatePath)) != ".layout" {
		return errors.New("template_path must end in .layout")
	}
	return nil
}

type LayoutTemplatePage struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type LayoutTemplateLayer struct {
	Name   string `json:"name"`
	Shared bool   `json:"shared"`
	Locked bool   `json:"locked"`
}

type LayoutTemplateSlot struct {
	SlotID                  string       `json:"slot_id"`
	PageIndex               int          `json:"page_index"`
	BoundsMM                LayoutRectMM `json:"bounds_mm"`
	DefaultScaleDenominator float64      `json:"default_scale_denominator,omitempty"`
	Perspective             bool         `json:"perspective,omitempty"`
}

type LayoutTemplatePanel struct {
	PanelID    string       `json:"panel_id"`
	PageIndex  int          `json:"page_index"`
	BoundsMM   LayoutRectMM `json:"bounds_mm"`
}

type LayoutTemplateStyle struct {
	StyleID   string `json:"style_id"`
	PageIndex int    `json:"page_index"`
}

type LayoutTemplateInspectOutput struct {
	TemplatePath  string                `json:"template_path,omitempty"`
	SchemaVersion int                   `json:"schema_version,omitempty"`
	TemplateID    string                `json:"template_id,omitempty"`
	TemplateKind  string                `json:"template_kind,omitempty"`
	Pages         []LayoutTemplatePage  `json:"pages"`
	Layers        []LayoutTemplateLayer `json:"layers"`
	Slots         []LayoutTemplateSlot  `json:"slots"`
	Panels        []LayoutTemplatePanel `json:"panels"`
	Styles        []LayoutTemplateStyle `json:"styles"`
	AutoTextTypes []string              `json:"auto_text_types"`
	Error         *ToolError            `json:"error,omitempty"`
}
